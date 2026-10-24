package solver

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Length is a non-negative length in inches, held exactly as an integer count
// of microinches. One microinch resolves 1/64 inch without rounding.
//
// The zero value is zero inches. A negative Length cannot be constructed.
type Length struct{ micro int64 }

const microPerInch = 1_000_000

var jsonNumber = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?(?:[eE]([+-]?[0-9]+))?$`)

// maxExponent bounds the exponent before big.Rat expands 10^exp. A non-zero
// value with a larger exponent overflows or is finer than one microinch.
const maxExponent = 1000

// ParseLength accepts a decimal number token, such as the text of a
// store.Inches value ("96", "11.25", "0.125"). It rejects a negative value,
// a value with more precision than one microinch, and a value too large for
// int64 microinches.
func ParseLength(token string) (Length, error) {
	token = strings.TrimSpace(token)
	m := jsonNumber.FindStringSubmatch(token)
	if m == nil {
		return Length{}, ErrLength
	}
	if strings.Trim(m[1]+strings.TrimPrefix(m[2], "."), "0") == "" {
		return Length{}, nil
	}
	if m[3] != "" {
		exp, err := strconv.Atoi(m[3])
		if err != nil || exp > maxExponent || exp < -maxExponent {
			return Length{}, ErrLength
		}
	}
	r, ok := new(big.Rat).SetString(token)
	if !ok {
		return Length{}, ErrLength
	}
	r.Mul(r, big.NewRat(microPerInch, 1))
	if !r.IsInt() || !r.Num().IsInt64() {
		return Length{}, ErrLength
	}
	return Length{micro: r.Num().Int64()}, nil
}

// String formats the length as a minimal decimal token in inches.
func (l Length) String() string {
	whole := strconv.FormatInt(l.micro/microPerInch, 10)
	frac := l.micro % microPerInch
	if frac == 0 {
		return whole
	}
	digits := strconv.FormatInt(frac+microPerInch, 10)[1:]
	return whole + "." + strings.TrimRight(digits, "0")
}

// IsZero reports whether the length is zero inches.
func (l Length) IsZero() bool { return l.micro == 0 }
