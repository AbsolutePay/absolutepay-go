package absolutepay

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func sign(secret, ts, body string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(ts + "." + body))
	return hex.EncodeToString(mac.Sum(nil))
}

func nowMS() string { return strconv.FormatInt(time.Now().UnixMilli(), 10) }

func TestVerifySignature(t *testing.T) {
	secret, body, ts := "whsec_x", `{"id":"e1"}`, nowMS()
	sig := sign(secret, ts, body)
	if !VerifySignature(secret, []byte(body), ts, sig) {
		t.Fatal("valid signature rejected")
	}
	if VerifySignature(secret, []byte(body), ts, "deadbeef") {
		t.Fatal("bad signature accepted")
	}
	if VerifySignature("", []byte(body), ts, sig) {
		t.Fatal("empty secret accepted")
	}
}

func TestConstructEvent(t *testing.T) {
	secret := "whsec_x"
	ts := nowMS()
	body := `{"id":"evt_1","type":"payment.succeeded","data":{"amount":"10"}}`
	h := http.Header{}
	h.Set("X-AbsolutePay-Timestamp", ts)
	h.Set("X-AbsolutePay-Signature", sign(secret, ts, body))

	e, err := ConstructEvent([]byte(body), h, secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Type != "payment.succeeded" || e.ID != "evt_1" {
		t.Fatalf("bad event: %+v", e)
	}
}

func TestConstructEventBadSignature(t *testing.T) {
	h := http.Header{}
	h.Set("X-AbsolutePay-Timestamp", nowMS())
	h.Set("X-AbsolutePay-Signature", "bad")
	if _, err := ConstructEvent([]byte("{}"), h, "whsec_x"); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("want ErrInvalidSignature, got %v", err)
	}
}

func TestConstructEventStaleTimestamp(t *testing.T) {
	secret := "whsec_x"
	ts := strconv.FormatInt(time.Now().Add(-10*time.Minute).UnixMilli(), 10)
	body := "{}"
	h := http.Header{}
	h.Set("X-AbsolutePay-Timestamp", ts)
	h.Set("X-AbsolutePay-Signature", sign(secret, ts, body))

	if _, err := ConstructEvent([]byte(body), h, secret); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("stale timestamp should be rejected, got %v", err)
	}
	// tolerance disabled -> accepted
	if _, err := ConstructEvent([]byte(body), h, secret, WithTolerance(0)); err != nil {
		t.Fatalf("tolerance=0 should accept, got %v", err)
	}
}
