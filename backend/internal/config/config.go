package config

import (
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr       string
	LogLevel       slog.Level
	DatabaseURL    Secret
	ClerkSecretKey Secret
}

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

func Load(dotenvPath string) (Config, error) {
	fileVars, err := godotenv.Read(dotenvPath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
		fileVars = nil
	}
	return parse(layered(os.LookupEnv, fileVars))
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

	if len(problems) > 0 {
		return Config{}, errors.Join(problems...)
	}

	return Config{
		HTTPAddr:       addr,
		LogLevel:       level,
		DatabaseURL:    Secret{value: dsn},
		ClerkSecretKey: Secret{value: clerk},
	}, nil
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
