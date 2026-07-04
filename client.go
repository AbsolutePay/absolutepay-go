// Package absolutepay is the official Go client for the AbsolutePay API.
//
// It signs every request from an app key automatically (HMAC-SHA512) and verifies
// inbound webhooks. Server-side only — your API key and signing secret must never
// reach a browser. Zero third-party dependencies (standard library only).
//
// Typical use:
//
//	ap, err := absolutepay.New("ap_live_...", absolutepay.WithSigningSecret("apisign_..."))
//	if err != nil {
//		log.Fatal(err)
//	}
//	balances, err := ap.Balances.List(ctx)
//
// Every list of scopes required by a call is documented on the service type that
// exposes it (for example BalancesService needs balances:read).
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

// Base URLs for the public API. Pass any other origin through WithBaseURL.
const (
	// ProductionBaseURL is the live API origin (real funds).
	ProductionBaseURL = "https://api.absolutepay.io"
	// SandboxBaseURL is the public sandbox API origin (mock funds); select it with WithSandbox.
	SandboxBaseURL = "https://sandbox-api.absolutepay.io"
)

// Client is the AbsolutePay API client. Construct it once with New and reuse it
// across goroutines; each resource service hangs off it as a field.
type Client struct {
	apiKey        string
	signingSecret string
	baseURL       string
	httpClient    *http.Client

	// Balances exposes workspace asset balances (scope: balances:read).
	Balances *BalancesService
	// Fees exposes fee previews (scope: balances:read).
	Fees *FeesService
	// Payouts exposes batch on-chain payouts (scopes: payouts:write / payouts:read).
	Payouts *PayoutsService
	// Refunds exposes refunds and the settled refund ledger (scopes: payments:write / ledger:read).
	Refunds *RefundsService
	// Conversions exposes currency conversions and the settled convert ledger (scopes: convert:write / ledger:read).
	Conversions *ConversionsService
	// Checkouts exposes hosted checkout links where the payer picks asset + network (scopes: invoices:write / invoices:read).
	Checkouts *CheckoutsService
	// Invoices exposes up-front fixed-address invoices (scopes: invoices:write / invoices:read).
	Invoices *InvoicesService
	// Subscriptions exposes recurring plans and subscriptions (scopes: subscriptions:write / subscriptions:read).
	Subscriptions *SubscriptionsService
	// GiftCards exposes gift-card issuance and lookup (scopes: balances:read / payments:write).
	GiftCards *GiftCardsService
	// OffRamp exposes crypto-to-fiat off-ramp (scopes: payouts:write / payouts:read).
	OffRamp *OffRampService
	// Reconciliation exposes settled payment/withdrawal reconciliation reports (scope: ledger:read).
	Reconciliation *ReconciliationService
	// Deposits exposes deposit chains, own-balance receive addresses, and history (scope: balances:read).
	Deposits *DepositsService
}

// Option customizes a Client during New. Options are applied in order.
type Option func(*Client)

// WithSigningSecret sets the request signing secret (apisign_...). It is required
// for app keys: when set, the client HMAC-SHA512-signs every request automatically.
// secret is the raw signing secret string. Keep it server-side only.
func WithSigningSecret(secret string) Option { return func(c *Client) { c.signingSecret = secret } }

// WithSandbox targets the public sandbox host instead of production when sandbox is
// true. It is ignored if WithBaseURL is also set (WithBaseURL wins).
func WithSandbox(sandbox bool) Option {
	return func(c *Client) {
		if sandbox && c.baseURL == "" {
			c.baseURL = SandboxBaseURL
		}
	}
}

// WithBaseURL overrides the API origin entirely, e.g. a local dev server. It takes
// precedence over WithSandbox. baseURL must use https unless the host is localhost
// or 127.0.0.1; a non-https, non-local URL makes New return an error.
func WithBaseURL(baseURL string) Option { return func(c *Client) { c.baseURL = baseURL } }

// WithHTTPClient sets a custom *http.Client, letting you control timeouts,
// transport, proxy, and TLS. It replaces the default 30s-timeout client.
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.httpClient = hc } }

// WithTimeout replaces the HTTP client with a fresh *http.Client whose per-request
// timeout is d. Use WithHTTPClient instead if you need to configure more than the timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient = &http.Client{Timeout: d} }
}

// New builds a Client. apiKey is required and identifies your workspace
// (ap_live_... for production, ap_test_... for sandbox/test). Pass Options to set
// the signing secret, choose sandbox/base URL, or supply a custom HTTP client. It
// returns an error if apiKey is empty or the resolved base URL is invalid or a
// non-https, non-localhost origin. Example:
//
//	ap, err := absolutepay.New(
//		"ap_live_...",
//		absolutepay.WithSigningSecret("apisign_..."),
//	)
//	if err != nil {
//		return err
//	}
//	balances, err := ap.Balances.List(ctx)
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
	c.Payouts = &PayoutsService{c}
	c.Refunds = &RefundsService{c}
	c.Conversions = &ConversionsService{c}
	c.Checkouts = &CheckoutsService{c}
	c.Invoices = &InvoicesService{c}
	c.Subscriptions = &SubscriptionsService{c: c, Plans: &SubscriptionPlansService{c}}
	c.GiftCards = &GiftCardsService{c}
	c.OffRamp = &OffRampService{c}
	c.Reconciliation = &ReconciliationService{c}
	c.Deposits = &DepositsService{c}
	return c, nil
}

// getPage GETs a cursor-paginated list into a Page[T]. It is a package-level
// generic helper because Go methods cannot carry their own type parameters.
func getPage[T any](ctx context.Context, c *Client, path string) (*Page[T], error) {
	var out Page[T]
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
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

// putInt adds an int query field to m only when it is positive (0 = omit).
func putInt(m map[string]string, k string, v int) {
	if v > 0 {
		m[k] = strconv.Itoa(v)
	}
}

// putInt64 adds an int64 query field to m only when it is positive (0 = omit).
func putInt64(m map[string]string, k string, v int64) {
	if v > 0 {
		m[k] = strconv.FormatInt(v, 10)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
