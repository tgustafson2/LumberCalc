package solver

import (
	"math"
	"math/bits"
)

func satAdd(a, b int64) int64 {
	if a > math.MaxInt64-b {
		return math.MaxInt64
	}
	return a + b
}

func satMul(a, b int64) int64 {
	if a != 0 && b > math.MaxInt64/a {
		return math.MaxInt64
	}
	return a * b
}

func satCeilMulDiv(a, b, c uint64) int64 {
	hi, lo := bits.Mul64(a, b)
	if hi >= c {
		return math.MaxInt64
	}
	q, r := bits.Div64(hi, lo, c)
	if r != 0 {
		q++
	}
	if q > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(q)
}

func satMulAdd(sum, a, b uint64) uint64 {
	hi, lo := bits.Mul64(a, b)
	if hi != 0 || sum+lo < sum {
		return math.MaxUint64
	}
	return sum + lo
}

func ratioLess(a, b, c, d uint64) bool {
	lh, ll := bits.Mul64(a, d)
	rh, rl := bits.Mul64(c, b)
	return lh < rh || (lh == rh && ll < rl)
}
