package absolutepay

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// JSON is a loosely-typed object returned by endpoints that have no dedicated
// response struct. It is an alias for map[string]any; read fields by key, e.g.
// resp["orderId"]. Values follow encoding/json defaults (numbers decode to float64).
type JSON = map[string]any

// RequestOption sets per-request extras such as an idempotency key. The resulting
// headers are merged AFTER request signing and are therefore NOT part of the signed
// canonical string.
type RequestOption func(map[string]string)

// WithIdempotencyKey makes a write retry-safe: replaying a request with the same
// key returns the original result instead of performing the action twice. key is a
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

// List returns every asset balance for the workspace. ctx controls
// cancellation/deadline. It returns one Balance per asset, or an error.
func (s *BalancesService) List(ctx context.Context) ([]Balance, error) {
	var out []Balance
	return out, s.c.do(ctx, http.MethodGet, "/v1/balances", nil, nil, &out)
}

// Summary values the whole balance in a single quote currency. quote is the
// currency code to value in (e.g. "USDT"); pass "" to use the server default
// (USDT). It returns a loosely-typed JSON summary, or an error.
func (s *BalancesService) Summary(ctx context.Context, quote string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/balances/summary"+qs(map[string]string{"quote": quote}), nil, nil, &out)
}

// --- Fees (scope: balances:read) ---

// FeesService previews fees before you commit to an operation. Requires the
// balances:read scope.
type FeesService struct{ c *Client }

// Preview returns the fee breakdown for an amount and payment type. amount is the
// decimal-string value; currency is its code (e.g. "USDT"); paymentType is one of
// the Payment* constants (pass "" for the default CHECKOUT). It returns a
// *FeePreview, or an error.
func (s *FeesService) Preview(ctx context.Context, amount, currency string, paymentType PaymentType) (*FeePreview, error) {
	var out FeePreview
	q := qs(map[string]string{"amount": amount, "currency": currency, "paymentType": paymentType})
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
// a currency. currency is the asset code (e.g. "USDT"). It returns the options as
// JSON, or an error.
func (s *PayoutsService) Options(ctx context.Context, currency string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/payouts/options"+qs(map[string]string{"currency": currency}), nil, nil, &out)
}

// Get looks up a payout batch by its id and returns its status as JSON, or an error.
func (s *PayoutsService) Get(ctx context.Context, id string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/payouts/"+seg(id), nil, nil, &out)
}

// --- Refunds (scope: payments:write) ---

// RefundsService issues and looks up refunds against checkout orders. Requires the
// payments:write scope.
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
// details as JSON (including a refundRequestId), or an error. Example:
//
//	refund, err := ap.Refunds.Create(ctx, absolutepay.RefundParams{
//		MerchantTradeNo: "order-123",
//		Amount:          absolutepay.Money{Amount: "10.00", Currency: "USDT"},
//		Reason:          "customer request",
//	})
func (s *RefundsService) Create(ctx context.Context, p RefundParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/refunds", p, nil, &out)
}

// Get looks up a refund by its refundRequestId and returns its status as JSON, or
// an error.
func (s *RefundsService) Get(ctx context.Context, id string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/refunds/"+seg(id), nil, nil, &out)
}

// --- Conversions (scope: convert:write) ---

// ConversionsService quotes and executes currency conversions. Requires the
// convert:write scope.
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
// result as JSON, or an error.
func (s *ConversionsService) Execute(ctx context.Context, p ConvertExecuteParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/conversions", p, nil, &out)
}

// Convert quotes then immediately executes a conversion in one call — a convenience
// wrapper over Quote + Execute. p describes the legs (see QuoteParams). It returns
// the executed conversion as JSON, or an error from either step. Example:
//
//	res, err := ap.Conversions.Convert(ctx, absolutepay.QuoteParams{
//		SellCurrency: "USDT",
//		BuyCurrency:  "BTC",
//		SellAmount:   "100.00",
//	})
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

// InvoicesService creates and manages invoices and hosted payment links. Writes
// require invoices:write; reads require invoices:read. Payer-facing endpoints that
// need no API key live under Public.
type InvoicesService struct {
	c *Client
	// Public holds the unauthenticated, payer-facing invoice endpoints.
	Public *PublicInvoicesService
}

// InvoiceParams is the request body for Create and CreateCheckout. On Create, set
// Chain to mint the deposit address up front; CreateCheckout ignores Chain and lets
// the payer choose the network.
type InvoiceParams struct {
	// Reference is your unique invoice reference.
	Reference string `json:"reference"`
	// Amount is the amount and currency to bill.
	Amount Money `json:"amount"`
	// Description is an optional human-readable line item / memo.
	Description string `json:"description,omitempty"`
	// CustomerEmail is the payer's email, used for receipts/notifications. Optional.
	CustomerEmail string `json:"customerEmail,omitempty"`
	// RedirectURL is an http(s) URL the payer's browser is sent to once the hosted
	// checkout reaches a terminal state. AbsolutePay appends
	// ?token=<invoiceToken>&status=<SUCCESS|EXPIRED|CANCELED> (preserving any existing
	// query). Echoed back on the invoice when set. Optional.
	RedirectURL string `json:"redirectUrl,omitempty"`
	// ExpiresAt is the expiry time in epoch milliseconds. Optional (0 = no explicit expiry).
	ExpiresAt int64 `json:"expiresAt,omitempty"`
	// Chain is the blockchain network. On Create it mints the deposit address up
	// front; ignored by CreateCheckout. Optional.
	Chain string `json:"chain,omitempty"`
}

// Create creates a fixed-asset invoice and returns it as JSON (including its token
// and, if Chain was set, a deposit address), or an error.
func (s *InvoicesService) Create(ctx context.Context, p InvoiceParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/invoices", p, nil, &out)
}

// CreateCheckout creates a hosted checkout link where the payer picks the asset and
// network at pay time. Any Chain in p is cleared. It returns the link as JSON
// (including its token/URL), or an error.
func (s *InvoicesService) CreateCheckout(ctx context.Context, p InvoiceParams) (JSON, error) {
	p.Chain = "" // checkout links let the payer choose the network
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/checkouts", p, nil, &out)
}

// List returns a keyset-paginated page of invoices. q sets the page size, cursor,
// and optional status filter (see PageQuery). Pass a prior page's Page.NextCursor
// as q.Before for the next page; NextCursor is nil on the last page. Example:
//
//	q := absolutepay.PageQuery{Limit: 50}
//	for {
//		page, err := ap.Invoices.List(ctx, q)
//		if err != nil {
//			return err
//		}
//		// ... process page.Items ...
//		if page.NextCursor == nil {
//			break
//		}
//		q.Before = *page.NextCursor
//	}
func (s *InvoicesService) List(ctx context.Context, q PageQuery) (*Page, error) {
	var out Page
	return &out, s.c.do(ctx, http.MethodGet, "/v1/invoices"+pageQuery(q), nil, nil, &out)
}

// Stats returns aggregate invoice statistics for the workspace as JSON, or an error.
func (s *InvoicesService) Stats(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/invoices/stats", nil, nil, &out)
}

// Pause pauses or unpauses an open invoice or payment link. token identifies the
// invoice; paused=true pauses it and paused=false resumes it. It returns the
// updated state as JSON, or an error.
func (s *InvoicesService) Pause(ctx context.Context, token string, paused bool) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/invoices/"+seg(token)+"/pause", map[string]bool{"paused": paused}, nil, &out)
}

