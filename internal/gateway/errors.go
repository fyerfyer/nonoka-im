package gateway

import "errors"

var (
	ErrConnectionClosed   = errors.New("connection closed")
	ErrSendChannelFull    = errors.New("send channel full")
	ErrAuthRequired       = errors.New("authentication required")
	ErrInvalidToken       = errors.New("invalid token")
	ErrConnectionNotFound = errors.New("connection not found")
)
