package domain

// Page represents pagination parameters.
type Page struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// PageResult represents a paginated result set.
type PageResult[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  Page  `json:"page"`
}

// Cursor represents cursor-based pagination.
type Cursor string

// CursorPage represents cursor-based pagination parameters.
type CursorPage struct {
	Limit int    `json:"limit"`
	Cursor Cursor `json:"cursor,omitempty"`
}

// CursorPageResult represents a cursor-based paginated result set.
type CursorPageResult[T any] struct {
	Items    []T     `json:"items"`
	NextCursor Cursor `json:"next_cursor,omitempty"`
	HasMore  bool    `json:"has_more"`
}
