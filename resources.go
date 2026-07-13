package absolutepay

import (
	"context"
	"net/http"
	"net/url"
)

// JSON is a loosely-typed object returned by endpoints that have no dedicated
// response struct. It is an alias for map[string]any; read fields by key, e.g.
// resp["token"]. Values follow encoding/json defaults (numbers decode to float64).
type JSON = map[string]any

// RequestOption sets per-request extras such as an idempotency key. The resulting
// headers are merged AFTER request signing and are therefore NOT part of the signed
// canonical string.
type RequestOption func(map[string]string)

// WithIdempotencyKey makes a money-moving POST retry-safe: replaying a request with
// the same key returns the original result instead of performing the action twice
// (a 409 surfaces an in-progress/conflicting replay as a normal *Error). key is a
// caller-chosen unique string (e.g. a UUID) that you reuse across retries of the
// same logical operation.
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

// BalancesService reads workspace asset balances. Requires the balances:read scope.
type BalancesService struct{ c *Client }

// List returns every asset balance for the workspace as a Page[Balance] (one item
// per asset; the list is not cursor-paged, so NextCursor is empty), or an error.
func (s *BalancesService) List(ctx context.Context) (*Page[Balance], error) {
	return getPage[Balance](ctx, s.c, "/v1/balances")
}

// --- Fees (scope: balances:read) ---

// FeesService previews fees before you commit to an operation. Requires the
// balances:read scope.
type FeesService struct{ c *Client }

// Preview returns the fee breakdown for an amount and payment type. amount is the
// decimal-string value; currency is its code (e.g. "USDT"); paymentType is one of
// the Payment* constants (pass "" for the default CHECKOUT). chain is the network
// (e.g. "MATIC"): REQUIRED for PaymentWithdrawal/PaymentPayout (payout fees are
// per-chain) and ignored for pay-in — pass "" for CHECKOUT. It returns a
// *FeePreview, or an error (a *Error with Code "chain_required" if chain is missing
// for a withdrawal/payout).
func (s *FeesService) Preview(ctx context.Context, amount, currency string, paymentType PaymentType, chain string) (*FeePreview, error) {
	if (paymentType == PaymentWithdrawal || paymentType == PaymentPayout) && chain == "" {
		return nil, &Error{Status: 400, Code: "chain_required", Title: "a chain is required to preview a payout/withdrawal fee"}
	}
	var out FeePreview
	q := qs(map[string]string{"amount": amount, "currency": currency, "paymentType": paymentType, "chain": chain})
	return &out, s.c.do(ctx, http.MethodGet, "/v1/fees/preview"+q, nil, nil, &out)
}

// --- Payouts (scopes: payouts:write / payouts:read) ---

// PayoutsService sends batch on-chain payouts and reads their status. Creating a
// payout requires payouts:write; reads require payouts:read.
type PayoutsService struct{ c *Client }

// PayoutItem is one recipient in a batch payout.
type PayoutItem struct {
	// RecipientAddress is the destination on-chain wallet address.
	RecipientAddress string `json:"recipientAddress"`
	// Chain is the blockchain network to send over, e.g. "TRX", "ETH".
	Chain string `json:"chain"`
	// Amount is the amount and currency to send to this recipient.
	Amount Money `json:"amount"`
	// Memo is an optional destination memo/tag (required by some chains/exchanges).
	Memo string `json:"memo,omitempty"`
}

// Create submits a batch payout of items and returns the batch details as JSON.
// Pass WithIdempotencyKey to make retries safe: replaying with the same key returns
// the original batch instead of paying twice. It returns an error if rejected.
// Example:
//
//	batch, err := ap.Payouts.Create(ctx,
//		[]absolutepay.PayoutItem{{
//			RecipientAddress: "T...",
//			Chain:            "TRX",
//			Amount:           absolutepay.Money{Amount: "5.00", Currency: "USDT"},
//		}},
//		absolutepay.WithIdempotencyKey("payout-2026-06-01-001"),
//	)
func (s *PayoutsService) Create(ctx context.Context, items []PayoutItem, opts ...RequestOption) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/payouts", map[string]any{"items": items}, headersFrom(opts), &out)
}

