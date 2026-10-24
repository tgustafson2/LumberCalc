package solver_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"lumbercalc/backend/internal/solver"
)

func in(t *testing.T, token string) solver.Length {
	t.Helper()
	l, err := solver.ParseLength(token)
	if err != nil {
		t.Fatalf("ParseLength(%q): %v", token, err)
	}
	return l
}

func demand(t *testing.T, length string, qty int) solver.Demand {
	return solver.Demand{Length: in(t, length), Quantity: qty}
}

func stock(t *testing.T, length string, price solver.Cents) solver.Stock {
	return solver.Stock{Length: in(t, length), Price: price}
}

func assertCovered(t *testing.T, req solver.Request, pl solver.Plan) {
	t.Helper()
	cut := make([]int, len(req.Demands))
	for _, b := range pl.Boards() {
		for _, d := range b.Cuts {
			cut[d] += b.Count
		}
	}
	for d, dem := range req.Demands {
		if cut[d] != dem.Quantity {
			t.Errorf("demand %d cut %d times, want %d", d, cut[d], dem.Quantity)
		}
	}
}

func solve(t *testing.T, req solver.Request) solver.Plan {
	t.Helper()
	pl, err := solver.Solve(context.Background(), req)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	assertCovered(t, req, pl)
	return pl
}

func TestSolvePlans(t *testing.T) {
	type want struct {
		status    solver.Status
		boards    []solver.Board
		purchases []solver.Purchase
		cost      solver.Cents
		waste     string
	}
	zero := solver.Leftover{}
	cases := []struct {
		name string
		req  solver.Request
		want want
	}{
		{
			name: "no demands",
			req:  solver.Request{Stock: []solver.Stock{stock(t, "96", 100)}},
			want: want{status: solver.Exact, waste: "0"},
		},
		{
			name: "two parts share the long board without kerf",
			req: solver.Request{
				Demands: []solver.Demand{demand(t, "48", 2)},
				Stock:   []solver.Stock{stock(t, "96", 100), stock(t, "48", 60)},
			},
			want: want{
				status:    solver.Exact,
				boards:    []solver.Board{{Stock: 0, Count: 1, Cuts: []int{0, 0}, Leftover: zero}},
				purchases: []solver.Purchase{{Stock: 0, Count: 1}},
				cost:      100,
				waste:     "0",
			},
		},
		{
			name: "kerf forces two short boards",
			req: solver.Request{
				Demands: []solver.Demand{demand(t, "48", 2)},
				Stock:   []solver.Stock{stock(t, "96", 100), stock(t, "48", 60)},
				Kerf:    in(t, "0.125"),
			},
			want: want{
				status:    solver.Exact,
				boards:    []solver.Board{{Stock: 1, Count: 2, Cuts: []int{0}, Leftover: zero}},
				purchases: []solver.Purchase{{Stock: 1, Count: 2}},
				cost:      120,
				waste:     "0",
			},
		},
		{
			name: "cheaper exact-length stock wins",
			req: solver.Request{
				Demands: []solver.Demand{demand(t, "5", 1)},
				Stock:   []solver.Stock{stock(t, "10", 10), stock(t, "5", 8)},
			},
			want: want{
				status:    solver.Exact,
				boards:    []solver.Board{{Stock: 1, Count: 1, Cuts: []int{0}, Leftover: zero}},
				purchases: []solver.Purchase{{Stock: 1, Count: 1}},
				cost:      8,
				waste:     "0",
			},
		},
		{
			name: "leftover at min remnant is kept",
			req: solver.Request{
				Demands:    []solver.Demand{demand(t, "90", 1)},
				Stock:      []solver.Stock{stock(t, "100", 50)},
				MinRemnant: in(t, "10"),
			},
			want: want{
				status:    solver.Exact,
				boards:    []solver.Board{{Stock: 0, Count: 1, Cuts: []int{0}, Leftover: solver.Leftover{Length: in(t, "10"), Kept: true}}},
				purchases: []solver.Purchase{{Stock: 0, Count: 1}},
				cost:      50,
				waste:     "0",
			},
		},
		{
			name: "leftover below min remnant is waste",
			req: solver.Request{
				Demands:    []solver.Demand{demand(t, "90", 1)},
				Stock:      []solver.Stock{stock(t, "100", 50)},
				MinRemnant: in(t, "12"),
			},
			want: want{
				status:    solver.Exact,
				boards:    []solver.Board{{Stock: 0, Count: 1, Cuts: []int{0}, Leftover: solver.Leftover{Length: in(t, "10")}}},
				purchases: []solver.Purchase{{Stock: 0, Count: 1}},
				cost:      50,
				waste:     "10",
			},
		},
		{
			name: "zero min remnant keeps nothing",
			req: solver.Request{
				Demands: []solver.Demand{demand(t, "90", 1)},
				Stock:   []solver.Stock{stock(t, "100", 50)},
			},
			want: want{
				status:    solver.Exact,
				boards:    []solver.Board{{Stock: 0, Count: 1, Cuts: []int{0}, Leftover: solver.Leftover{Length: in(t, "10")}}},
				purchases: []solver.Purchase{{Stock: 0, Count: 1}},
				cost:      50,
				waste:     "10",
			},
		},
		{
			name: "equal lengths of two demands share a board in index order",
			req: solver.Request{
				Demands: []solver.Demand{demand(t, "10", 1), demand(t, "10", 1)},
				Stock:   []solver.Stock{stock(t, "20", 5)},
			},
			want: want{
				status:    solver.Exact,
				boards:    []solver.Board{{Stock: 0, Count: 1, Cuts: []int{0, 1}, Leftover: zero}},
				purchases: []solver.Purchase{{Stock: 0, Count: 1}},
				cost:      5,
				waste:     "0",
			},
		},
		{
			name: "same cost keeps the shorter leftover",
			req: solver.Request{
				Demands: []solver.Demand{demand(t, "6", 1)},
				Stock:   []solver.Stock{stock(t, "10", 10), stock(t, "8", 10)},
			},
			want: want{
				status:    solver.Exact,
				boards:    []solver.Board{{Stock: 1, Count: 1, Cuts: []int{0}, Leftover: solver.Leftover{Length: in(t, "2")}}},
				purchases: []solver.Purchase{{Stock: 1, Count: 1}},
				cost:      10,
				waste:     "2",
			},
		},
		{
			name: "same cost and waste buys fewer boards",
			req: solver.Request{
				Demands: []solver.Demand{demand(t, "6", 2)},
				Stock:   []solver.Stock{stock(t, "12", 10), stock(t, "6", 5)},
			},
			want: want{
				status:    solver.Exact,
				boards:    []solver.Board{{Stock: 0, Count: 1, Cuts: []int{0, 0}, Leftover: zero}},
				purchases: []solver.Purchase{{Stock: 0, Count: 1}},
				cost:      10,
				waste:     "0",
			},
		},
		{
			name: "one long board beats two cheaper short boards",
			req: solver.Request{
				Demands: []solver.Demand{demand(t, "6", 2)},
				Stock:   []solver.Stock{stock(t, "12", 10), stock(t, "6", 9)},
			},
			want: want{
				status:    solver.Exact,
				boards:    []solver.Board{{Stock: 0, Count: 1, Cuts: []int{0, 0}, Leftover: zero}},
				purchases: []solver.Purchase{{Stock: 0, Count: 1}},
				cost:      10,
				waste:     "0",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.Budget = time.Second
			pl := solve(t, tc.req)
			if pl.Status() != tc.want.status {
				t.Errorf("status %v, want %v", pl.Status(), tc.want.status)
			}
			if got := pl.Boards(); !reflect.DeepEqual(got, tc.want.boards) {
				t.Errorf("boards %+v, want %+v", got, tc.want.boards)
			}
			if got := pl.Purchases(); !reflect.DeepEqual(got, tc.want.purchases) {
				t.Errorf("purchases %+v, want %+v", got, tc.want.purchases)
			}
			if pl.Cost() != tc.want.cost {
				t.Errorf("cost %d, want %d", pl.Cost(), tc.want.cost)
			}
			if got := pl.Waste().String(); got != tc.want.waste {
				t.Errorf("waste %s, want %s", got, tc.want.waste)
			}
		})
	}
}

