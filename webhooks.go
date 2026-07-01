package absolutepay

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Event is a delivered callback (the JSON the platform POSTs to your callback URL).
type Event struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// DefaultWebhookTolerance is the freshness window enforced on webhook timestamps
// (replay defense).
const DefaultWebhookTolerance = 5 * time.Minute

// VerifySignature reports whether HMAC-SHA512 over "{timestamp}.{rawBody}" with
// secret matches signature (constant-time).
func VerifySignature(secret string, rawBody []byte, timestamp, signature string) bool {
	if secret == "" || timestamp == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// eventOptions holds ConstructEvent configuration.
type eventOptions struct{ tolerance time.Duration }

// EventOption customizes ConstructEvent.
type EventOption func(*eventOptions)

// WithTolerance sets the freshness window (replay defense). Pass 0 to disable it.
func WithTolerance(d time.Duration) EventOption {
	return func(o *eventOptions) { o.tolerance = d }
}

// ConstructEvent verifies a callback's signature and freshness and returns the
// parsed event. Pass the RAW request body, the request headers, and your app's
// callback secret (whsec_...). Returns ErrInvalidSignature on any failure.
func ConstructEvent(rawBody []byte, headers http.Header, secret string, opts ...EventOption) (*Event, error) {
	o := eventOptions{tolerance: DefaultWebhookTolerance}
	for _, opt := range opts {
		opt(&o)
	}
	ts := headers.Get("X-AbsolutePay-Timestamp")
	sig := headers.Get("X-AbsolutePay-Signature")
	if !VerifySignature(secret, rawBody, ts, sig) {
		return nil, ErrInvalidSignature
	}
	if o.tolerance > 0 {
		ms, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			return nil, ErrInvalidSignature
		}
		age := time.Since(time.UnixMilli(ms))
		if age < 0 {
			age = -age
		}
		if age > o.tolerance {
			return nil, ErrInvalidSignature
		}
	}
	var e Event
	if err := json.Unmarshal(rawBody, &e); err != nil {
		return nil, err
	}
	return &e, nil
}