// Options lists the supported chains plus the per-chain withdraw fee and limits for
// a currency, as a Page (one item per chain option). currency is the asset code
// (e.g. "USDT"). It returns the options, or an error.
func (s *PayoutsService) Options(ctx context.Context, currency string) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/payouts/options"+qs(map[string]string{"currency": currency}))
}

// Get looks up a payout batch by its id and returns its status as JSON, or an error.
func (s *PayoutsService) Get(ctx context.Context, id string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/payouts/"+seg(id), nil, nil, &out)
}

// --- Refunds (scopes: payments:write to issue, ledger:read to list/get) ---

// RefundsService issues refunds against settled checkout orders and reads the
// settled refund ledger. Issuing requires payments:write; reads require ledger:read.
type RefundsService struct{ c *Client }

// RefundParams is the request body for Create.
type RefundParams struct {
	// MerchantTradeNo is the order reference of the checkout being refunded.
	MerchantTradeNo string `json:"merchantTradeNo"`
	// Amount is the amount and currency to refund (may be a partial amount).
	Amount Money `json:"amount"`
	// Reason is an optional human-readable refund reason.
	Reason string `json:"reason,omitempty"`
}

// Create issues a refund against a settled checkout order and returns the refund
// details as JSON (including a refundRequestId), or an error. Pass
// WithIdempotencyKey to make retries safe. Example:
//
//	refund, err := ap.Refunds.Create(ctx, absolutepay.RefundParams{
//		MerchantTradeNo: "order-123",
//		Amount:          absolutepay.Money{Amount: "10.00", Currency: "USDT"},
//		Reason:          "customer request",
//	}, absolutepay.WithIdempotencyKey("refund-order-123-1"))
func (s *RefundsService) Create(ctx context.Context, p RefundParams, opts ...RequestOption) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/refunds", p, headersFrom(opts), &out)
}

// Get looks up a refund by its refundRequestId and returns its status as JSON, or
// an error.
func (s *RefundsService) Get(ctx context.Context, id string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/refunds/"+seg(id), nil, nil, &out)
}

// List returns a cursor-paginated page of the settled REFUND ledger history (the
// page carries Total, the true count for the filtered range). q filters by time
// range/currency and pages; pass a prior Page.NextCursor as q.Before for the next
// page ("" means the last page). See LedgerQuery.
func (s *RefundsService) List(ctx context.Context, q LedgerQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/refunds"+qs(q.values()))
}

// --- Conversions (scopes: convert:write to execute, ledger:read to list) ---

// ConversionsService quotes and executes currency conversions and reads the settled
// convert ledger. Quoting/executing require convert:write; List requires ledger:read.
type ConversionsService struct{ c *Client }

// QuoteParams requests a conversion quote. Set exactly one of SellAmount or
// BuyAmount to fix that side; the other is computed from the rate.
type QuoteParams struct {
	// SellCurrency is the currency code you are converting from.
	SellCurrency string `json:"sellCurrency"`
	// BuyCurrency is the currency code you are converting to.
	BuyCurrency string `json:"buyCurrency"`
	// SellAmount fixes the amount to sell, as a decimal string. Optional (set this OR BuyAmount).
	SellAmount string `json:"sellAmount,omitempty"`
	// BuyAmount fixes the amount to buy, as a decimal string. Optional (set this OR SellAmount).
	BuyAmount string `json:"buyAmount,omitempty"`
}

// ConvertQuote is a conversion quote returned by Quote. It is short-lived; pass its
// QuoteID to Execute to lock in the trade.
type ConvertQuote struct {
	// QuoteID identifies this quote; pass it to Execute.
	QuoteID string `json:"quoteId"`
	// Rate is the quoted exchange rate as a decimal string.
	Rate string `json:"rate"`
	// SellCurrency is the currency being sold.
	SellCurrency string `json:"sellCurrency"`
	// SellAmount is the amount to be sold, as a decimal string.
	SellAmount string `json:"sellAmount"`
	// BuyCurrency is the currency being bought.
	BuyCurrency string `json:"buyCurrency"`
	// BuyAmount is the amount to be bought, as a decimal string.
	BuyAmount string `json:"buyAmount"`
}

// ConvertExecuteParams executes a previously obtained conversion quote.
type ConvertExecuteParams struct {
	// QuoteID is the id from the ConvertQuote returned by Quote.
	QuoteID string `json:"quoteId"`
	// Sell is the amount and currency to sell (must match the quote).
	Sell Money `json:"sell"`
	// Buy is the amount and currency to buy (must match the quote).
	Buy Money `json:"buy"`
}