// Void makes an invoice or payment link permanently unpayable. token identifies the
// invoice. It returns the updated state as JSON, or an error.
func (s *InvoicesService) Void(ctx context.Context, token string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/invoices/"+seg(token)+"/void", nil, nil, &out)
}

// PublicInvoicesService holds the unauthenticated, payer-facing invoice endpoints
// (no API key required). Reach it via InvoicesService.Public. token is the public
// invoice token shown to the payer.
type PublicInvoicesService struct{ c *Client }

// Get returns the public view of an invoice by token as JSON, or an error.
func (s *PublicInvoicesService) Get(ctx context.Context, token string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/public/invoices/"+seg(token), nil, nil, &out)
}

// Assets lists the assets/networks the payer can use to pay the invoice identified
// by token. It returns one JSON object per payable asset, or an error.
func (s *PublicInvoicesService) Assets(ctx context.Context, token string) ([]JSON, error) {
	var out []JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/public/invoices/"+seg(token)+"/assets", nil, nil, &out)
}

// DepositParams selects the asset for a hosted-invoice deposit.
type DepositParams struct {
	// Currency is the asset code the payer chose, e.g. "USDT".
	Currency string `json:"currency"`
	// Chain is the blockchain network for the deposit, e.g. "TRX".
	Chain string `json:"chain"`
	// FullCurrType is the provider's fully-qualified currency/network identifier
	// (as returned by Assets).
	FullCurrType string `json:"fullCurrType"`
}

