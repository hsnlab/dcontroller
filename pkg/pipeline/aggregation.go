package pipeline

import (
	"fmt"
	"strings"

	opv1a1 "hsnlab/dcontroller/pkg/api/operator/v1alpha1"
	"hsnlab/dcontroller/pkg/cache"
)

var _ Evaluator = &Aggregation{}

const aggregateOp = "@aggregate"

// Aggregation is an operation that can be used to process, objects, or alter the shape of a list
// of objects in a view.
type Aggregation struct {
	*opv1a1.Aggregation
	engine Engine
}

// NewAggregation creates a new aggregation from a seralized representation.
func NewAggregation(engine Engine, config *opv1a1.Aggregation) *Aggregation {
	if config == nil {
		return nil
	}
	return &Aggregation{
		Aggregation: config,
		engine:      engine,
	}
}

func (a *Aggregation) String() string {
	ss := []string{}
	for _, e := range a.Expressions {
		ss = append(ss, e.String())
	}
	return fmt.Sprintf("%s:[%s]", aggregateOp, strings.Join(ss, ","))
}

// Evaluate processes an aggregation expression on the given delta.
func (a *Aggregation) Evaluate(delta cache.Delta) ([]cache.Delta, error) {
	eng := a.engine
	res, err := eng.EvaluateAggregation(a, delta)
	if err != nil {
		return nil, NewAggregationError(fmt.Errorf("aggregation error: %w", err))
	}

	return res, nil
}