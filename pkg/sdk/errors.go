package sdk

import (
	"errors"

	rt "nonoka-im/pkg/sdk/internal/realtime"
)

var (
	ErrNotConnected         = rt.ErrNotConnected
	ErrAlreadyConnected     = rt.ErrAlreadyConnected
	ErrClosed               = rt.ErrClosed
	ErrRunStreamClosed      = errors.New("sdk: run stream closed")
	ErrInvalidConfiguration = errors.New("sdk: invalid configuration")
	ErrConnectionClosed     = errors.New("sdk: connection closed")
	ErrAuthFailed           = rt.ErrAuthFailed
	ErrRequestTimeout       = rt.ErrRequestTimeout
	ErrSearchUnavailable    = errors.New("sdk: search is not configured")
)

// SendError preserves the client message ID generated for a failed send so a
// caller can retry idempotently instead of generating a second message.
type SendError struct {
	ClientMessageID string
	Err             error
}

func (e *SendError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return "sdk: send failed for client message " + e.ClientMessageID + ": " + e.Err.Error()
}

func (e *SendError) Unwrap() error { return e.Err }

// IsCode reports whether err is an APIError with the given code.
func IsCode(err error, code string) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == code
}

// IsStatus reports whether err is an APIError with the given HTTP status.
func IsStatus(err error, status int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}

func IsNotFound(err error) bool     { return IsStatus(err, 404) }
func IsForbidden(err error) bool    { return IsStatus(err, 403) }
func IsUnauthorized(err error) bool { return IsStatus(err, 401) }
func IsConflict(err error) bool     { return IsStatus(err, 409) }
func IsRateLimited(err error) bool  { return IsStatus(err, 429) }

// Retryable reports whether the error is safe to retry at the transport layer.
func Retryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable
	}
	return false
}