// Quote previews a conversion without moving any funds and returns a *ConvertQuote
// (rate and both legs), or an error. Follow with Execute to commit it.
func (s *ConversionsService) Quote(ctx context.Context, p QuoteParams) (*ConvertQuote, error) {
	var out ConvertQuote
	return &out, s.c.do(ctx, http.MethodPost, "/v1/conversions/quote", p, nil, &out)
}

// Execute runs a previously quoted conversion (this moves funds) and returns the
// result as JSON, or an error. Pass WithIdempotencyKey to make retries safe.
func (s *ConversionsService) Execute(ctx context.Context, p ConvertExecuteParams, opts ...RequestOption) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/conversions", p, headersFrom(opts), &out)
}

// List returns a cursor-paginated page of the settled CONVERT ledger history (the
// page carries Total, the true count for the filtered range). See LedgerQuery.
func (s *ConversionsService) List(ctx context.Context, q LedgerQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/conversions"+qs(q.values()))
}

// --- Checkouts (scopes: invoices:write / invoices:read) ---

// CheckoutsService creates and manages hosted checkout links, where the payer picks
// the asset and network at pay time. Writes require invoices:write; reads require
// invoices:read.
type CheckoutsService struct{ c *Client }

// CheckoutParams is the request body for CheckoutsService.Create.
type CheckoutParams struct {
	// Reference is your unique checkout reference.
	Reference string `json:"reference"`
	// Amount is the amount and currency to bill.
	Amount Money `json:"amount"`
	// Description is an optional human-readable line item / memo.
	Description string `json:"description,omitempty"`
	// CustomerEmail is the payer's email, used for receipts/notifications. Optional.
	CustomerEmail string `json:"customerEmail,omitempty"`
	// ExpiresAt is the expiry time in epoch milliseconds. Optional (0 = no explicit expiry).
	ExpiresAt int64 `json:"expiresAt,omitempty"`
	// RedirectURL is an http(s) URL the payer's browser is sent to once the hosted
	// checkout reaches a terminal state. AbsolutePay appends
	// ?token=<token>&status=<SUCCESS|EXPIRED|CANCELED> (preserving any existing query).
	// Optional.
	RedirectURL string `json:"redirectUrl,omitempty"`
}

// ResourceUpdate patches an open checkout or invoice. Only set fields are sent; a
// pointer left nil is omitted. To explicitly clear RedirectURL/Description, send an
// empty string; to change Paused, set the pointer.
type ResourceUpdate struct {
	// Paused pauses (true) or resumes (false) the link. nil leaves it unchanged.
	Paused *bool `json:"paused,omitempty"`
	// RedirectURL replaces the post-checkout redirect URL. nil leaves it unchanged.
	RedirectURL *string `json:"redirectUrl,omitempty"`
	// ExpiresAt replaces the expiry (epoch ms). nil leaves it unchanged.
	ExpiresAt *int64 `json:"expiresAt,omitempty"`
	// Description replaces the description. nil leaves it unchanged.
	Description *string `json:"description,omitempty"`
}

// Create creates a hosted checkout link and returns it as JSON (including token and
// checkoutUrl — send the payer to checkoutUrl). It returns an error if rejected.
func (s *CheckoutsService) Create(ctx context.Context, p CheckoutParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/checkouts", p, nil, &out)
}

// List returns a cursor-paginated page of checkout links. q sets page size, cursor,
// order, and optional status/search filters; pass a prior Page.NextCursor as
// q.Before ("" means the last page). See ListQuery.
func (s *CheckoutsService) List(ctx context.Context, q ListQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/checkouts"+qs(q.values()))
}

// Get looks up a checkout link by its token and returns it as JSON, or an error.
// Use it as a settlement-confirmation fallback alongside the payment.succeeded
// webhook.
func (s *CheckoutsService) Get(ctx context.Context, token string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/checkouts/"+seg(token), nil, nil, &out)
}

// Update patches an open checkout link (pause/resume, redirect, expiry,
// description) and returns the updated state as JSON, or an error. See ResourceUpdate.
func (s *CheckoutsService) Update(ctx context.Context, token string, patch ResourceUpdate) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPatch, "/v1/checkouts/"+seg(token), patch, nil, &out)
}