// Deposit selects an asset for the invoice identified by token and returns the
// deposit instructions (address/amount) as JSON, or an error.
func (s *PublicInvoicesService) Deposit(ctx context.Context, token string, p DepositParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/public/invoices/"+seg(token)+"/deposit", p, nil, &out)
}

// Quote returns the amount of currency required to pay the invoice identified by
// token. currency is the asset code the payer wants to pay in. It returns the quote
// as JSON, or an error.
func (s *PublicInvoicesService) Quote(ctx context.Context, token, currency string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/public/invoices/"+seg(token)+"/quote", map[string]string{"currency": currency}, nil, &out)
}

// Status returns the current payment status of the invoice identified by token as
// JSON, or an error. Poll this from the payer-facing page.
func (s *PublicInvoicesService) Status(ctx context.Context, token string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/public/invoices/"+seg(token)+"/status", nil, nil, &out)
}

// TrackOpen records that the payer opened the hosted invoice identified by token
// (an analytics/engagement signal). It sends no body and ignores the response,
// returning only an error if the request is rejected.
func (s *PublicInvoicesService) TrackOpen(ctx context.Context, token string) error {
	return s.c.do(ctx, http.MethodPost, "/v1/public/invoices/"+seg(token)+"/open", nil, nil, nil)
}

// --- Subscriptions (scopes: subscriptions:read / subscriptions:write) ---

// SubscriptionsService manages recurring billing plans and subscriptions. Writes
// require subscriptions:write; reads require subscriptions:read.
type SubscriptionsService struct{ c *Client }

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

// ListPlans returns the workspace's recurring billing plans as JSON, or an error.
func (s *SubscriptionsService) ListPlans(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/subscription-plans", nil, nil, &out)
}

// CreatePlan creates a recurring billing plan from p and returns it as JSON, or an
// error.
func (s *SubscriptionsService) CreatePlan(ctx context.Context, p PlanParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/subscription-plans", p, nil, &out)
}

// List returns a keyset-paginated page of subscriptions. q sets page size, cursor,
// and optional status filter; pass a prior Page.NextCursor as q.Before for the next
// page (nil NextCursor means last page). See PageQuery and Page.
func (s *SubscriptionsService) List(ctx context.Context, q PageQuery) (*Page, error) {
	var out Page
	return &out, s.c.do(ctx, http.MethodGet, "/v1/subscriptions"+pageQuery(q), nil, nil, &out)
}

// Create subscribes a customer to a plan (see SubscribeParams) and returns the new
// subscription as JSON, or an error.
func (s *SubscriptionsService) Create(ctx context.Context, p SubscribeParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/subscriptions", p, nil, &out)
}

