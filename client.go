// Package absolutepay is the official Go client for the AbsolutePay API.
//
// It signs every request from an app key automatically (HMAC-SHA512) and verifies
// inbound webhooks. Server-side only — your API key and signing secret must never
// reach a browser. Zero third-party dependencies (standard library only).
package absolutepay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The only public API origins. Anything else must be passed via WithBaseURL.
const (
	ProductionBaseURL = "https://api.absolutepay.io"
	SandboxBaseURL    = "https://sandbox-api.absolutepay.io"
)

// Client is the AbsolutePay API client. Construct it once with New and reuse it;
// each resource service hangs off it.
type Client struct {
	apiKey        string
	signingSecret string
	baseURL       string
	httpClient    *http.Client

	Balances      *BalancesService
	Fees          *FeesService
	Payments      *PaymentsService
	Payouts       *PayoutsService
	Refunds       *RefundsService
	Conversions   *ConversionsService
	Invoices      *InvoicesService
	Subscriptions *SubscriptionsService
	GiftCards     *GiftCardsService
	OffRamp       *OffRampService
	Transactions  *TransactionsService
}

// Option customizes a Client.
type Option func(*Client)

// WithSigningSecret sets the request signing secret (apisign_...). Required for
// app keys — when set, every request is HMAC-signed.
func WithSigningSecret(secret string) Option { return func(c *Client) { c.signingSecret = secret } }

// WithSandbox targets the public sandbox host instead of production. Ignored when
// WithBaseURL is also set.
func WithSandbox(sandbox bool) Option {
	return func(c *Client) {
		if sandbox && c.baseURL == "" {
			c.baseURL = SandboxBaseURL
		}
	}
}

// WithBaseURL overrides the API origin entirely (takes precedence over WithSandbox).
func WithBaseURL(baseURL string) Option { return func(c *Client) { c.baseURL = baseURL } }

// WithHTTPClient sets a custom *http.Client (timeouts, transport, proxy).
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.httpClient = hc } }

// WithTimeout sets the per-request timeout on the default HTTP client.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient = &http.Client{Timeout: d} }
}

// New builds a Client. apiKey is required (ap_live_ / ap_test_). Options can set
// the signing secret, sandbox/base URL, and HTTP client.
func New(apiKey string, opts ...Option) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("absolutepay: apiKey is required")
	}
	c := &Client{apiKey: apiKey, httpClient: &http.Client{Timeout: 30 * time.Second}}
	// WithBaseURL / WithSandbox interplay: apply base URL first if present, so
	// WithSandbox's "only if unset" guard behaves regardless of option order.
	for _, o := range opts {
		if o != nil {
			o(c)
		}
	}
	if c.baseURL == "" {
		c.baseURL = ProductionBaseURL
	}
	c.baseURL = strings.TrimRight(c.baseURL, "/")
	// Never send the API key + signing headers over cleartext. https only, except localhost.
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("absolutepay: invalid baseURL %q: %w", c.baseURL, err)
	}
	if u.Scheme != "https" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" {
		return nil, fmt.Errorf("absolutepay: baseURL must use https (got %q); http is allowed only for localhost", c.baseURL)
	}

	c.Balances = &BalancesService{c}
	c.Fees = &FeesService{c}
	c.Payments = &PaymentsService{c}
	c.Payouts = &PayoutsService{c}
	c.Refunds = &RefundsService{c}
	c.Conversions = &ConversionsService{c}
	c.Invoices = &InvoicesService{c, &PublicInvoicesService{c}}
	c.Subscriptions = &SubscriptionsService{c}
	c.GiftCards = &GiftCardsService{c}
	c.OffRamp = &OffRampService{c}
	c.Transactions = &TransactionsService{c}
	return c, nil
}

// do performs a request. path is the path+query. body is JSON-marshaled (nil for
// none). extra headers are merged AFTER signing (not part of the canonical string).
// out, if non-nil, receives the decoded JSON response.
func (c *Client) do(ctx context.Context, method, path string, body any, extra map[string]string, out any) error {
	var bodyStr string
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyStr = string(b)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.signingSecret != "" {
		for k, v := range signRequest(c.signingSecret, method, path, bodyStr) {
			req.Header.Set(k, v)
		}
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &Error{Code: "network_error", Title: err.Error()}
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		e := &Error{
			Status:    resp.StatusCode,
			Code:      "error",
			Title:     "HTTP " + strconv.Itoa(resp.StatusCode),
			RequestID: resp.Header.Get("x-request-id"),
		}
		var p struct {
			Code   string `json:"code"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		}
		if json.Unmarshal(data, &p) == nil {
			if p.Code != "" {
				e.Code = p.Code
			}
			if p.Title != "" {
				e.Title = p.Title
			}
			e.Detail = p.Detail
		} else if len(data) > 0 {
			e.Detail = string(data[:min(len(data), 300)])
		}
		return e
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// qs builds a "?a=1&b=2" query string from defined values (skips empty).
func qs(params map[string]string) string {
	v := url.Values{}
	for k, val := range params {
		if val != "" {
			v.Set(k, val)
		}
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

// pageQuery renders a PageQuery to query params (drops zero/empty fields).
func pageQuery(p PageQuery) string {
	m := map[string]string{"before": p.Before, "status": p.Status}
	if p.Limit > 0 {
		m["limit"] = strconv.Itoa(p.Limit)
	}
	return qs(m)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