// Delete voids a checkout link, making it permanently unpayable. token identifies
// the link. It returns an error if the request is rejected.
func (s *CheckoutsService) Delete(ctx context.Context, token string) error {
	return s.c.do(ctx, http.MethodDelete, "/v1/checkouts/"+seg(token), nil, nil, nil)
}

// --- Invoices (scopes: invoices:write / invoices:read) ---

// InvoicesService creates and manages up-front invoices: the deposit address is
// minted at create time on a fixed Chain. Writes require invoices:write; reads
// require invoices:read.
type InvoicesService struct{ c *Client }

// InvoiceParams is the request body for InvoicesService.Create. Chain is REQUIRED —
// it mints the deposit address up front. For a payer-picks-the-network flow, use
// CheckoutsService instead.
type InvoiceParams struct {
	// Reference is your unique invoice reference.
	Reference string `json:"reference"`
	// Amount is the amount and currency to bill.
	Amount Money `json:"amount"`
	// Chain is the blockchain network; REQUIRED. It mints the deposit address up front.
	Chain string `json:"chain"`
	// Description is an optional human-readable line item / memo.
	Description string `json:"description,omitempty"`
	// CustomerEmail is the payer's email, used for receipts/notifications. Optional.
	CustomerEmail string `json:"customerEmail,omitempty"`
	// ExpiresAt is the expiry time in epoch milliseconds. Optional (0 = no explicit expiry).
	ExpiresAt int64 `json:"expiresAt,omitempty"`
	// RedirectURL is an http(s) URL the payer's browser returns to at a terminal
	// state (?token=…&status=…). Optional.
	RedirectURL string `json:"redirectUrl,omitempty"`
}

// Create creates a fixed-asset invoice (Chain required) and returns it as JSON
// (including its token, address, chain, and amount), or an error.
func (s *InvoicesService) Create(ctx context.Context, p InvoiceParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/invoices", p, nil, &out)
}

// List returns a cursor-paginated page of invoices. q sets page size, cursor,
// order, and optional status/search filters; pass a prior Page.NextCursor as
// q.Before ("" means the last page). See ListQuery.
func (s *InvoicesService) List(ctx context.Context, q ListQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/invoices"+qs(q.values()))
}

// Get looks up an invoice by its token and returns it as JSON, or an error.
func (s *InvoicesService) Get(ctx context.Context, token string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/invoices/"+seg(token), nil, nil, &out)
}

// Update patches an open invoice (pause/resume, redirect, expiry, description) and
// returns the updated state as JSON, or an error. See ResourceUpdate.
func (s *InvoicesService) Update(ctx context.Context, token string, patch ResourceUpdate) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPatch, "/v1/invoices/"+seg(token), patch, nil, &out)
}

// Delete voids an invoice, making it permanently unpayable. token identifies the
// invoice. It returns an error if the request is rejected.
func (s *InvoicesService) Delete(ctx context.Context, token string) error {
	return s.c.do(ctx, http.MethodDelete, "/v1/invoices/"+seg(token), nil, nil, nil)
}

// --- Subscriptions (scopes: subscriptions:read / subscriptions:write) ---

// SubscriptionsService manages recurring subscriptions. Reach the plan catalog via
// the nested Plans sub-service (ap.Subscriptions.Plans). Writes require
// subscriptions:write; reads require subscriptions:read.
type SubscriptionsService struct {
	c *Client
	// Plans manages the recurring billing plan catalog (ap.Subscriptions.Plans.*).
	Plans *SubscriptionPlansService
}

// SubscriptionPlansService manages the recurring billing plan catalog. Reach it via
// ap.Subscriptions.Plans. Creating a plan requires subscriptions:write; listing
// requires subscriptions:read.
type SubscriptionPlansService struct{ c *Client }

// PlanParams defines a recurring billing plan.
type PlanParams struct {
	// MerchantPlanNo is your unique plan reference.
	MerchantPlanNo string `json:"merchantPlanNo"`
	// Name is the human-readable plan name.
	Name string `json:"name"`
	// Amount is the amount and currency charged each cycle.
	Amount Money `json:"amount"`
	// Interval is the billing interval unit, e.g. "DAY", "WEEK", "MONTH", "YEAR".
	Interval string `json:"interval"`
	// IntervalCount is the number of Interval units between charges (e.g. 3 with "MONTH" = quarterly).
	IntervalCount int `json:"intervalCount"`
	// TotalCycles is the number of charges before the subscription ends (0 = unlimited).
	TotalCycles int `json:"totalCycles"`
}

