package openhandle

import (
	"context"
	"errors"
	"time"
)

// Billing contains authoritative accounting metadata returned in headers.
// Monetary values remain decimal strings to avoid precision loss.
type Billing struct {
	Cost           string
	DatasetVersion string
	Disposition    string
	Environment    string
	ListPrice      string
}

// ResponseMetadata is shared by singular responses and pages.
type ResponseMetadata struct {
	Platform   Platform  `json:"platform"`
	Resource   Resource  `json:"resource"`
	CapturedAt time.Time `json:"captured_at"`
	Source     string    `json:"source"`
	RequestID  string    `json:"-"`
	Billing    Billing   `json:"-"`
}

func (m *ResponseMetadata) setResponseMetadata(requestID string, billing Billing) {
	m.RequestID = requestID
	m.Billing = billing
}

type responseMetadataSetter interface {
	setResponseMetadata(string, Billing)
}

// PageMeta contains the opaque pagination cursor returned by the API.
type PageMeta struct {
	Cursors PageCursors `json:"cursors"`
}

type PageCursors struct {
	Next *string `json:"next"`
}

// Page is a typed page returned by a list or search operation.
type Page[T any] struct {
	ResponseMetadata
	Data []T      `json:"data"`
	Meta PageMeta `json:"meta"`
	next func(context.Context) (*Page[T], error)
}

// Next returns the following page, or nil without a request at the end.
func (p *Page[T]) Next(ctx context.Context) (*Page[T], error) {
	if p == nil || p.Meta.Cursors.Next == nil || *p.Meta.Cursors.Next == "" || p.next == nil {
		return nil, nil
	}
	return p.next(ctx)
}

// HasNextPage reports whether the API returned another opaque cursor.
func (p *Page[T]) HasNextPage() bool {
	return p != nil && p.Meta.Cursors.Next != nil && *p.Meta.Cursors.Next != ""
}

// NextCursor returns the opaque next cursor, or the empty string at the end.
func (p *Page[T]) NextCursor() string {
	if !p.HasNextPage() {
		return ""
	}
	return *p.Meta.Cursors.Next
}

// Iterator lazily retrieves one page at a time.
type Iterator[T any] struct {
	fetch   func(context.Context) (*Page[T], error)
	page    *Page[T]
	index   int
	value   T
	err     error
	started bool
}

func newIterator[T any](fetch func(context.Context) (*Page[T], error)) *Iterator[T] {
	return &Iterator[T]{fetch: fetch}
}

// Next advances to the next item, requesting a page only when necessary.
func (i *Iterator[T]) Next(ctx context.Context) bool {
	if i == nil || i.err != nil {
		return false
	}
	for {
		if i.page != nil && i.index < len(i.page.Data) {
			i.value = i.page.Data[i.index]
			i.index++
			return true
		}
		var page *Page[T]
		if !i.started {
			i.started = true
			page, i.err = i.fetch(ctx)
		} else {
			page, i.err = i.page.Next(ctx)
		}
		if i.err != nil || page == nil {
			return false
		}
		i.page = page
		i.index = 0
		if len(page.Data) == 0 && !page.HasNextPage() {
			return false
		}
	}
}

// Value returns the current item. Call it only after Next reports true.
func (i *Iterator[T]) Value() T { return i.value }

// Err returns the first error encountered while iterating.
func (i *Iterator[T]) Err() error {
	if i == nil {
		return errors.New("openhandle: nil iterator")
	}
	return i.err
}
