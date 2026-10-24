package solver

import (
	"cmp"
	"context"
	"math"
	"slices"
	"time"
)

// Solve returns the cheapest set of boards that cuts every demand in req.
//
// A cancelled ctx returns ctx.Err and a zero Plan. Cancellation does not
// return a partial plan. Budget is a different limit. When it expires,
// Solve returns the best plan found, with Status Feasible.
//
// An empty Request.Demands returns a Plan with no boards, zero cost, and
// zero waste, and Status Exact. This still requires a ctx that is not
// already cancelled.
//
// Solve returns an *InputError or an *InfeasibleError for a bad request.
// A zero Budget skips the search and returns the first feasible plan with
// Status Feasible. Solve does not panic on any Request.
//
// The objective is lowest Cost, then lowest Waste, then fewest boards.
// For one length, cuts use the lowest demand index that still has quantity.
func Solve(ctx context.Context, req Request) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	p, err := parse(req)
	if err != nil {
		return Plan{}, err
	}
	if len(p.demands) == 0 {
		return Plan{p: p, status: Exact}, nil
	}
	start := seed(p)
	if p.budget == 0 {
		return Plan{p: p, status: Feasible, patterns: start}, nil
	}
	deadline := time.Now().Add(p.budget)
	var layouts []layout
	for s := range p.stock {
		ls, complete := maximalLayouts(p, s)
		if !complete {
			return Plan{p: p, status: Feasible, patterns: start}, nil
		}
		layouts = append(layouts, ls...)
	}
	slices.SortStableFunc(layouts, func(a, b layout) int {
		return cmp.Compare(a.stock, b.stock)
	})
	best, done := search(ctx, p, layouts, start, deadline)
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	status := Feasible
	if done {
		status = Exact
	}
	return Plan{p: p, status: status, patterns: best}, nil
}

type mix []int

func seed(p problem) []pattern {
	var pats []pattern
	for d, dem := range p.demands {
		best := -1
		for s, st := range p.stock {
			if st.Length.micro >= dem.Length.micro && (best < 0 || st.Price < p.stock[best].Price) {
				best = s
			}
		}
		if pt, ok := newPattern(p, best, dem.Quantity, []int{d}); ok {
			pats = append(pats, pt)
		}
	}
	return pats
}

const checkEvery = 1024

type ratio struct {
	num uint64
	den uint64
	ok  bool
}

func (r ratio) less(o ratio) bool {
	return !o.ok || (r.ok && ratioLess(r.num, r.den, o.num, o.den))
}

type searcher struct {
	ctx      context.Context
	deadline time.Time
	p        problem
	layouts  []layout

	ticks     []uint64
	lastCover []int
	perPart   [][]ratio
	perTick   []ratio

	remaining []int64
	x         mix
	cost      int64

	best     Plan
	bestCost int64
	nodes    int
	stopped  bool
}

func search(ctx context.Context, p problem, layouts []layout, incumbent []pattern, deadline time.Time) ([]pattern, bool) {
	s := newSearcher(ctx, p, layouts, deadline)
	s.best = Plan{p: p, patterns: incumbent}
	s.bestCost = int64(s.best.Cost())
	s.walk(0)
	return s.best.patterns, !s.stopped
}

func newSearcher(ctx context.Context, p problem, layouts []layout, deadline time.Time) *searcher {
	k, ns := len(p.lengths), len(p.stock)
	s := &searcher{
		ctx:       ctx,
		deadline:  deadline,
		p:         p,
		layouts:   layouts,
		ticks:     make([]uint64, k),
		lastCover: make([]int, k),
		perPart:   make([][]ratio, ns+1),
		perTick:   make([]ratio, ns+1),
		remaining: make([]int64, k),
		x:         make(mix, len(layouts)),
	}
	for i := range k {
		s.ticks[i] = p.partTicks(i)
		s.remaining[i] = int64(p.lengthQuantity(i))
		s.lastCover[i] = -1
	}
	for l, lay := range layouts {
		for i, c := range lay.countByLength {
			if c > 0 {
				s.lastCover[i] = l
			}
		}
	}
	s.perPart[ns] = make([]ratio, k)
	for st := ns - 1; st >= 0; st-- {
		price, capacity := uint64(p.stock[st].Price), p.stockTicks(st)
		s.perTick[st] = s.perTick[st+1]
		if r := (ratio{num: price, den: capacity, ok: capacity > 0}); r.less(s.perTick[st]) {
			s.perTick[st] = r
		}
		s.perPart[st] = slices.Clone(s.perPart[st+1])
		for i := range k {
			most := min(capacity/s.ticks[i], uint64(p.lengthQuantity(i)))
			if most == 0 {
				continue
			}
			if r := (ratio{num: price, den: most, ok: true}); r.less(s.perPart[st][i]) {
				s.perPart[st][i] = r
			}
		}
	}
	return s
}