// SubscribeParams subscribes a customer to an existing plan.
type SubscribeParams struct {
	// MerchantSubNo is your unique subscription reference.
	MerchantSubNo string `json:"merchantSubNo"`
	// PlanNo is the plan's reference (MerchantPlanNo) to subscribe to.
	PlanNo string `json:"planNo"`
	// CallbackURL is an optional per-subscription webhook override URL.
	CallbackURL string `json:"callbackUrl,omitempty"`
}

// List returns the workspace's recurring billing plans as a Page (one item per
// plan), or an error.
func (s *SubscriptionPlansService) List(ctx context.Context) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/subscription-plans")
}

// Create creates a recurring billing plan from p and returns it as JSON, or an
// error. Pass WithIdempotencyKey to make retries safe.
func (s *SubscriptionPlansService) Create(ctx context.Context, p PlanParams, opts ...RequestOption) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/subscription-plans", p, headersFrom(opts), &out)
}

// List returns a cursor-paginated page of subscriptions. q sets page size, cursor,
// order, and optional status/search filters; pass a prior Page.NextCursor as
// q.Before ("" means the last page). See ListQuery.
func (s *SubscriptionsService) List(ctx context.Context, q ListQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/subscriptions"+qs(q.values()))
}

// Create subscribes a customer to a plan (see SubscribeParams) and returns the new
// subscription as JSON, or an error. Pass WithIdempotencyKey to make retries safe.
func (s *SubscriptionsService) Create(ctx context.Context, p SubscribeParams, opts ...RequestOption) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/subscriptions", p, headersFrom(opts), &out)
}

// Deductions returns the per-cycle charge history for a subscription as a Page (one
// item per cycle). merchantSubNo is the subscription reference. It returns an error
// if rejected.
func (s *SubscriptionsService) Deductions(ctx context.Context, merchantSubNo string) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/subscriptions/"+seg(merchantSubNo)+"/deductions")
}

// Cancel cancels a subscription identified by merchantSubNo and returns the updated
// state as JSON, or an error.
func (s *SubscriptionsService) Cancel(ctx context.Context, merchantSubNo string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/subscriptions/"+seg(merchantSubNo)+"/cancel", nil, nil, &out)
}

// --- Gift cards (scopes: balances:read to read, payments:write to issue) ---

// GiftCardsService issues and looks up gift cards. Reads require balances:read;
// issuing a card requires payments:write.
type GiftCardsService struct{ c *Client }

// GiftCardParams issues a gift card.
type GiftCardParams struct {
	// Title is the gift card's display title.
	Title string `json:"title"`
	// TemplateID is the design template id (from Templates) to render the card with.
	TemplateID string `json:"templateId"`
	// Amount is the face value and currency loaded onto the card.
	Amount Money `json:"amount"`
}

// Templates returns the available gift-card design templates as a Page (one item
// per template), or an error. Use a returned template's id as GiftCardParams.TemplateID.
func (s *GiftCardsService) Templates(ctx context.Context) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/giftcards/templates")
}

// List returns a cursor-paginated page of issued gift cards. q sets page size,
// cursor, order, and optional status/search filters; pass a prior Page.NextCursor
// as q.Before ("" means the last page). See ListQuery.
func (s *GiftCardsService) List(ctx context.Context, q ListQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/giftcards"+qs(q.values()))
}

// Get looks up an issued gift card by its card number (cardNum) and returns it as
// JSON, or an error.
func (s *GiftCardsService) Get(ctx context.Context, cardNum string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/giftcards/"+seg(cardNum), nil, nil, &out)
}

// Create issues a gift card from p and returns it as JSON (including its card
// number), or an error. Pass WithIdempotencyKey to make retries safe.
func (s *GiftCardsService) Create(ctx context.Context, p GiftCardParams, opts ...RequestOption) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/giftcards", p, headersFrom(opts), &out)
}

// --- Off-ramp (scopes: payouts:read / payouts:write) ---

