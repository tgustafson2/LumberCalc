package solver

import (
	"errors"
	"fmt"
)

// ErrLength means the text is not a length this package can hold.
// Zero is a length. A negative value, a value finer than one microinch,
// and a value that overflows int64 microinches are not.
var ErrLength = errors.New("solver: length")

// Reason names one rule that a Request broke. The reason also names the field.
type Reason int

const (
	reasonUnknown Reason = iota
	DemandZeroLength
	DemandQuantityNotPositive
	StockZeroLength
	StockPriceNegative
	StockDuplicateLength
	BudgetNegative
)

// InputError reports a Request that Solve cannot accept.
type InputError struct {
	Reason Reason
	// Index is the position in Request.Demands or Request.Stock. It is -1
	// for BudgetNegative.
	Index int
}

func (e *InputError) Error() string {
	return fmt.Sprintf("solver: invalid input: reason %d at index %d", e.Reason, e.Index)
}

// InfeasibleError reports demands that are longer than every stock length.
// An empty Request.Stock with any demand makes every demand infeasible.
type InfeasibleError struct {
	// Demands holds indices into Request.Demands, in ascending order.
	Demands []int
}

func (e *InfeasibleError) Error() string {
	return fmt.Sprintf("solver: %d demands fit on no stock length", len(e.Demands))
}
