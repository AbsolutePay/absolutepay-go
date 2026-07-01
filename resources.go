package absolutepay

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// JSON is a loosely-typed object returned by endpoints without a dedicated struct.
type JSON = map[string]any

// RequestOption sets per-request extras (e.g. an idempotency key). These headers
// are merged AFTER signing and are not part of the signed canonical string.
type RequestOption func(map[string]string)

// WithIdempotencyKey makes a write retry-safe — the same key returns the original
// result instead of acting twice.
func WithIdempotencyKey(key string) RequestOption {
	return func(h map[string]string) { h["Idempotency-Key"] = key }
}

func headersFrom(opts []RequestOption) map[string]string {
	if len(opts) == 0 {
		return nil
	}
	h := map[string]string{}
	for _, o := range opts {
		o(h)
	}
	return h
}

func seg(s string) string { return url.PathEscape(s) }

// --- Balances (scope: balances:read) ---

type BalancesService struct{ c *Client }

// List returns every asset balance for the workspace.
func (s *BalancesService) List(ctx context.Context) ([]Balance, error) {
	var out []Balance
	return out, s.c.do(ctx, http.MethodGet, "/v1/balances", nil, nil, &out)
}

// Summary values the whole balance in one quote currency (default USDT).
func (s *BalancesService) Summary(ctx context.Context, quote string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/balances/summary"+qs(map[string]string{"quote": quote}), nil, nil, &out)
}

// --- Fees (scope: balances:read) ---

type FeesService struct{ c *Client }

// Preview returns the total fee on an amount for a payment type (default CHECKOUT).
func (s *FeesService) Preview(ctx context.Context, amount, currency string, paymentType PaymentType) (*FeePreview, error) {
	var out FeePreview
	q := qs(map[string]string{"amount": amount, "currency": currency, "paymentType": paymentType})
	return &out, s.c.do(ctx, http.MethodGet, "/v1/fees/preview"+q, nil, nil, &out)
}

// --- Payments (scope: payments:write) ---

type PaymentsService struct{ c *Client }

// CheckoutParams is the body for CreateCheckout.
type CheckoutParams struct {
	MerchantTradeNo string `json:"merchantTradeNo,omitempty"`
	Amount          Money  `json:"amount"`
	Chain           string `json:"chain"`
	MerchantUserID  int64  `json:"merchantUserId"`
	GoodsName       string `json:"goodsName"`
	TerminalType    string `json:"terminalType,omitempty"`
	ExpiresIn       int    `json:"expiresIn,omitempty"`
	Method          string `json:"method,omitempty"`
}

// CreateCheckout creates a pay-in order.
func (s *PaymentsService) CreateCheckout(ctx context.Context, p CheckoutParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/checkout", p, nil, &out)
}

// GetCheckout looks up a checkout by merchant trade number.
func (s *PaymentsService) GetCheckout(ctx context.Context, merchantTradeNo string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/checkout/"+seg(merchantTradeNo), nil, nil, &out)
}

// --- Payouts (scopes: payouts:write / payouts:read) ---

type PayoutsService struct{ c *Client }

// PayoutItem is one recipient in a batch payout.
type PayoutItem struct {
	RecipientAddress string `json:"recipientAddress"`
	Chain            string `json:"chain"`
	Amount           Money  `json:"amount"`
	Memo             string `json:"memo,omitempty"`
}

// Create submits a batch payout. Pass WithIdempotencyKey to make retries safe.
func (s *PayoutsService) Create(ctx context.Context, items []PayoutItem, opts ...RequestOption) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/payouts", map[string]any{"items": items}, headersFrom(opts), &out)
}

// Options lists supported chains + per-chain withdraw fee/limits for a currency.
func (s *PayoutsService) Options(ctx context.Context, currency string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/payouts/options"+qs(map[string]string{"currency": currency}), nil, nil, &out)
}

// Get looks up a payout batch by id.
func (s *PayoutsService) Get(ctx context.Context, id string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/payouts/"+seg(id), nil, nil, &out)
}

// --- Refunds (scope: payments:write) ---

type RefundsService struct{ c *Client }

// RefundParams is the body for Create.
type RefundParams struct {
	MerchantTradeNo string `json:"merchantTradeNo"`
	Amount          Money  `json:"amount"`
	Reason          string `json:"reason,omitempty"`
}

func (s *RefundsService) Create(ctx context.Context, p RefundParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/refunds", p, nil, &out)
}

// Get looks up a refund by its refundRequestId.
func (s *RefundsService) Get(ctx context.Context, id string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/refunds/"+seg(id), nil, nil, &out)
}

// --- Conversions (scope: convert:write) ---

type ConversionsService struct{ c *Client }

// QuoteParams requests a conversion quote. Set exactly one of SellAmount / BuyAmount.
type QuoteParams struct {
	SellCurrency string `json:"sellCurrency"`
	BuyCurrency  string `json:"buyCurrency"`
	SellAmount   string `json:"sellAmount,omitempty"`
	BuyAmount    string `json:"buyAmount,omitempty"`
}

