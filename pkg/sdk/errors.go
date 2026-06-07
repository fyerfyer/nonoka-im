package sdk

import "errors"

var (
	// ErrNotConnected indicates the client is not connected to the gateway.
	ErrNotConnected = errors.New("sdk: not connected")

	// ErrAlreadyConnected indicates the client is already connected.
	ErrAlreadyConnected = errors.New("sdk: already connected")

	// ErrAuthFailed indicates authentication failed.
	ErrAuthFailed = errors.New("sdk: authentication failed")

	// ErrRequestTimeout indicates a request timed out waiting for response.
	ErrRequestTimeout = errors.New("sdk: request timeout")

	// ErrConnectionClosed indicates the underlying WebSocket connection was closed.
	ErrConnectionClosed = errors.New("sdk: connection closed")

	// ErrInvalidState indicates the client is in an invalid state for the operation.
	ErrInvalidState = errors.New("sdk: invalid state")

	// ErrServerError indicates the server returned an error response.
	ErrServerError = errors.New("sdk: server error")
)
