package absolutepay

import (
	"errors"
	"fmt"
)

// Error is returned when the API responds with a non-2xx status. It carries the
// platform's problem+json fields.
type Error struct {
	Status    int    // HTTP status code (0 for a transport/network failure)
	Code      string // stable, machine-readable code (branch on this)
	Title     string // human-readable summary
	Detail    string // optional extra context
	RequestID string // x-request-id, include it when reporting a problem
}

func (e *Error) Error() string {
	if e.Title != "" {
		return fmt.Sprintf("absolutepay: %d %s (%s)", e.Status, e.Title, e.Code)
	}
	return fmt.Sprintf("absolutepay: %d %s", e.Status, e.Code)
}

// IsAuth reports a 401/403 — bad/insufficient credentials, missing scope, or an
// invalid request signature.
func (e *Error) IsAuth() bool { return e.Status == 401 || e.Status == 403 }

// IsRateLimited reports a 429 — back off and retry after a moment.
func (e *Error) IsRateLimited() bool { return e.Status == 429 }

// ErrInvalidSignature is returned by ConstructEvent when a webhook fails
// signature or freshness verification.
var ErrInvalidSignature = errors.New("absolutepay: invalid webhook signature")
