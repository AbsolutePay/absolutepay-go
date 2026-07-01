package absolutepay

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"testing"
)

func TestCanonicalRequest(t *testing.T) {
	got := canonicalRequest("get", "/v1/balances", "1700000000000", "nonce-1", "")
	// sha256("") = e3b0c442...b855
	want := "GET\n/v1/balances\n1700000000000\nnonce-1\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got != want {
		t.Fatalf("canonical mismatch:\n got %q\nwant %q", got, want)
	}
}

func TestSignRequestHeadersAndVerify(t *testing.T) {
	const secret = "apisign_test"
	h := signRequest(secret, "POST", "/v1/refunds", `{"a":1}`)
	for _, k := range []string{"X-AbsolutePay-Timestamp", "X-AbsolutePay-Nonce", "X-AbsolutePay-Signature"} {
		if h[k] == "" {
			t.Fatalf("missing header %s", k)
		}
	}
	if len(h["X-AbsolutePay-Signature"]) != 128 { // hex SHA-512
		t.Fatalf("signature length = %d, want 128", len(h["X-AbsolutePay-Signature"]))
	}
	canon := canonicalRequest("POST", "/v1/refunds", h["X-AbsolutePay-Timestamp"], h["X-AbsolutePay-Nonce"], `{"a":1}`)
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(canon))
	if want := hex.EncodeToString(mac.Sum(nil)); h["X-AbsolutePay-Signature"] != want {
		t.Fatalf("signature mismatch")
	}
}

func TestNonceUnique(t *testing.T) {
	if newNonce() == newNonce() {
		t.Fatal("nonce should be unique per call")
	}
}
