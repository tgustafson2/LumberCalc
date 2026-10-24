package solver

import "slices"

// Status tells whether the search proved the plan cheapest.
type Status int

const (
	statusUnknown Status = iota
	// Exact means the search finished. No plan costs less.
	Exact
	// Feasible means the budget expired first. The plan cuts every demand,
	// but a cheaper plan may exist.
	Feasible
)

// Plan is the result of one Solve.
type Plan struct {
	p        problem
	status   Status
	patterns []pattern
}

// Board is a read-only view of Count identical boards cut the same way.
type Board struct {
	// Stock is an index into Request.Stock.
	Stock int
	Count int
	// Cuts holds indices into Request.Demands in saw order, one entry per part.
	Cuts     []int
	Leftover Leftover
}

// Leftover is the length that remains on one board after its last part.
type Leftover struct {
	Length Length
	// Kept is true when Length is at least Request.MinRemnant and
	// MinRemnant is not zero. A kept leftover is an offcut, not waste.
	Kept bool
}

// Purchase is Count boards of one stock length.
type Purchase struct {
	// Stock is an index into Request.Stock.
	Stock int
	Count int
}

// Status reports whether the plan is proven cheapest.
func (pl Plan) Status() Status { return pl.status }

// Boards lists every distinct cut pattern with its count. The caller may
// change the returned slices.
func (pl Plan) Boards() []Board {
	var boards []Board
	for _, pt := range pl.patterns {
		boards = append(boards, Board{
			Stock:    pt.stock,
			Count:    pt.count,
			Cuts:     slices.Clone(pt.cuts),
			Leftover: pt.leftover(pl.p),
		})
	}
	return boards
}

// Purchases is the buy list, one entry per stock length bought, in
// Request.Stock order.
func (pl Plan) Purchases() []Purchase {
	counts := make([]int64, len(pl.p.stock))
	for _, pt := range pl.patterns {
		counts[pt.stock] = satAdd(counts[pt.stock], int64(pt.count))
	}
	var buys []Purchase
	for s, n := range counts {
		if n > 0 {
			buys = append(buys, Purchase{Stock: s, Count: int(n)})
		}
	}
	return buys
}

// Cost is the sum of price times boards bought. It saturates at
// math.MaxInt64 cents.
func (pl Plan) Cost() Cents {
	var cost int64
	for _, pt := range pl.patterns {
		cost = satAdd(cost, satMul(int64(pl.p.stock[pt.stock].Price), int64(pt.count)))
	}
	return Cents(cost)
}

// Waste is the total leftover length that is not a kept offcut. It
// saturates at math.MaxInt64 microinches.
func (pl Plan) Waste() Length {
	var waste int64
	for _, pt := range pl.patterns {
		if lo := pt.leftover(pl.p); !lo.Kept {
			waste = satAdd(waste, satMul(lo.Length.micro, int64(pt.count)))
		}
	}
	return Length{micro: waste}
}

func (pl Plan) boardCount() int64 {
	var n int64
	for _, pt := range pl.patterns {
		n = satAdd(n, int64(pt.count))
	}
	return n
}

func (pl Plan) better(other Plan) bool {
	if a, b := pl.Cost(), other.Cost(); a != b {
		return a < b
	}
	if a, b := pl.Waste().micro, other.Waste().micro; a != b {
		return a < b
	}
	return pl.boardCount() < other.boardCount()
}
