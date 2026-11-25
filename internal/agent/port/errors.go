package port

import (
	"context"
	"errors"

	"nonoka-im/internal/agent/domain"
)

// ErrorClass classifies a failed run so the worker can decide between an
// immediate terminal state and a bounded retry. It is part of the port
// contract, not of the Eino adapter.
type ErrorClass string

const (
	ClassTransientInfra ErrorClass = "transient_infra"
	ClassModel          ErrorClass = "model_error"
	ClassTool           ErrorClass = "tool_error"
	ClassAuthDenied     ErrorClass = "auth_denied"
	ClassInvalidConfig  ErrorClass = "invalid_config"
	ClassTimeout        ErrorClass = "timeout"
	ClassCanceled       ErrorClass = "canceled"
	ClassInternal       ErrorClass = "internal"
	ClassQuota          ErrorClass = "quota"
	ClassBudget         ErrorClass = "budget"
	ClassRateLimit      ErrorClass = "rate_limit"
	// ClassSideEffect marks a run whose write tool already committed an
	// external effect. It is terminal and requires an explicit operator retry.
	ClassSideEffect ErrorClass = "side_effect_committed"
)

var (
	// ErrSummary marks a genuine summarization failure so the harness can
	// retry the run without summarization.
	ErrSummary = errors.New("summarization failed")
	// ErrModel marks model-provider failures.
	ErrModel = errors.New("model error")
	// ErrTool marks tool execution failures.
	ErrTool = errors.New("tool error")
	// ErrInvalidConfig marks a version/configuration problem that retrying
	// cannot fix.
	ErrInvalidConfig = errors.New("invalid agent configuration")
	// ErrAuthDenied marks an authorization failure that retrying cannot fix.
	ErrAuthDenied = errors.New("agent authorization denied")
)

// Error attaches a stable class to an underlying error.
type Error struct {
	Class ErrorClass
	Err   error
}

func (e *Error) Error() string { return string(e.Class) + ": " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Classify returns the error class and whether the run may be retried.
func Classify(err error) (ErrorClass, bool) {
	if err == nil {
		return ClassInternal, false
	}
	var classed *Error
	if errors.As(err, &classed) {
		return classed.Class, retryableClass(classed.Class)
	}
	switch {
	case errors.Is(err, context.Canceled):
		return ClassCanceled, false
	case errors.Is(err, context.DeadlineExceeded):
		return ClassTimeout, true
	case errors.As(err, new(*domain.QuotaError)):
		return ClassQuota, false
	case errors.As(err, new(*domain.BudgetError)):
		return ClassBudget, false
	case errors.As(err, new(*domain.RateLimitError)):
		return ClassRateLimit, false
	case errors.Is(err, ErrAuthDenied):
		return ClassAuthDenied, false
	case errors.Is(err, ErrInvalidConfig):
		return ClassInvalidConfig, false
	case errors.Is(err, ErrSummary), errors.Is(err, ErrModel):
		return ClassModel, true
	case errors.Is(err, ErrTool):
		return ClassTool, true
	default:
		return ClassInternal, false
	}
}

func retryableClass(class ErrorClass) bool {
	switch class {
	case ClassTransientInfra, ClassModel, ClassTool, ClassTimeout:
		return true
	default:
		// Unknown/internal/config/auth/cancel/side-effect failures must not be
		// retried automatically; adapters mark genuinely transient failures
		// explicitly.
		return false
	}
}