// OffRampService converts crypto to fiat and pays out to a registered bank
// account. Reads require payouts:read; quoting/withdrawing require payouts:write.
type OffRampService struct{ c *Client }

// OffRampQuoteParams requests a crypto-to-fiat quote.
type OffRampQuoteParams struct {
	// CryptoCurrency is the asset code being sold, e.g. "USDT".
	CryptoCurrency string `json:"cryptoCurrency"`
	// FiatCurrency is the target fiat currency code, e.g. "USD", "EUR".
	FiatCurrency string `json:"fiatCurrency"`
	// CryptoAmount is the amount of crypto to sell, as a decimal string.
	CryptoAmount string `json:"cryptoAmount"`
}

// OffRampWithdrawParams executes an off-ramp against a prior quote and a registered
// bank account.
type OffRampWithdrawParams struct {
	// QuoteToken is the token from the OffRampService.Quote response.
	QuoteToken string `json:"quoteToken"`
	// BankAccountID identifies the registered destination bank account.
	BankAccountID string `json:"bankAccountId"`
	// CryptoCurrency is the asset code being sold (must match the quote).
	CryptoCurrency string `json:"cryptoCurrency"`
	// FiatCurrency is the target fiat currency code (must match the quote).
	FiatCurrency string `json:"fiatCurrency"`
	// CryptoAmount is the crypto amount to sell, as a decimal string (must match the quote).
	CryptoAmount string `json:"cryptoAmount"`
	// FiatAmount is the fiat amount to receive, as a decimal string (from the quote).
	FiatAmount string `json:"fiatAmount"`
}

// Countries returns the fiat destination countries supported for off-ramp as a Page
// (one item per country), or an error.
func (s *OffRampService) Countries(ctx context.Context) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/offramp/countries")
}

// Banks returns the workspace's registered destination bank accounts as a Page (one
// item per account), or an error. Use a returned account's id as
// OffRampWithdrawParams.BankAccountID.
func (s *OffRampService) Banks(ctx context.Context) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/offramp/banks")
}

// Quote returns a crypto-to-fiat quote (including a quote token and fiat amount) for
// p as JSON, or an error. Follow with Withdraw to execute it.
func (s *OffRampService) Quote(ctx context.Context, p OffRampQuoteParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/offramp/quote", p, nil, &out)
}

// Withdraw executes an off-ramp against a quote and registered bank account (see
// OffRampWithdrawParams) and returns the order as JSON, or an error. Pass
// WithIdempotencyKey to make retries safe.
func (s *OffRampService) Withdraw(ctx context.Context, p OffRampWithdrawParams, opts ...RequestOption) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/offramp/withdraw", p, headersFrom(opts), &out)
}

// Orders returns a cursor-paginated page of off-ramp orders. q sets page size,
// cursor, order, and optional status/search filters; pass a prior Page.NextCursor
// as q.Before ("" means the last page). See ListQuery.
func (s *OffRampService) Orders(ctx context.Context, q ListQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/offramp/orders"+qs(q.values()))
}

// DocFile is a base64-encoded document uploaded inline with an off-ramp bank
// registration or compliance submission (e.g. a proof-of-address certificate or a
// passport scan).
type DocFile struct {
	// Filename is the original file name, e.g. "passport.pdf".
	Filename string `json:"filename"`
	// ContentType is the file's MIME type, e.g. "application/pdf" or "image/png".
	ContentType string `json:"contentType"`
	// DataBase64 is the file's bytes, base64-encoded (standard encoding, no data: URI prefix).
	DataBase64 string `json:"dataBase64"`
}

// BankParams registers a fiat destination bank account for off-ramp withdrawals.
type BankParams struct {
	// BankAccountName is the account holder's name as it appears at the bank.
	BankAccountName string `json:"bankAccountName"`
	// BankName is the destination bank's name.
	BankName string `json:"bankName"`
	// CountryID is the numeric id of the bank's country (from OffRampService.Countries).
	CountryID int `json:"countryId"`
	// IBAN is the destination account's IBAN.
	IBAN string `json:"iban"`
	// Swift is the bank's SWIFT/BIC code. Optional.
	Swift string `json:"swift,omitempty"`
	// Address is the account holder's address. Optional.
	Address string `json:"address,omitempty"`
	// RemittanceLineNumber is an optional remittance reference line required by some corridors.
	RemittanceLineNumber string `json:"remittanceLineNumber,omitempty"`
	// File is the proof-of-account document uploaded inline (see DocFile).
	File DocFile `json:"file"`
}

