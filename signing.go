package absolutepay

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// canonicalRequest builds the string that gets signed:
// METHOD\npath\ntimestamp\nnonce\nsha256hex(body). path is path+query as sent.
func canonicalRequest(method, path, ts, nonce, body string) string {
	sum := sha256.Sum256([]byte(body))
	return strings.ToUpper(method) + "\n" + path + "\n" + ts + "\n" + nonce + "\n" + hex.EncodeToString(sum[:])
}

// signRequest returns the three signature headers for one request. path MUST be
// the path+query exactly as sent; body the exact serialized body ("" for none).
func signRequest(secret, method, path, body string) map[string]string {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := newNonce()
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(canonicalRequest(method, path, ts, nonce, body)))
	return map[string]string{
		"X-AbsolutePay-Timestamp": ts,
		"X-AbsolutePay-Nonce":     nonce,
		"X-AbsolutePay-Signature": hex.EncodeToString(mac.Sum(nil)),
	}
}

func newNonce() string {
	b := make([]byte, 16)
	// crypto/rand.Read never returns a short read on success; on the (astronomically
	// unlikely) error we still return a usable, unique-enough value from the clock.
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}