func (s *searcher) stopIfDue() bool {
	s.nodes++
	if s.nodes%checkEvery == 0 && (s.ctx.Err() != nil || !time.Now().Before(s.deadline)) {
		s.stopped = true
	}
	return s.stopped
}

func (s *searcher) walk(l int) {
	if s.stopIfDue() {
		return
	}
	met := true
	for i, r := range s.remaining {
		if r > 0 {
			met = false
			if s.lastCover[i] < l {
				return
			}
		}
	}
	if met {
		s.leaf()
		return
	}
	if l == len(s.layouts) || pricedOut(satAdd(s.cost, s.bound(l)), s.bestCost) {
		return
	}

	lay := s.layouts[l]
	var most int64
	for i, c := range lay.countByLength {
		if r := s.remaining[i]; r > 0 && c > 0 {
			most = max(most, (r+int64(c)-1)/int64(c))
		}
	}
	price := int64(s.p.stock[lay.stock].Price)
	cost := s.cost
	for n := most; n >= 0 && !s.stopped; n-- {
		s.x[l] = int(n)
		for i, c := range lay.countByLength {
			s.remaining[i] -= n * int64(c)
		}
		s.cost = satAdd(cost, satMul(price, n))
		s.walk(l + 1)
		for i, c := range lay.countByLength {
			s.remaining[i] += n * int64(c)
		}
	}
	s.x[l] = 0
	s.cost = cost
}

func pricedOut(candidate, best int64) bool {
	return candidate > best
}

func (s *searcher) bound(l int) int64 {
	st := s.layouts[l].stock
	var lb int64
	var ticks uint64
	for i, r := range s.remaining {
		if r <= 0 {
			continue
		}
		if pp := s.perPart[st][i]; pp.ok {
			lb = max(lb, satCeilMulDiv(uint64(r), pp.num, pp.den))
		}
		ticks = satMulAdd(ticks, uint64(r), s.ticks[i])
	}
	if pt := s.perTick[st]; pt.ok {
		lb = max(lb, satCeilMulDiv(ticks, pt.num, pt.den))
	}
	return lb
}

func (s *searcher) leaf() {
	cand := Plan{p: s.p, patterns: assignCuts(s.p, s.layouts, s.x)}
	if cand.better(s.best) {
		s.best = cand
		s.bestCost = int64(cand.Cost())
	}
}

func assignCuts(p problem, layouts []layout, m mix) []pattern {
	remaining := make([]int, len(p.demands))
	for d, dem := range p.demands {
		remaining[d] = dem.Quantity
	}
	next := make([]int, len(p.lengths))
	type take struct{ demand, parts int }
	var pats []pattern
	for l, boards := range m {
		lay := layouts[l]
		for boards > 0 {
			var takes []take
			for i, c := range lay.countByLength {
				for j := next[i]; c > 0 && j < len(p.byLength[i]); j++ {
					d := p.byLength[i][j]
					t := min(c, remaining[d])
					takes = append(takes, take{d, t})
					c -= t
				}
			}
			if len(takes) == 0 {
				break
			}
			run := boards
			var cuts []int
			for _, t := range takes {
				run = min(run, remaining[t.demand]/t.parts)
				for range t.parts {
					cuts = append(cuts, t.demand)
				}
			}
			for _, t := range takes {
				remaining[t.demand] -= t.parts * run
			}
			for i := range next {
				for next[i] < len(p.byLength[i]) && remaining[p.byLength[i][next[i]]] == 0 {
					next[i]++
				}
			}
			boards -= run
			pats = mergePattern(p, pats, lay.stock, run, cuts)
		}
	}
	return pats
}

func mergePattern(p problem, pats []pattern, stock, count int, cuts []int) []pattern {
	for i := range pats {
		if pats[i].stock == stock && slices.Equal(pats[i].cuts, cuts) {
			pats[i].count = int(min(satAdd(int64(pats[i].count), int64(count)), math.MaxInt))
			return pats
		}
	}
	if pt, ok := newPattern(p, stock, count, cuts); ok {
		pats = append(pats, pt)
	}
	return pats
}
