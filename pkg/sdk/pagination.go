package sdk

// Page is the shared cursor-paginated response shape.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

// ListOptions supplies cursor-based pagination.
type ListOptions struct {
	Cursor string
	Limit  int
}
