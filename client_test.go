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
	c, cap := newStub(t, 200, `[{"currency":"USDT","available":"1","locked":"0"}]`)
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
	c, cap := newStub(t, 200, `{"quote":"USDT","total":"0","lines":[]}`)
	if _, err := c.Balances.Summary(context.Background(), "USDT"); err != nil {
		t.Fatal(err)
	}
	if cap.path != "/v1/balances/summary?quote=USDT" {
		t.Fatalf("query not built: %q", cap.path)
	}

	c2, cap2 := newStub(t, 201, `{"token":"inv_1"}`)
	_, err := c2.Invoices.Create(context.Background(), InvoiceParams{
		Reference: "r1", Amount: Money{Amount: "1.00", Currency: "USDT"}, Chain: "MATIC",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cap2.method != http.MethodPost || !strings.Contains(cap2.body, `"reference":"r1"`) || !strings.Contains(cap2.body, `"chain":"MATIC"`) {
		t.Fatalf("bad POST body: %s", cap2.body)
	}
	if cap2.headers.Get("Content-Type") != "application/json" {
		t.Fatal("missing content-type")
	}
}

func TestErrorMapping(t *testing.T) {
	c, _ := newStub(t, 403, `{"code":"forbidden","title":"requires invoices:read"}`)
	_, err := c.Invoices.List(context.Background(), PageQuery{})
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
		io.WriteString(w, "[]")
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

func TestTransactionsQueryParams(t *testing.T) {
	c, cap := newStub(t, 200, `{"entries":[]}`)
	_, err := c.Transactions.List(context.Background(), TransactionsQuery{From: 1000, To: 2000, Limit: 50, Offset: 100, Currency: "USDT"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"from=1000", "to=2000", "limit=50", "offset=100", "currency=USDT"} {
		if !strings.Contains(cap.path, want) {
			t.Fatalf("missing %s in %q", want, cap.path)
		}
	}
	for _, bad := range []string{"startTime", "page=", "count="} {
		if strings.Contains(cap.path, bad) {
			t.Fatalf("unexpected %s in %q", bad, cap.path)
		}
	}
}

func TestPaginationCursor(t *testing.T) {
	c, cap := newStub(t, 200, `{"items":[{"token":"a"}],"nextCursor":"CUR2"}`)
	page, err := c.Invoices.List(context.Background(), PageQuery{Limit: 2, Before: "CUR1", Status: "OPEN"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"limit=2", "before=CUR1", "status=OPEN"} {
		if !strings.Contains(cap.path, want) {
			t.Fatalf("missing %s in %q", want, cap.path)
		}
	}
	if page.NextCursor == nil || *page.NextCursor != "CUR2" {
		t.Fatalf("nextCursor not decoded: %+v", page.NextCursor)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(page.Items))
	}
}