// ConvertQuote is a conversion quote.
type ConvertQuote struct {
	QuoteID      string `json:"quoteId"`
	Rate         string `json:"rate"`
	SellCurrency string `json:"sellCurrency"`
	SellAmount   string `json:"sellAmount"`
	BuyCurrency  string `json:"buyCurrency"`
	BuyAmount    string `json:"buyAmount"`
}

// ConvertExecuteParams executes a previously quoted conversion.
type ConvertExecuteParams struct {
	QuoteID string `json:"quoteId"`
	Sell    Money  `json:"sell"`
	Buy     Money  `json:"buy"`
}

// Quote previews a conversion (no funds move).
func (s *ConversionsService) Quote(ctx context.Context, p QuoteParams) (*ConvertQuote, error) {
	var out ConvertQuote
	return &out, s.c.do(ctx, http.MethodPost, "/v1/conversions/quote", p, nil, &out)
}

// Execute runs a previously quoted conversion.
func (s *ConversionsService) Execute(ctx context.Context, p ConvertExecuteParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/conversions", p, nil, &out)
}

// Convert quotes then executes in one call.
func (s *ConversionsService) Convert(ctx context.Context, p QuoteParams) (JSON, error) {
	q, err := s.Quote(ctx, p)
	if err != nil {
		return nil, err
	}
	return s.Execute(ctx, ConvertExecuteParams{
		QuoteID: q.QuoteID,
		Sell:    Money{Amount: q.SellAmount, Currency: q.SellCurrency},
		Buy:     Money{Amount: q.BuyAmount, Currency: q.BuyCurrency},
	})
}

// --- Invoices + hosted payment links (scopes: invoices:write / invoices:read) ---

type InvoicesService struct {
	c *Client
	// Public holds the unauthenticated payer-facing endpoints.
	Public *PublicInvoicesService
}

// InvoiceParams is the body for Create / CreateCheckout. Set Chain on Create to
// mint the deposit address up front.
type InvoiceParams struct {
	Reference     string `json:"reference"`
	Amount        Money  `json:"amount"`
	Description   string `json:"description,omitempty"`
	CustomerEmail string `json:"customerEmail,omitempty"`
	ExpiresAt     int64  `json:"expiresAt,omitempty"`
	Chain         string `json:"chain,omitempty"`
}

func (s *InvoicesService) Create(ctx context.Context, p InvoiceParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/invoices", p, nil, &out)
}

// CreateCheckout creates a hosted checkout link (the payer picks the asset).
func (s *InvoicesService) CreateCheckout(ctx context.Context, p InvoiceParams) (JSON, error) {
	p.Chain = "" // checkout links let the payer choose the network
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/checkouts", p, nil, &out)
}

// List returns a keyset-paginated page. Pass a prior page's NextCursor as
// PageQuery.Before for the next page; NextCursor is nil on the last page.
func (s *InvoicesService) List(ctx context.Context, q PageQuery) (*Page, error) {
	var out Page
	return &out, s.c.do(ctx, http.MethodGet, "/v1/invoices"+pageQuery(q), nil, nil, &out)
}

func (s *InvoicesService) Stats(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/invoices/stats", nil, nil, &out)
}

// Pause pauses or unpauses an open invoice/link.
func (s *InvoicesService) Pause(ctx context.Context, token string, paused bool) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/invoices/"+seg(token)+"/pause", map[string]bool{"paused": paused}, nil, &out)
}

// Void makes an invoice/link permanently unpayable.
func (s *InvoicesService) Void(ctx context.Context, token string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/invoices/"+seg(token)+"/void", nil, nil, &out)
}

// PublicInvoicesService holds the unauthenticated payer endpoints (no API key).
type PublicInvoicesService struct{ c *Client }

func (s *PublicInvoicesService) Get(ctx context.Context, token string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/public/invoices/"+seg(token), nil, nil, &out)
}

func (s *PublicInvoicesService) Assets(ctx context.Context, token string) ([]JSON, error) {
	var out []JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/public/invoices/"+seg(token)+"/assets", nil, nil, &out)
}

// DepositParams selects the asset for a hosted-invoice deposit.
type DepositParams struct {
	Currency     string `json:"currency"`
	Chain        string `json:"chain"`
	FullCurrType string `json:"fullCurrType"`
}

func (s *PublicInvoicesService) Deposit(ctx context.Context, token string, p DepositParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/public/invoices/"+seg(token)+"/deposit", p, nil, &out)
}

func (s *PublicInvoicesService) Quote(ctx context.Context, token, currency string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/public/invoices/"+seg(token)+"/quote", map[string]string{"currency": currency}, nil, &out)
}

func (s *PublicInvoicesService) Status(ctx context.Context, token string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/public/invoices/"+seg(token)+"/status", nil, nil, &out)
}

// --- Subscriptions (scopes: subscriptions:read / subscriptions:write) ---

type SubscriptionsService struct{ c *Client }

// PlanParams defines a recurring billing plan.
type PlanParams struct {
	MerchantPlanNo string `json:"merchantPlanNo"`
	Name           string `json:"name"`
	Amount         Money  `json:"amount"`
	Interval       string `json:"interval"`
	IntervalCount  int    `json:"intervalCount"`
	TotalCycles    int    `json:"totalCycles"`
}

