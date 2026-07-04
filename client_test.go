package absolutepay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type capture struct {
	method  string
	path    string // path + query
	headers http.Header
	body    string
}

// newStub spins an httptest server (127.0.0.1 → allowed over http) and a client
// pointed at it. It records the last request into *capture.
func newStub(t *testing.T, status int, body string) (*Client, *capture) {
	t.Helper()
	cap := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.method = r.Method
		cap.path = r.URL.RequestURI()
		cap.headers = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		cap.body = string(b)
		w.Header().Set("x-request-id", "req_1")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	c, err := New("ap_live_x", WithSigningSecret("apisign_x"), WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, cap
}

func TestNewRequiresAPIKey(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("empty apiKey should error")
	}
}

func TestNewRejectsCleartext(t *testing.T) {
	if _, err := New("k", WithBaseURL("http://api.evil.com")); err == nil {
		t.Fatal("cleartext non-localhost baseURL should error")
	}
	if _, err := New("k", WithBaseURL("http://localhost:3000")); err != nil {
		t.Fatalf("localhost http should be allowed: %v", err)
	}
	if _, err := New("k", WithBaseURL("https://api.test")); err != nil {
		t.Fatalf("https should be allowed: %v", err)
	}
}

func TestBaseURLResolution(t *testing.T) {
	c, _ := New("k")
	if c.baseURL != ProductionBaseURL {
		t.Fatalf("default = %q, want production", c.baseURL)
	}
	c, _ = New("k", WithSandbox(true))
	if c.baseURL != SandboxBaseURL {
		t.Fatalf("sandbox = %q, want sandbox", c.baseURL)
	}
	c, _ = New("k", WithSandbox(true), WithBaseURL("https://api.test"))
	if c.baseURL != "https://api.test" {
		t.Fatalf("baseURL should win over sandbox, got %q", c.baseURL)
	}
}

func TestSignsAndSendsBearer(t *testing.T) {
	c, cap := newStub(t, 200, `{"items":[{"currency":"USDT","available":"1","locked":"0"}],"nextCursor":null}`)
	if _, err := c.Balances.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cap.path != "/v1/balances" || cap.method != http.MethodGet {
		t.Fatalf("got %s %s", cap.method, cap.path)
	}
	if cap.headers.Get("Authorization") != "Bearer ap_live_x" {
		t.Fatalf("bad auth header: %q", cap.headers.Get("Authorization"))
	}
	if cap.headers.Get("X-AbsolutePay-Signature") == "" || cap.headers.Get("X-AbsolutePay-Nonce") == "" {
		t.Fatal("missing signature headers")
	}
}

