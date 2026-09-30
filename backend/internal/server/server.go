// Package server is the HTTP process.
// /healthz and /readyz are public. /v1/ is mounted only when Deps.V1 is set.
package server

//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../openapi.yaml

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// Deps is the logger, the readiness probes, and the optional /v1 handler.
type Deps struct {
	Log   *slog.Logger
	Ready []Check
	V1    http.Handler
}

// Check is one readiness probe. Name must be unique and non-empty, and Probe must be non-nil.
type Check struct {
	Name  string
	Probe func(context.Context) error
}

const readyTimeout = 2 * time.Second
const shutdownGrace = 10 * time.Second

// New requires a logger and at least one uniquely named check.
// The chain is request id, then access log, then panic recovery, then the mux.
// /healthz does not run probes. /readyz runs every probe with a 2 second timeout.
// A failing probe is 503, and the body lists names and ok flags without the error text.
func New(d Deps) (http.Handler, error) {
	if d.Log == nil {
		return nil, errors.New("log is required")
	}
	if len(d.Ready) == 0 {
		return nil, errors.New("ready checks are required")
	}
	seen := make(map[string]struct{}, len(d.Ready))
	for _, c := range d.Ready {
		if c.Name == "" {
			return nil, errors.New("check name is required")
		}
		if c.Probe == nil {
			return nil, fmt.Errorf("check %q has nil probe", c.Name)
		}
		if _, ok := seen[c.Name]; ok {
			return nil, fmt.Errorf("duplicate check name %q", c.Name)
		}
		seen[c.Name] = struct{}{}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	mux.Handle("GET /readyz", readyz(d.Ready, d.Log))
	if d.V1 != nil {
		mux.Handle("/v1/", d.V1)
	}
	mux.HandleFunc("/", notFound)

	return requestID(accessLog(d.Log, recoverPanic(d.Log, mux))), nil
}

// Serve listens on addr until ctx is canceled, then drains in-flight requests for 10 seconds.
func Serve(ctx context.Context, addr string, h http.Handler, log *slog.Logger) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	errc := make(chan error, 1)
	go func() {
		errc <- srv.Serve(ln)
	}()

	log.Info("listening", "addr", addr)

	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-errc
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// NewLogger writes JSON and adds request_id when the request-id middleware
// stored one on the context.
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(ctxHandler{slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})})
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, Health{Status: HealthStatusOk})
}

func readyz(checks []Check, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
		defer cancel()

		results := make([]ReadinessCheck, len(checks))
		ready := true
		for i, c := range checks {
			err := c.Probe(ctx)
			ok := err == nil
			results[i] = ReadinessCheck{Name: c.Name, Ok: ok}
			if !ok {
				ready = false
				log.WarnContext(r.Context(), "readiness check failed", "check", c.Name)
			}
		}
		if ready {
			WriteJSON(w, http.StatusOK, ReadyOk{Status: ReadyOkStatusReady, Checks: results})
			return
		}
		WriteJSON(w, http.StatusServiceUnavailable, ReadyFailed{Status: ReadyFailedStatusNotReady, Checks: results})
	})
}

func notFound(w http.ResponseWriter, _ *http.Request) {
	WriteError(w, http.StatusNotFound)
}

func WriteError(w http.ResponseWriter, status int) {
	WriteJSON(w, status, Error{Error: strings.ToLower(http.StatusText(status))})
}

// WriteJSON sets Content-Type to application/json, writes status, then encodes body.
// An encode error is discarded because the status line is already sent.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		ctx := context.WithValue(r.Context(), ctxKey{}, id)
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(b[:])
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func accessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"duration", time.Since(start).String(),
		)
	})
}

func recoverPanic(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.ErrorContext(r.Context(), "handler panic")
				WriteError(w, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type ctxKey struct{}

type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, ok := ctx.Value(ctxKey{}).(string); ok && id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h ctxHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return ctxHandler{h.Handler.WithAttrs(as)}
}

func (h ctxHandler) WithGroup(name string) slog.Handler {
	return ctxHandler{h.Handler.WithGroup(name)}
}