func TestSolveInfeasibleListsEveryDemand(t *testing.T) {
	req := solver.Request{
		Demands: []solver.Demand{demand(t, "100", 1), demand(t, "10", 1)},
		Stock:   []solver.Stock{stock(t, "96", 100)},
		Budget:  time.Second,
	}
	_, err := solver.Solve(context.Background(), req)
	var ie *solver.InfeasibleError
	if !errors.As(err, &ie) {
		t.Fatalf("error %v, want *InfeasibleError", err)
	}
	if !slices.Equal(ie.Demands, []int{0}) {
		t.Errorf("demands %v, want [0]", ie.Demands)
	}
}

func TestSolveInfeasibleListsEveryOverlongDemand(t *testing.T) {
	req := solver.Request{
		Demands: []solver.Demand{demand(t, "10", 1), demand(t, "40", 1), demand(t, "30", 1)},
		Stock:   []solver.Stock{stock(t, "20", 5)},
		Budget:  time.Second,
	}
	_, err := solver.Solve(context.Background(), req)
	var ie *solver.InfeasibleError
	if !errors.As(err, &ie) {
		t.Fatalf("error %v, want *InfeasibleError", err)
	}
	if !slices.Equal(ie.Demands, []int{1, 2}) {
		t.Errorf("demands %v, want [1 2]", ie.Demands)
	}
}

func TestSolveZeroBudgetReturnsSeed(t *testing.T) {
	req := solver.Request{
		Demands: []solver.Demand{demand(t, "5", 1)},
		Stock:   []solver.Stock{stock(t, "10", 10), stock(t, "5", 8)},
	}
	pl := solve(t, req)
	if pl.Status() != solver.Feasible {
		t.Errorf("status %v, want Feasible", pl.Status())
	}
	if pl.Cost() != 8 {
		t.Errorf("cost %d, want 8", pl.Cost())
	}
}

func TestSolveCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := solver.Request{
		Demands: []solver.Demand{demand(t, "5", 1)},
		Stock:   []solver.Stock{stock(t, "10", 10)},
		Budget:  time.Second,
	}
	pl, err := solver.Solve(ctx, req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v, want context.Canceled", err)
	}
	if !reflect.DeepEqual(pl, solver.Plan{}) {
		t.Errorf("plan %+v, want zero Plan", pl)
	}
}

func TestSolveFineStepOnLongBoard(t *testing.T) {
	req := solver.Request{
		Demands: []solver.Demand{demand(t, "0.015625", 1)},
		Stock:   []solver.Stock{stock(t, "96", 7)},
		Budget:  2 * time.Second,
	}
	pl := solve(t, req)
	if pl.Status() != solver.Exact {
		t.Errorf("status %v, want Exact", pl.Status())
	}
	if pl.Cost() != 7 {
		t.Errorf("cost %d, want 7", pl.Cost())
	}
	if got := pl.Waste().String(); got != "95.984375" {
		t.Errorf("waste %s, want 95.984375", got)
	}
}

func bruteCost(parts, stockLen, stockPrice []int, kerf int) int {
	best := -1
	var boards [][]int
	var place func(j int)
	boardCost := func(b []int) int {
		used := kerf * (len(b) - 1)
		for _, p := range b {
			used += p
		}
		price := -1
		for s, l := range stockLen {
			if used <= l && (price < 0 || stockPrice[s] < price) {
				price = stockPrice[s]
			}
		}
		return price
	}
	place = func(j int) {
		if j == len(parts) {
			total := 0
			for _, b := range boards {
				c := boardCost(b)
				if c < 0 {
					return
				}
				total += c
			}
			if best < 0 || total < best {
				best = total
			}
			return
		}
		for i := range boards {
			boards[i] = append(boards[i], parts[j])
			place(j + 1)
			boards[i] = boards[i][:len(boards[i])-1]
		}
		boards = append(boards, []int{parts[j]})
		place(j + 1)
		boards = boards[:len(boards)-1]
	}
	place(0)
	return best
}

func TestSolveMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for trial := range 300 {
		var req solver.Request
		var parts []int
		for range 1 + rng.IntN(3) {
			l, q := 1+rng.IntN(10), 1+rng.IntN(3)
			req.Demands = append(req.Demands, demand(t, strconv.Itoa(l), q))
			for range q {
				parts = append(parts, l)
			}
		}
		var stockLen, stockPrice []int
		for _, l := range rng.Perm(14)[:1+rng.IntN(3)] {
			l += 10
			price := 1 + rng.IntN(20)
			stockLen, stockPrice = append(stockLen, l), append(stockPrice, price)
			req.Stock = append(req.Stock, stock(t, strconv.Itoa(l), solver.Cents(price)))
		}
		kerf := rng.IntN(2)
		req.Kerf = in(t, strconv.Itoa(kerf))
		req.Budget = time.Second

		pl := solve(t, req)
		want := bruteCost(parts, stockLen, stockPrice, kerf)
		if pl.Status() != solver.Exact || int(pl.Cost()) != want {
			t.Fatalf("trial %d: %+v gave status %v cost %d, want Exact cost %d", trial, req, pl.Status(), pl.Cost(), want)
		}
	}
}

func TestSolveInputErrors(t *testing.T) {
	ok := func() solver.Request {
		return solver.Request{
			Demands: []solver.Demand{demand(t, "10", 1)},
			Stock:   []solver.Stock{stock(t, "96", 100)},
			Budget:  time.Second,
		}
	}
	cases := []struct {
		name   string
		edit   func(*solver.Request)
		reason solver.Reason
		index  int
	}{
		{"demand length zero", func(r *solver.Request) { r.Demands[0].Length = solver.Length{} }, solver.DemandZeroLength, 0},
		{"demand quantity zero", func(r *solver.Request) { r.Demands[0].Quantity = 0 }, solver.DemandQuantityNotPositive, 0},
		{"stock length zero", func(r *solver.Request) { r.Stock[0].Length = solver.Length{} }, solver.StockZeroLength, 0},
		{"stock price negative", func(r *solver.Request) { r.Stock[0].Price = -1 }, solver.StockPriceNegative, 0},
		{"stock length repeated", func(r *solver.Request) { r.Stock = append(r.Stock, stock(t, "96", 50)) }, solver.StockDuplicateLength, 1},
		{"budget negative", func(r *solver.Request) { r.Budget = -1 }, solver.BudgetNegative, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := ok()
			tc.edit(&req)
			_, err := solver.Solve(context.Background(), req)
			var ie *solver.InputError
			if !errors.As(err, &ie) {
				t.Fatalf("error %v, want *InputError", err)
			}
			if ie.Reason != tc.reason || ie.Index != tc.index {
				t.Errorf("reason %d index %d, want reason %d index %d", ie.Reason, ie.Index, tc.reason, tc.index)
			}
		})
	}
}
