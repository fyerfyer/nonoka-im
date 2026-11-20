package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// Page is the uniform list response: {items, nextCursor, hasMore}.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

type cursorPayload struct {
	Offset int `json:"offset"`
}

// Limit parses ?limit with a default and a hard maximum.
func Limit(r *http.Request, def, max int) int {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// Offset parses ?cursor into an offset. An empty cursor means offset 0.
func Offset(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor")
	}
	var p cursorPayload
	if err := json.Unmarshal(data, &p); err != nil || p.Offset < 0 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return p.Offset, nil
}

func EncodeCursor(offset int) string {
	data, _ := json.Marshal(cursorPayload{Offset: offset})
	return base64.RawURLEncoding.EncodeToString(data)
}

// NewPage builds a page from a fetched slice. Callers fetch limit+1 rows so
// hasMore is exact without a count query.
func NewPage[T any](items []T, offset, limit int, hasMore bool) Page[T] {
	next := ""
	if hasMore {
		next = EncodeCursor(offset + limit)
	}
	return Page[T]{Items: items, NextCursor: next, HasMore: hasMore}
}
