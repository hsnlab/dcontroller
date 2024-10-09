package pipeline

import (
	"fmt"
)

type ErrAggregation = error

func NewAggregationError(err error) ErrAggregation {
	return fmt.Errorf("failed to evaluate aggregation expression: %w", err)
}

type ErrJoin = error

func NewJoinError(err error) ErrJoin {
	return fmt.Errorf("failed to evaluate join expression: %w", err)
}

type ErrInvalidObject = error

func NewInvalidObjectError(message string) ErrInvalidObject {
	return fmt.Errorf("invalid object: %s", message)
}