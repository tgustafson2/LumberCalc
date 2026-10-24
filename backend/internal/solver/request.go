package solver

import (
	"cmp"
	"slices"
	"time"
)

// Cents is money in integer cents.
type Cents int64

// Request is one 1D cutting-stock problem for one cross-section. Solve
// validates it. The caller keeps it, because a Plan refers to its demands and
// stock by slice index.
type Request struct {
	// Demands lists the parts to cut. Two demands may have the same length.
	// A cut in the Plan names its demand by index, so the caller can map a
	// cut back to its own piece.
	Demands []Demand

	// Stock lists the board lengths for sale. Each length appears once.
	Stock []Stock

	// Kerf is the saw gap between two adjacent parts on one board. There is
	// no gap after the last part. Zero means no gap. The solver has no default.
	Kerf Length

	// MinRemnant is the shortest leftover kept as an offcut. A shorter
	// leftover is waste. Zero keeps no offcut, so every leftover is waste.
	MinRemnant Length

	// Budget is the time the search may use. When it expires, Solve returns
	// the best plan found so far with Status Feasible. Zero skips the search
	// and returns the first feasible plan.
	Budget time.Duration
}

// Demand is Quantity parts of one Length.
type Demand struct {
	Length   Length
	Quantity int
}

// Stock is one board length for sale at Price cents per board.
type Stock struct {
	Length Length
	Price  Cents
}

type problem struct {
	demands    []Demand
	stock      []Stock
	kerf       Length
	minRemnant Length
	budget     time.Duration

	lengths  []Length
	byLength [][]int

	microsPerTick int64
}

func parse(req Request) (problem, error) {
	for i, d := range req.Demands {
		if d.Length.IsZero() {
			return problem{}, &InputError{Reason: DemandZeroLength, Index: i}
		}
		if d.Quantity <= 0 {
			return problem{}, &InputError{Reason: DemandQuantityNotPositive, Index: i}
		}
	}
	seen := make(map[Length]bool, len(req.Stock))
	var longest Length
	for i, s := range req.Stock {
		if s.Length.IsZero() {
			return problem{}, &InputError{Reason: StockZeroLength, Index: i}
		}
		if s.Price < 0 {
			return problem{}, &InputError{Reason: StockPriceNegative, Index: i}
		}
		if seen[s.Length] {
			return problem{}, &InputError{Reason: StockDuplicateLength, Index: i}
		}
		seen[s.Length] = true
		if s.Length.micro > longest.micro {
			longest = s.Length
		}
	}
	if req.Budget < 0 {
		return problem{}, &InputError{Reason: BudgetNegative, Index: -1}
	}

	var infeasible []int
	for i, d := range req.Demands {
		if d.Length.micro > longest.micro {
			infeasible = append(infeasible, i)
		}
	}
	if infeasible != nil {
		return problem{}, &InfeasibleError{Demands: infeasible}
	}

	p := problem{
		demands:    slices.Clone(req.Demands),
		stock:      slices.Clone(req.Stock),
		kerf:       req.Kerf,
		minRemnant: req.MinRemnant,
		budget:     req.Budget,
	}

	slot := make(map[Length]int)
	for _, d := range p.demands {
		if _, ok := slot[d.Length]; !ok {
			slot[d.Length] = 0
			p.lengths = append(p.lengths, d.Length)
		}
	}
	slices.SortFunc(p.lengths, func(a, b Length) int { return cmp.Compare(b.micro, a.micro) })
	for i, l := range p.lengths {
		slot[l] = i
	}
	p.byLength = make([][]int, len(p.lengths))
	for i, d := range p.demands {
		j := slot[d.Length]
		p.byLength[j] = append(p.byLength[j], i)
	}

	var step int64
	for _, l := range p.lengths {
		step = gcd(step, l.micro)
	}
	for _, s := range p.stock {
		step = gcd(step, s.Length.micro)
	}
	step = gcd(gcd(step, p.kerf.micro), p.minRemnant.micro)
	p.microsPerTick = max(step, 1)
	return p, nil
}

func gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
