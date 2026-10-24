package solver

import "math"

type pattern struct {
	stock int
	count int
	cuts  []int
}

func newPattern(p problem, stock, count int, cuts []int) (pattern, bool) {
	if count < 1 || len(cuts) == 0 {
		return pattern{}, false
	}
	if usedLength(p, cuts) > p.stock[stock].Length.micro {
		return pattern{}, false
	}
	return pattern{stock: stock, count: count, cuts: cuts}, true
}

func usedLength(p problem, cuts []int) int64 {
	var used int64
	for i, c := range cuts {
		used = satAdd(used, p.demands[c].Length.micro)
		if i > 0 {
			used = satAdd(used, p.kerf.micro)
		}
	}
	return used
}

func (pt pattern) leftover(p problem) Leftover {
	used := usedLength(p, pt.cuts)
	l := Length{micro: p.stock[pt.stock].Length.micro - used}
	return Leftover{Length: l, Kept: !p.minRemnant.IsZero() && l.micro >= p.minRemnant.micro}
}

type layout struct {
	stock         int
	countByLength []int
}

const maxLayoutNodes = 100_000

func (p problem) ticksIncludingKerf(micro int64) uint64 {
	return (uint64(micro) + uint64(p.kerf.micro)) / uint64(p.microsPerTick)
}

func (p problem) partTicks(i int) uint64 {
	return p.ticksIncludingKerf(p.lengths[i].micro)
}

func (p problem) stockTicks(s int) uint64 {
	return p.ticksIncludingKerf(p.stock[s].Length.micro)
}

func (p problem) lengthQuantity(i int) int {
	var q int64
	for _, d := range p.byLength[i] {
		q = satAdd(q, int64(p.demands[d].Quantity))
	}
	return int(min(q, math.MaxInt))
}

func maximalLayouts(p problem, stock int) (layouts []layout, complete bool) {
	k := len(p.lengths)
	w := make([]uint64, k)
	q := make([]int, k)
	for i := range k {
		w[i], q[i] = p.partTicks(i), p.lengthQuantity(i)
	}
	reach := make([]uint64, k+1)
	for i := k - 1; i >= 0; i-- {
		reach[i] = satMulAdd(reach[i+1], uint64(q[i]), w[i])
	}

	counts := make([]int, k)
	nodes := 0
	aborted := false
	var walk func(i int, room, unmetPart uint64)
	walk = func(i int, room, unmetPart uint64) {
		if i == k {
			if room >= unmetPart {
				return
			}
			total := 0
			for _, c := range counts {
				total += c
			}
			if total > 0 {
				layouts = append(layouts, layout{stock: stock, countByLength: append([]int(nil), counts...)})
			}
			return
		}
		most := uint64(q[i])
		if fit := room / w[i]; fit < most {
			most = fit
		}
		if most > maxLayoutNodes {
			aborted = true
			return
		}
		for n := int(most); n >= 0; n-- {
			nodes++
			if nodes > maxLayoutNodes {
				aborted = true
				break
			}
			counts[i] = n
			left := room - uint64(n)*w[i]
			open := unmetPart
			if n < q[i] {
				open = min(open, w[i])
			}
			if left < reach[i+1] || left-reach[i+1] < open {
				walk(i+1, left, open)
			}
			if aborted {
				break
			}
		}
		counts[i] = 0
	}
	walk(0, p.stockTicks(stock), math.MaxUint64)
	return layouts, !aborted
}