// Deductions returns the per-cycle charge history for a subscription.
// merchantSubNo is the subscription reference. It returns the history as JSON, or
// an error.
func (s *SubscriptionsService) Deductions(ctx context.Context, merchantSubNo string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/subscriptions/"+seg(merchantSubNo)+"/deductions", nil, nil, &out)
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

// Templates returns the available gift-card design templates as JSON, or an error.
// Use a returned template's id as GiftCardParams.TemplateID.
func (s *GiftCardsService) Templates(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/giftcards/templates", nil, nil, &out)
}

// List returns a keyset-paginated page of issued gift cards. q sets page size,
// cursor, and optional status filter; pass a prior Page.NextCursor as q.Before for
// the next page (nil NextCursor means last page). See PageQuery and Page.
func (s *GiftCardsService) List(ctx context.Context, q PageQuery) (*Page, error) {
	var out Page
	return &out, s.c.do(ctx, http.MethodGet, "/v1/giftcards"+pageQuery(q), nil, nil, &out)
}

// Get looks up an issued gift card by its card number (cardNum) and returns it as
// JSON, or an error.
func (s *GiftCardsService) Get(ctx context.Context, cardNum string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/giftcards/"+seg(cardNum), nil, nil, &out)
}

// Create issues a gift card from p and returns it as JSON (including its card
// number), or an error.
func (s *GiftCardsService) Create(ctx context.Context, p GiftCardParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/giftcards", p, nil, &out)
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

// Countries returns the fiat destination countries supported for off-ramp as JSON,
// or an error.
func (s *OffRampService) Countries(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/offramp/countries", nil, nil, &out)
}

// Banks returns the workspace's registered destination bank accounts as JSON, or an
// error. Use a returned account's id as OffRampWithdrawParams.BankAccountID.
func (s *OffRampService) Banks(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/offramp/banks", nil, nil, &out)
}

// Quote returns a crypto-to-fiat quote (including a quote token and fiat amount) for
// p as JSON, or an error. Follow with Withdraw to execute it.
func (s *OffRampService) Quote(ctx context.Context, p OffRampQuoteParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/offramp/quote", p, nil, &out)
}

// Withdraw executes an off-ramp against a quote and registered bank account (see
// OffRampWithdrawParams) and returns the order as JSON, or an error.
func (s *OffRampService) Withdraw(ctx context.Context, p OffRampWithdrawParams) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/offramp/withdraw", p, nil, &out)
}

// Orders returns a keyset-paginated page of off-ramp orders. q sets page size,
// cursor, and optional status filter; pass a prior Page.NextCursor as q.Before for
// the next page (nil NextCursor means last page). See PageQuery and Page.
func (s *OffRampService) Orders(ctx context.Context, q PageQuery) (*Page, error) {
	var out Page
	return &out, s.c.do(ctx, http.MethodGet, "/v1/offramp/orders"+pageQuery(q), nil, nil, &out)
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

// DeleteBank removes a registered destination bank account. bankAccountID is the
// account's id (from RegisterBank or Banks). It returns an error if the request is
// rejected.
func (s *OffRampService) DeleteBank(ctx context.Context, bankAccountID string) error {
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

// --- Transactions / unified ledger (scope: ledger:read) ---

// TransactionsService reads the unified ledger across all operation types. Requires
// the ledger:read scope.
type TransactionsService struct{ c *Client }

// TransactionsQuery filters the ledger. All fields are optional; zero/empty fields
// are omitted from the request.
type TransactionsQuery struct {
	// Currency filters to a single asset code, e.g. "USDT". "" = all currencies.
	Currency string
	// From is the inclusive start of the time range, in epoch milliseconds (0 = unbounded).
	From int64
	// To is the inclusive end of the time range, in epoch milliseconds (0 = unbounded).
	To int64
	// Limit is the maximum number of rows to return (0 = server default).
	Limit int
	// Offset is the number of rows to skip for offset-based pagination.
	Offset int
	// Format selects the response format; "csv" returns an export instead of JSON.
	Format string
}

// List returns ledger entries matching q as JSON, or an error. When q.Format is
// "csv" the response is a CSV export. See TransactionsQuery for the filters.
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

// --- Reconciliation (scope: ledger:read) ---

// ReconciliationService reads settlement reconciliation reports that pair each
// settled payment/withdrawal with its network fee and net amount. Requires the
// ledger:read scope.
type ReconciliationService struct{ c *Client }

// ReconciliationQuery filters a reconciliation report by time range and page. All
// fields are optional; zero values are omitted from the request.
type ReconciliationQuery struct {
	// From is the inclusive start of the time range, in epoch milliseconds (0 = unbounded).
	From int64
	// To is the inclusive end of the time range, in epoch milliseconds (0 = unbounded).
	To int64
	// Limit is the maximum number of rows to return (0 = server default).
	Limit int
	// Offset is the number of rows to skip for offset-based pagination.
	Offset int
}

// reconQuery renders a ReconciliationQuery to query params (drops zero fields).
func reconQuery(q ReconciliationQuery) string {
	m := map[string]string{}
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
	return qs(m)
}

// Payments returns the settled pay-in reconciliation report matching q as JSON, or
// an error. See ReconciliationQuery for the time-range and pagination filters.
func (s *ReconciliationService) Payments(ctx context.Context, q ReconciliationQuery) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/reconciliation/payments"+reconQuery(q), nil, nil, &out)
}

// Withdrawals returns the settled withdrawal/payout reconciliation report matching q
// as JSON, or an error. See ReconciliationQuery for the time-range and pagination
// filters.
func (s *ReconciliationService) Withdrawals(ctx context.Context, q ReconciliationQuery) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/reconciliation/withdrawals"+reconQuery(q), nil, nil, &out)
}

// --- Deposits (scope: balances:read) ---

// DepositsService mints on-chain deposit addresses that credit the workspace
// balance. Requires the balances:read scope.
type DepositsService struct{ c *Client }

// Chains returns the blockchain networks available for deposits as JSON, or an
// error. Use a returned chain code with CreateAddress.
func (s *DepositsService) Chains(ctx context.Context) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodGet, "/v1/deposits/chains", nil, nil, &out)
}

// CreateAddress mints a deposit address on chain (e.g. "TRX", "ETH") that credits
// the workspace balance. It returns the address details as JSON, or an error.
func (s *DepositsService) CreateAddress(ctx context.Context, chain string) (JSON, error) {
	var out JSON
	return out, s.c.do(ctx, http.MethodPost, "/v1/deposits/address", map[string]string{"chain": chain}, nil, &out)
}
