package solver_test

import (
	"errors"
	"testing"

	"lumbercalc/backend/internal/solver"
)

func TestParseLengthRoundTrips(t *testing.T) {
	cases := map[string]string{
		"96":       "96",
		"1.5":      "1.5",
		"0.125":    "0.125",
		"1e-1":     "0.1",
		"0.015625": "0.015625",
		"0":        "0",
		" 11.25\n": "11.25",
		"2.50":     "2.5",
		"0e999999": "0",
	}
	for in, want := range cases {
		l, err := solver.ParseLength(in)
		if err != nil {
			t.Errorf("ParseLength(%q) error %v", in, err)
			continue
		}
		if got := l.String(); got != want {
			t.Errorf("ParseLength(%q).String() = %q, want %q", in, got, want)
		}
	}
}

func TestParseLengthRejects(t *testing.T) {
	for _, in := range []string{
		"-1", "-0", "", "1/2", "0.0000001", "1 2", "abc", "01",
		"9223372036855", "1e999999999", "1e-999999999",
	} {
		if _, err := solver.ParseLength(in); !errors.Is(err, solver.ErrLength) {
			t.Errorf("ParseLength(%q) error = %v, want ErrLength", in, err)
		}
	}
}