// BankMaterialsParams submits additional compliance documents for a registered bank
// account (e.g. when off-ramp review requests more materials).
type BankMaterialsParams struct {
	// Certificate holds proof-of-account / certificate documents.
	Certificate []DocFile `json:"certificate"`
	// Passport holds identity (passport) documents.
	Passport []DocFile `json:"passport"`
}

// RegisterBank registers a fiat destination bank account (see BankParams, including
// the inline proof document) and returns the created account as JSON (with its id,
// usable as OffRampWithdrawParams.BankAccountID), or an error.
func (s *OffRampService) RegisterBank(ctx context.Context, p BankParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/offramp/banks", p, nil, &out)
}

// RemoveBank removes a registered destination bank account. bankAccountID is the
// account's id (from RegisterBank or Banks). It returns an error if rejected.
func (s *OffRampService) RemoveBank(ctx context.Context, bankAccountID string) error {
	return s.c.do(ctx, http.MethodDelete, "/v1/offramp/banks/"+seg(bankAccountID), nil, nil, nil)
}

// SubmitBankMaterials uploads additional compliance documents for a registered bank
// account. bankAccountID identifies the account; p carries the certificate/passport
// documents (see BankMaterialsParams). It returns the updated review state as JSON,
// or an error.
func (s *OffRampService) SubmitBankMaterials(ctx context.Context, bankAccountID string, p BankMaterialsParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/offramp/banks/"+seg(bankAccountID)+"/materials", p, nil, &out)
}

// --- Reconciliation (scope: ledger:read) ---

// ReconciliationService reads settlement reconciliation reports that pair each
// settled payment/withdrawal with its network fee and net amount. Requires the
// ledger:read scope.
type ReconciliationService struct{ c *Client }

// Payments returns a cursor-paginated page of the settled pay-in reconciliation
// report matching q (the page carries Total, the true count for the filtered range),
// or an error. See ReconciliationQuery.
func (s *ReconciliationService) Payments(ctx context.Context, q ReconciliationQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/reconciliation/payments"+qs(q.values()))
}

// Withdrawals returns a cursor-paginated page of the settled withdrawal/payout
// reconciliation report matching q (the page carries Total), or an error. See
// ReconciliationQuery.
func (s *ReconciliationService) Withdrawals(ctx context.Context, q ReconciliationQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/reconciliation/withdrawals"+qs(q.values()))
}

// --- Deposits (scope: balances:read) ---

// DepositsService lists deposit chains, mints own-balance receive addresses, and
// reads settled deposit history. Requires the balances:read scope.
type DepositsService struct{ c *Client }

// Chains returns the blockchain networks available for deposits as a Page (one item
// per chain), or an error. Use a returned chain code with CreateAddress.
func (s *DepositsService) Chains(ctx context.Context) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/deposits/chains")
}

// CreateAddress mints (or returns the existing) deposit address on chain (e.g.
// "TRX", "ETH") that credits the workspace balance. It is idempotent per chain and
// returns the address as JSON, or an error.
func (s *DepositsService) CreateAddress(ctx context.Context, chain string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/deposits/address", map[string]string{"chain": chain}, nil, &out)
}

// Addresses returns a cursor-paginated page of the workspace's minted deposit
// addresses. q filters by chain and pages; pass a prior Page.NextCursor as q.Before
// ("" means the last page). See AddressQuery.
func (s *DepositsService) Addresses(ctx context.Context, q AddressQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/deposits/addresses"+qs(q.values()))
}

// GetAddress returns the workspace's deposit address for a single chain as JSON, or
// an error.
func (s *DepositsService) GetAddress(ctx context.Context, chain string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/deposits/addresses/"+seg(chain), nil, nil, &out)
}

// List returns a cursor-paginated page of settled deposit history. q filters by
// chain/time range and pages; pass a prior Page.NextCursor as q.Before ("" means
// the last page). See DepositHistoryQuery.
func (s *DepositsService) List(ctx context.Context, q DepositHistoryQuery) (*Page[JSON], error) {
	return getPage[JSON](ctx, s.c, "/v1/deposits"+qs(q.values()))
}
