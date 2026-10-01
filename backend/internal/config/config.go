// Package config loads process configuration from a dotenv file and the environment.
// Load ignores a missing file. A variable set in the process environment overrides the file.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// Config is validated process configuration.
// HTTP_ADDR defaults to :8080 and LOG_LEVEL defaults to info.
// CLERK_AUTHORIZED_PARTIES is required. A blank or comma-only value is rejected.
// Each entry is an origin. The scheme is http or https. The entry has a host.
// The entry has no user info, path, query, or fragment.
type Config struct {
	HTTPAddr               string
	LogLevel               slog.Level
	DatabaseURL            Secret
	ClerkSecretKey         Secret
	ClerkAuthorizedParties []string
}

// Secret hides a credential from fmt, slog, and JSON. Reveal is the only way to read it.
type Secret struct {
	value string
}

func (s Secret) Reveal() string               { return s.value }
func (s Secret) IsSet() bool                  { return s.value != "" }
func (s Secret) String() string               { return "[redacted]" }
func (s Secret) GoString() string             { return "[redacted]" }
func (s Secret) LogValue() slog.Value         { return slog.StringValue("[redacted]") }
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"[redacted]"`), nil }

type lookup func(key string) (value string, ok bool)

// Load reads dotenvPath, then lets the process environment override it.
// A missing file is not an error. Invalid or missing required values come back as one joined error.
func Load(dotenvPath string) (Config, error) {
	env, err := readEnv(dotenvPath)
	if err != nil {
		return Config{}, err
	}
	return parse(env)
}

// LoadDatabaseURL reads DATABASE_URL with the same file and process rules as Load.
// Clerk settings are not read. Migrate uses this so a database-only environment can migrate.
func LoadDatabaseURL(dotenvPath string) (Secret, error) {
	env, err := readEnv(dotenvPath)
	if err != nil {
		return Secret{}, err
	}
	dsn, _ := env("DATABASE_URL")
	if dsn == "" {
		return Secret{}, errors.New("DATABASE_URL is required")
	}
	return Secret{value: dsn}, nil
}

func readEnv(dotenvPath string) (lookup, error) {
	fileVars, err := godotenv.Read(dotenvPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		fileVars = nil
	}
	return layered(os.LookupEnv, fileVars), nil
}

func parse(env lookup) (Config, error) {
	var problems []error

	addr, ok := env("HTTP_ADDR")
	if !ok || addr == "" {
		addr = ":8080"
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		problems = append(problems, errors.New("HTTP_ADDR is invalid"))
	}

	levelRaw, _ := env("LOG_LEVEL")
	level, err := parseLevel(levelRaw)
	if err != nil {
		problems = append(problems, err)
	}

	dsn, _ := env("DATABASE_URL")
	if dsn == "" {
		problems = append(problems, errors.New("DATABASE_URL is required"))
	}

	clerk, _ := env("CLERK_SECRET_KEY")
	if clerk == "" {
		problems = append(problems, errors.New("CLERK_SECRET_KEY is required"))
	}

	partiesRaw, partiesOK := env("CLERK_AUTHORIZED_PARTIES")
	parties, partiesErr := clerkParties(partiesRaw, partiesOK)
	if partiesErr != nil {
		problems = append(problems, partiesErr)
	}

	if len(problems) > 0 {
		return Config{}, errors.Join(problems...)
	}

	return Config{
		HTTPAddr:               addr,
		LogLevel:               level,
		DatabaseURL:            Secret{value: dsn},
		ClerkSecretKey:         Secret{value: clerk},
		ClerkAuthorizedParties: parties,
	}, nil
}

func clerkParties(raw string, ok bool) ([]string, error) {
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, errors.New("CLERK_AUTHORIZED_PARTIES is required")
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !isOrigin(p) {
			return nil, fmt.Errorf("CLERK_AUTHORIZED_PARTIES entry %q is not an origin", p)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, errors.New("CLERK_AUTHORIZED_PARTIES is required")
	}
	return out, nil
}

// isOrigin reports whether s is an origin a browser can send.
// A trailing slash is a path, so "http://localhost:5173/" is rejected.
func isOrigin(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != "" && u.User == nil && u.Path == "" &&
		u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

func parseLevel(raw string) (slog.Level, error) {
	if raw == "" {
		return slog.LevelInfo, nil
	}
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, errors.New("LOG_LEVEL is invalid")
	}
}

func layered(primary lookup, fallback map[string]string) lookup {
	return func(key string) (string, bool) {
		if v, ok := primary(key); ok {
			return v, true
		}
		if fallback == nil {
			return "", false
		}
		v, ok := fallback[key]
		return v, ok
	}
}
