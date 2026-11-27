package sdk

import (
	"errors"
	"fmt"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var (
	ErrNotConnected         = errors.New("sdk: not connected")
	ErrAlreadyConnected     = errors.New("sdk: already connected")
	ErrClosed               = errors.New("sdk: client closed")
	ErrRunStreamClosed      = errors.New("sdk: run stream closed")
	ErrInvalidConfiguration = errors.New("sdk: invalid configuration")
	ErrConnectionClosed     = errors.New("sdk: connection closed")
	ErrAuthFailed           = errors.New("sdk: authentication failed")
	ErrRequestTimeout       = errors.New("sdk: request timeout")
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

// APIError is the typed error returned for HTTP API failures and WebSocket
// error packets. It never wraps internal protobuf or Kratos types.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
	Retryable  bool
	RetryAfter time.Duration
	RequestID  string
}

func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Code != "" {
		return fmt.Sprintf("sdk: %s (%d): %s", e.Code, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("sdk: http %d: %s", e.StatusCode, e.Message)
}

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

func apiErrorFromKratos(err error) error {
	if err == nil {
		return nil
	}
	var kerr *kerrors.Error
	if !errors.As(err, &kerr) {
		return err
	}
	status := int(kerr.Code)
	if status < 100 || status > 599 {
		status = 500
	}
	apiErr := &APIError{
		StatusCode: status, Code: kerr.Reason, Message: kerr.Message,
		Retryable: status == 408 || status == 429 || status >= 500,
	}
	if apiErr.Code == "" {
		apiErr.Code = fmt.Sprintf("HTTP_%d", status)
	}
	if retry := kerr.Metadata["retry-after-ms"]; retry != "" {
		var ms int64
		if _, scanErr := fmt.Sscanf(retry, "%d", &ms); scanErr == nil && ms > 0 {
			apiErr.RetryAfter = time.Duration(ms) * time.Millisecond
		}
	}
	return apiErr
}
