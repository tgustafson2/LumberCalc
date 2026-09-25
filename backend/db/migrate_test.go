package db

import (
	"crypto/sha256"
	"slices"
	"testing"
	"testing/fstest"
)

func m(version int, name, sql string) migration {
	return migration{version: version, name: name, sql: sql, sha256: sha256.Sum256([]byte(sql))}
}

func a(version int, name, sql string) applied {
	return applied{version: version, name: name, sha256: sha256.Sum256([]byte(sql))}
}

func names(ms []migration) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = m.name
	}
	return out
}

func TestPlan(t *testing.T) {
	want := []migration{
		m(1, "001_init", "CREATE TABLE a ();"),
		m(2, "002_more", "CREATE TABLE b ();"),
		m(3, "003_last", "CREATE TABLE c ();"),
	}
	tests := []struct {
		name    string
		have    []applied
		pending []string
		err     string
	}{
		{
			name:    "empty database",
			have:    nil,
			pending: []string{"001_init", "002_more", "003_last"},
		},
		{
			name:    "partly applied",
			have:    []applied{a(1, "001_init", "CREATE TABLE a ();")},
			pending: []string{"002_more", "003_last"},
		},
		{
			name: "up to date",
			have: []applied{
				a(1, "001_init", "CREATE TABLE a ();"),
				a(2, "002_more", "CREATE TABLE b ();"),
				a(3, "003_last", "CREATE TABLE c ();"),
			},
			pending: []string{},
		},
		{
			name: "checksum drift",
			have: []applied{
				a(1, "001_init", "CREATE TABLE a ();"),
				a(2, "002_more", "CREATE TABLE b (id int);"),
			},
			err: "migration 002_more was changed after it was applied as 002_more",
		},
		{
			name: "renamed file",
			have: []applied{a(1, "001_first", "CREATE TABLE a ();")},
			err:  "migration 001_init was changed after it was applied as 001_first",
		},
		{
			name: "gap",
			have: []applied{
				a(1, "001_init", "CREATE TABLE a ();"),
				a(3, "003_last", "CREATE TABLE c ();"),
			},
			err: "database is missing migration 002_more but has 003 003_last",
		},
		{
			name: "database ahead of binary",
			have: []applied{
				a(1, "001_init", "CREATE TABLE a ();"),
				a(2, "002_more", "CREATE TABLE b ();"),
				a(3, "003_last", "CREATE TABLE c ();"),
				a(4, "004_future", "CREATE TABLE d ();"),
			},
			err: "database has migration 004 004_future, this binary knows only up to 003",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := plan(want, tt.have)
			if tt.err != "" {
				if err == nil || err.Error() != tt.err {
					t.Fatalf("err = %v, want %q", err, tt.err)
				}
				if got != nil {
					t.Fatalf("pending = %v, want nil on error", names(got))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if g := names(got); !slices.Equal(g, tt.pending) {
				t.Fatalf("pending = %v, want %v", g, tt.pending)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name  string
		files fstest.MapFS
		want  []string
		err   string
	}{
		{
			name: "sorted by version",
			files: fstest.MapFS{
				"migrations/002_more.sql": {Data: []byte("SELECT 2;")},
				"migrations/001_init.sql": {Data: []byte("SELECT 1;")},
			},
			want: []string{"001_init", "002_more"},
		},
		{
			name: "gap",
			files: fstest.MapFS{
				"migrations/001_init.sql": {Data: []byte("SELECT 1;")},
				"migrations/003_last.sql": {Data: []byte("SELECT 3;")},
			},
			err: "migration 003_last: want version 002",
		},
		{
			name: "bad name",
			files: fstest.MapFS{
				"migrations/1_init.sql": {Data: []byte("SELECT 1;")},
			},
			err: "migration file 1_init.sql: want NNN_name.sql",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := load(tt.files)
			if tt.err != "" {
				if err == nil || err.Error() != tt.err {
					t.Fatalf("err = %v, want %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if g := names(got); !slices.Equal(g, tt.want) {
				t.Fatalf("names = %v, want %v", g, tt.want)
			}
		})
	}
}

func TestEmbeddedIsInit(t *testing.T) {
	if g := names(embedded); !slices.Equal(g, []string{"001_init"}) {
		t.Fatalf("embedded = %v, want [001_init]", g)
	}
}
