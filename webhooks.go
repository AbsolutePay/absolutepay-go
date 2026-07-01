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

// Event is a delivered webhook callback: the JSON the platform POSTs to your
// configured callback URL. Type identifies the event; Data is the event-specific
// payload as raw JSON that you unmarshal into the shape you expect.
//
// Known Type values include: "payment.succeeded", "charge.refunded",
// "payout.settled", "payout.partial", and "payout.failed".
type Event struct {
	// ID is the unique event identifier (use it to de-duplicate deliveries).
	ID string `json:"id"`
	// Type is the event name, e.g. "payment.succeeded".
	Type string `json:"type"`
	// Data is the event-specific payload as raw JSON; unmarshal it into your target type.
	Data json.RawMessage `json:"data"`
}

// DefaultWebhookTolerance is the default freshness window enforced on webhook
// timestamps (replay defense): a callback older than this is rejected.
const DefaultWebhookTolerance = 5 * time.Minute

// VerifySignature reports whether the HMAC-SHA512 of "{timestamp}.{rawBody}" keyed
// by secret equals signature, compared in constant time. secret is your app's
// callback secret (whsec_...); rawBody is the exact request body bytes; timestamp
// and signature come from the X-AbsolutePay-Timestamp and X-AbsolutePay-Signature
// headers. It returns false if any input is empty or the signatures differ. This is
// the low-level check; most callers use ConstructEvent, which also enforces freshness.
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

// EventOption customizes ConstructEvent (currently the freshness tolerance).
type EventOption func(*eventOptions)

// WithTolerance sets the freshness window used for replay defense. d is the maximum
// allowed age of a webhook timestamp; pass WithTolerance(0) to disable the freshness
// check entirely (signature is still verified). Defaults to DefaultWebhookTolerance.
func WithTolerance(d time.Duration) EventOption {
	return func(o *eventOptions) { o.tolerance = d }
}

// ConstructEvent verifies a webhook callback's signature and freshness, then parses
// and returns the Event. Pass the RAW request body bytes (not a re-encoded copy),
// the request headers, and your app's callback secret (whsec_...). By default it
// rejects callbacks whose timestamp is more than DefaultWebhookTolerance old; tune
// or disable that with WithTolerance. It returns ErrInvalidSignature if the
// signature or freshness check fails, or a JSON error if the body cannot be parsed.
// Example (net/http handler):
//
//	body, _ := io.ReadAll(r.Body)
//	evt, err := absolutepay.ConstructEvent(body, r.Header, "whsec_...")
//	if err != nil {
//		http.Error(w, "bad signature", http.StatusBadRequest)
//		return
//	}
//	switch evt.Type {
//	case "payment.succeeded":
//		// json.Unmarshal(evt.Data, &payload)
//	}
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
