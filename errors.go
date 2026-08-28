package openhandle

import (
	"fmt"
	"time"
)

// Error is an error returned by the Openhandle API or transport runtime.
// Callers should branch on Code, never Message.
type Error struct {
	Code       string
	Message    string
	RequestID  string
	Retryable  bool
	RetryAfter time.Duration
	Status     int
	Details    map[string]any
	Cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.RequestID != "" {
		return fmt.Sprintf("openhandle: %s: %s (request %s)", e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("openhandle: %s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ReferenceError reports a locally invalid resource reference. It is returned
// before any network request is made.
type ReferenceError struct {
	Message string
	Cause   error
}

func (e *ReferenceError) Error() string { return "openhandle: " + e.Message }
func (e *ReferenceError) Unwrap() error { return e.Cause }

// ReferenceMismatchError reports a social URL for a different resource than
// the selector on which it was supplied.
type ReferenceMismatchError struct {
	ExpectedPlatform string
	ExpectedResource string
	ActualPlatform   string
	ActualResource   string
}

func (e *ReferenceMismatchError) Error() string {
	return fmt.Sprintf(
		"openhandle: expected a %s %s URL, received a %s %s URL",
		e.ExpectedPlatform,
		e.ExpectedResource,
		e.ActualPlatform,
		e.ActualResource,
	)
}