// SubscribeParams subscribes a customer to a plan.
type SubscribeParams struct {
	MerchantSubNo string `json:"merchantSubNo"`
	PlanNo        string `json:"planNo"`
	CallbackURL   string `json:"callbackUrl,omitempty"`
}

func (s *SubscriptionsService) ListPlans(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/subscription-plans", nil, nil, &out)
}

func (s *SubscriptionsService) CreatePlan(ctx context.Context, p PlanParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/subscription-plans", p, nil, &out)
}

// List returns a keyset-paginated page of subscriptions (see PageQuery / Page).
func (s *SubscriptionsService) List(ctx context.Context, q PageQuery) (*Page, error) {
	var out Page
	return &out, s.c.do(ctx, http.MethodGet, "/v1/subscriptions"+pageQuery(q), nil, nil, &out)
}

func (s *SubscriptionsService) Create(ctx context.Context, p SubscribeParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/subscriptions", p, nil, &out)
}

// Deductions returns the per-cycle charge history for a subscription.
func (s *SubscriptionsService) Deductions(ctx context.Context, merchantSubNo string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/subscriptions/"+seg(merchantSubNo)+"/deductions", nil, nil, &out)
}

func (s *SubscriptionsService) Cancel(ctx context.Context, merchantSubNo string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/subscriptions/"+seg(merchantSubNo)+"/cancel", nil, nil, &out)
}

// --- Gift cards (scopes: balances:read to read, payments:write to issue) ---

type GiftCardsService struct{ c *Client }

// GiftCardParams issues a gift card.
type GiftCardParams struct {
	Title      string `json:"title"`
	TemplateID string `json:"templateId"`
	Amount     Money  `json:"amount"`
}

func (s *GiftCardsService) Templates(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/giftcards/templates", nil, nil, &out)
}

// List returns a keyset-paginated page of issued gift cards (see PageQuery / Page).
func (s *GiftCardsService) List(ctx context.Context, q PageQuery) (*Page, error) {
	var out Page
	return &out, s.c.do(ctx, http.MethodGet, "/v1/giftcards"+pageQuery(q), nil, nil, &out)
}

func (s *GiftCardsService) Get(ctx context.Context, cardNum string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/giftcards/"+seg(cardNum), nil, nil, &out)
}

func (s *GiftCardsService) Create(ctx context.Context, p GiftCardParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/giftcards", p, nil, &out)
}

// --- Off-ramp (scopes: payouts:read / payouts:write) ---

type OffRampService struct{ c *Client }

// OffRampQuoteParams requests a crypto→fiat quote.
type OffRampQuoteParams struct {
	CryptoCurrency string `json:"cryptoCurrency"`
	FiatCurrency   string `json:"fiatCurrency"`
	CryptoAmount   string `json:"cryptoAmount"`
}

// OffRampWithdrawParams executes an off-ramp against a quote + registered bank.
type OffRampWithdrawParams struct {
	QuoteToken     string `json:"quoteToken"`
	BankAccountID  string `json:"bankAccountId"`
	CryptoCurrency string `json:"cryptoCurrency"`
	FiatCurrency   string `json:"fiatCurrency"`
	CryptoAmount   string `json:"cryptoAmount"`
	FiatAmount     string `json:"fiatAmount"`
}

func (s *OffRampService) Countries(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/offramp/countries", nil, nil, &out)
}

func (s *OffRampService) Banks(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/offramp/banks", nil, nil, &out)
}

func (s *OffRampService) Quote(ctx context.Context, p OffRampQuoteParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/offramp/quote", p, nil, &out)
}

func (s *OffRampService) Withdraw(ctx context.Context, p OffRampWithdrawParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/offramp/withdraw", p, nil, &out)
}

// Orders returns a keyset-paginated page of off-ramp orders (see PageQuery / Page).
func (s *OffRampService) Orders(ctx context.Context, q PageQuery) (*Page, error) {
	var out Page
	return &out, s.c.do(ctx, http.MethodGet, "/v1/offramp/orders"+pageQuery(q), nil, nil, &out)
}

// --- Transactions / unified ledger (scope: ledger:read) ---

type TransactionsService struct{ c *Client }

// TransactionsQuery filters the ledger. From/To are epoch milliseconds; page with
// Limit/Offset. Format "csv" returns an export.
type TransactionsQuery struct {
	Currency string
	From     int64
	To       int64
	Limit    int
	Offset   int
	Format   string
}

func (s *TransactionsService) List(ctx context.Context, q TransactionsQuery) (JSON, error) {
	m := map[string]string{"currency": q.Currency, "format": q.Format}
	if q.From > 0 {
		m["from"] = strconv.FormatInt(q.From, 10)
	}
	if q.To > 0 {
		m["to"] = strconv.FormatInt(q.To, 10)
	}
	if q.Limit > 0 {
		m["limit"] = strconv.Itoa(q.Limit)
	}
	if q.Offset > 0 {
		m["offset"] = strconv.Itoa(q.Offset)
	}
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/transactions"+qs(m), nil, nil, &out)
}
