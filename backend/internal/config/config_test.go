package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMissingDatabaseURL(t *testing.T) {
	secret := "sk_test_should_not_leak"
	_, err := parse(func(key string) (string, bool) {
		switch key {
		case "CLERK_SECRET_KEY":
			return secret, true
		default:
			return "", false
		}
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "DATABASE_URL") {
		t.Fatalf("error %q missing DATABASE_URL", msg)
	}
	if strings.Contains(msg, secret) {
		t.Fatalf("error %q leaked secret", msg)
	}
}

func TestParseHTTPAddrInvalidOmitsValue(t *testing.T) {
	const bad = "secret-host-without-port"
	_, err := parse(lookupMap(map[string]string{
		"DATABASE_URL": "postgres://localhost/db",
		"HTTP_ADDR":    bad,
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "HTTP_ADDR") {
		t.Fatal("error missing HTTP_ADDR")
	}
	if strings.Contains(msg, bad) {
		t.Fatal("error included HTTP_ADDR value")
	}
}

func TestConfigLogOmitsSecrets(t *testing.T) {
	const dsn = "postgres://user:s3cret-pass@localhost:5432/lumbercalc"
	const clerk = "sk_test_supersecret"
	cfg, err := parse(lookupMap(map[string]string{
		"DATABASE_URL":     dsn,
		"CLERK_SECRET_KEY": clerk,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("cfg", "config", cfg)
	logged := buf.String()
	if strings.Contains(logged, "s3cret-pass") || strings.Contains(logged, clerk) {
		t.Fatal("log included a secret")
	}
	printed := fmt.Sprintf("%v %+v %#v", cfg, cfg, cfg)
	if strings.Contains(printed, "s3cret-pass") || strings.Contains(printed, clerk) {
		t.Fatal("fmt included a secret")
	}
}

func TestParseHTTPAddrDefault(t *testing.T) {
	cfg, err := parse(func(key string) (string, bool) {
		if key == "DATABASE_URL" {
			return "postgres://localhost:5432/lumbercalc?sslmode=disable", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
}

func TestParseLogLevel(t *testing.T) {
	t.Run("debug", func(t *testing.T) {
		cfg, err := parse(lookupMap(map[string]string{
			"DATABASE_URL": "postgres://localhost/db",
			"LOG_LEVEL":    "debug",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.LogLevel != slog.LevelDebug {
			t.Fatalf("LogLevel = %v, want debug", cfg.LogLevel)
		}
	})
	t.Run("empty", func(t *testing.T) {
		cfg, err := parse(lookupMap(map[string]string{
			"DATABASE_URL": "postgres://localhost/db",
		}))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.LogLevel != slog.LevelInfo {
			t.Fatalf("LogLevel = %v, want info", cfg.LogLevel)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		_, err := parse(lookupMap(map[string]string{
			"DATABASE_URL": "postgres://localhost/db",
			"LOG_LEVEL":    "nope",
		}))
		if err == nil {
			t.Fatal("expected error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "LOG_LEVEL") {
			t.Fatalf("error %q missing LOG_LEVEL", msg)
		}
		if strings.Contains(msg, "nope") {
			t.Fatalf("error %q contains value", msg)
		}
	})
}

func TestSecretRedaction(t *testing.T) {
	s := Secret{value: "sk_test_supersecret"}
	if strings.Contains(s.String(), "sk_test_supersecret") {
		t.Fatalf("String leaked: %q", s.String())
	}
	if strings.Contains(s.GoString(), "sk_test_supersecret") {
		t.Fatalf("GoString leaked: %q", s.GoString())
	}
	if strings.Contains(fmt.Sprintf("%#v", s), "sk_test_supersecret") {
		t.Fatalf("fmt GoString form leaked: %s", fmt.Sprintf("%#v", s))
	}
	lv := s.LogValue().String()
	if strings.Contains(lv, "sk_test_supersecret") {
		t.Fatalf("LogValue leaked: %q", lv)
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "sk_test_supersecret") {
		t.Fatalf("MarshalJSON leaked: %s", b)
	}
}

func TestLoadMissingDotenv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/lumbercalc?sslmode=disable")
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.env"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL.Reveal() != "postgres://localhost:5432/lumbercalc?sslmode=disable" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL.Reveal())
	}
}

func TestProcessEnvWinsOverDotenv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "DATABASE_URL=postgres://file/db\nHTTP_ADDR=:9999\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "postgres://env/db")
	t.Setenv("HTTP_ADDR", ":7777")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL.Reveal() != "postgres://env/db" {
		t.Fatalf("DatabaseURL = %q, want env value", cfg.DatabaseURL.Reveal())
	}
	if cfg.HTTPAddr != ":7777" {
		t.Fatalf("HTTPAddr = %q, want :7777", cfg.HTTPAddr)
	}
}

func lookupMap(m map[string]string) lookup {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}