func TestQueryAndPostBody(t *testing.T) {
	c, cap := newStub(t, 200, `{"items":[{"currency":"USDT","available":"1","locked":"0"}],"nextCursor":null}`)
	if _, err := c.Balances.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cap.path != "/v1/balances" {
		t.Fatalf("path not built: %q", cap.path)
	}

	c2, cap2 := newStub(t, 201, `{"token":"inv_1"}`)
	_, err := c2.Invoices.Create(context.Background(), InvoiceParams{
		Reference: "r1", Amount: Money{Amount: "1.00", Currency: "USDT"}, Chain: "MATIC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cap2.method != http.MethodPost || cap2.path != "/v1/invoices" || !strings.Contains(cap2.body, `"reference":"r1"`) || !strings.Contains(cap2.body, `"chain":"MATIC"`) {
		t.Fatalf("bad POST body: %s %s %s", cap2.method, cap2.path, cap2.body)
	}
	if cap2.headers.Get("Content-Type") != "application/json" {
		t.Fatal("missing content-type")
	}
}

func TestCheckoutRedirectURL(t *testing.T) {
	// Set: redirectUrl is marshaled into the POST body, and no chain is sent.
	c, cap := newStub(t, 201, `{"token":"chk_1","checkoutUrl":"https://pay.example/x"}`)
	_, err := c.Checkouts.Create(context.Background(), CheckoutParams{
		Reference:   "r1",
		Amount:      Money{Amount: "1.00", Currency: "USDT"},
		RedirectURL: "https://shop.example.com/thanks",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cap.path != "/v1/checkouts" {
		t.Fatalf("bad checkout path: %q", cap.path)
	}
	if !strings.Contains(cap.body, `"redirectUrl":"https://shop.example.com/thanks"`) {
		t.Fatalf("redirectUrl not in body: %s", cap.body)
	}
	if strings.Contains(cap.body, `"chain"`) {
		t.Fatalf("checkout must not carry a chain: %s", cap.body)
	}

	// Unset: omitempty drops the field entirely.
	c2, cap2 := newStub(t, 201, `{"token":"chk_2"}`)
	_, err = c2.Checkouts.Create(context.Background(), CheckoutParams{
		Reference: "r2", Amount: Money{Amount: "1.00", Currency: "USDT"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cap2.body, "redirectUrl") {
		t.Fatalf("redirectUrl should be omitted when empty: %s", cap2.body)
	}
}

func TestResourceUpdatePatch(t *testing.T) {
	c, cap := newStub(t, 200, `{"token":"chk_1","paused":true}`)
	paused := true
	_, err := c.Checkouts.Update(context.Background(), "chk_1", ResourceUpdate{Paused: &paused})
	if err != nil {
		t.Fatal(err)
	}
	if cap.method != http.MethodPatch || cap.path != "/v1/checkouts/chk_1" {
		t.Fatalf("bad PATCH request: %s %s", cap.method, cap.path)
	}
	if !strings.Contains(cap.body, `"paused":true`) {
		t.Fatalf("paused not in patch body: %s", cap.body)
	}
	// A nil pointer field must be omitted entirely.
	if strings.Contains(cap.body, "redirectUrl") || strings.Contains(cap.body, "expiresAt") {
		t.Fatalf("unset fields should be omitted: %s", cap.body)
	}
}

func TestDeleteVoidsCheckout(t *testing.T) {
	c, cap := newStub(t, 204, ``)
	if err := c.Checkouts.Delete(context.Background(), "chk_1"); err != nil {
		t.Fatal(err)
	}
	if cap.method != http.MethodDelete || cap.path != "/v1/checkouts/chk_1" {
		t.Fatalf("bad DELETE request: %s %s", cap.method, cap.path)
	}
}

func TestErrorMapping(t *testing.T) {
	c, _ := newStub(t, 403, `{"code":"forbidden","title":"requires invoices:read","detail":"scope missing"}`)
	_, err := c.Invoices.List(context.Background(), ListQuery{})
	var apErr *Error
	if !errors.As(err, &apErr) {
		t.Fatalf("want *Error, got %T", err)
	}
	if apErr.Status != 403 || apErr.Code != "forbidden" || !apErr.IsAuth() {
		t.Fatalf("bad error: %+v", apErr)
	}
	if apErr.RequestID != "req_1" {
		t.Fatalf("request id not captured: %q", apErr.RequestID)
	}
}

func TestIdempotencyKeyForwardedAfterSigning(t *testing.T) {
	c, cap := newStub(t, 202, `{"merchantBatchNo":"po_1"}`)
	_, err := c.Payouts.Create(context.Background(),
		[]PayoutItem{{RecipientAddress: "0xabc", Chain: "MATIC", Amount: Money{Amount: "1.00", Currency: "USDT"}}},
		WithIdempotencyKey("batch-001"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if cap.headers.Get("Idempotency-Key") != "batch-001" {
		t.Fatalf("idempotency key not forwarded: %q", cap.headers.Get("Idempotency-Key"))
	}
	if cap.headers.Get("X-AbsolutePay-Signature") == "" {
		t.Fatal("still must be signed")
	}
}

func TestIdempotencyKeyOmitted(t *testing.T) {
	c, cap := newStub(t, 202, `{}`)
	_, _ = c.Payouts.Create(context.Background(),
		[]PayoutItem{{RecipientAddress: "0xabc", Chain: "MATIC", Amount: Money{Amount: "1", Currency: "USDT"}}})
	if cap.headers.Get("Idempotency-Key") != "" {
		t.Fatal("idempotency key should be absent")
	}
}

func TestNoSigningWithoutSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-AbsolutePay-Signature") != "" {
			t.Error("must not sign without a secret")
		}
		io.WriteString(w, `{"items":[],"nextCursor":null}`)
	}))
	t.Cleanup(srv.Close)
	c, err := New("ap_test_x", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Balances.List(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLedgerQueryParams(t *testing.T) {
	c, cap := newStub(t, 200, `{"items":[],"nextCursor":null,"total":0}`)
	_, err := c.Refunds.List(context.Background(), LedgerQuery{From: 1000, To: 2000, Limit: 50, Currency: "USDT", Order: OrderDesc})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"from=1000", "to=2000", "limit=50", "currency=USDT", "order=desc"} {
		if !strings.Contains(cap.path, want) {
			t.Fatalf("missing %s in %q", want, cap.path)
		}
	}
	for _, bad := range []string{"offset=", "page=", "count="} {
		if strings.Contains(cap.path, bad) {
			t.Fatalf("unexpected %s in %q", bad, cap.path)
		}
	}
}

func TestPaginationCursor(t *testing.T) {
	c, cap := newStub(t, 200, `{"items":[{"token":"a"}],"nextCursor":"CUR2"}`)
	page, err := c.Invoices.List(context.Background(), ListQuery{Limit: 2, Before: "CUR1", Status: "OPEN", Order: OrderAsc})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"limit=2", "before=CUR1", "status=OPEN", "order=asc"} {
		if !strings.Contains(cap.path, want) {
			t.Fatalf("missing %s in %q", want, cap.path)
		}
	}
	if page.NextCursor != "CUR2" {
		t.Fatalf("nextCursor not decoded: %q", page.NextCursor)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(page.Items))
	}
}

func TestLastPageEmptyCursor(t *testing.T) {
	c, _ := newStub(t, 200, `{"items":[{"token":"a"}],"nextCursor":null}`)
	page, err := c.Checkouts.List(context.Background(), ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor != "" {
		t.Fatalf("null nextCursor should decode to empty string, got %q", page.NextCursor)
	}
}
