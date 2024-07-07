package pipeline

import (
	"fmt"
)

type ErrInvalidArguments = error

func NewInvalidArgumentsError(content string) ErrInvalidArguments {
	return fmt.Errorf("invalid arguments at %q", content)
}

type ErrUnmarshal = error

func NewUnmarshalError(kind, content string) ErrUnmarshal {
	return fmt.Errorf("JSON parsing error in %s at %q", kind, content)
}

type ErrExpression = error

func NewExpressionError(kind, content string, err error) ErrExpression {
	return fmt.Errorf("failed to evaluate %s expression %q: %w", kind, content, err)
}

type ErrAggregation = error

func NewAggregationError(kind, content string, err error) ErrAggregation {
	return fmt.Errorf("failed to evaluate %s aggregation with expression %q: %w",
		kind, content, err)
}

type ErrInvalidObject = error

func NewInvalidObjectError(message string) ErrInvalidObject {
	return fmt.Errorf("invalid object: %s", message)
}