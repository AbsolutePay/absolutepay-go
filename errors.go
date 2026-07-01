package absolutepay

import (
	"errors"
	"fmt"
)

// Error is returned when the API responds with a non-2xx status (or a transport
// failure occurs). It carries the platform's problem+json fields. Every resource
// method returns this concrete type via the error interface; type-assert to *Error
// to inspect the fields or call IsAuth / IsRateLimited.
type Error struct {
	// Status is the HTTP status code, or 0 for a transport/network failure.
	Status int
	// Code is the stable, machine-readable error code; branch on this in your code.
	Code string
	// Title is a short human-readable summary of the problem.
	Title string
	// Detail is optional extra context about the specific failure (may be empty).
	Detail string
	// RequestID is the server's x-request-id; include it when reporting a problem.
	RequestID string
}

// Error implements the error interface, formatting the status, title, and code.
func (e *Error) Error() string {
	if e.Title != "" {
		return fmt.Sprintf("absolutepay: %d %s (%s)", e.Status, e.Title, e.Code)
	}
	return fmt.Sprintf("absolutepay: %d %s", e.Status, e.Code)
}

// IsAuth reports whether the error is a 401 or 403 — bad or insufficient
// credentials, a missing scope, or an invalid request signature.
func (e *Error) IsAuth() bool { return e.Status == 401 || e.Status == 403 }

// IsRateLimited reports whether the error is a 429 — you are being throttled; back
// off and retry after a moment.
func (e *Error) IsRateLimited() bool { return e.Status == 429 }

// ErrInvalidSignature is returned by ConstructEvent (and reported by
// VerifySignature) when a webhook fails signature or freshness verification.
var ErrInvalidSignature = errors.New("absolutepay: invalid webhook signature")
