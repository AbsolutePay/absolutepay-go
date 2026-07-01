package absolutepay

import "encoding/json"

// Money is a monetary value: a decimal-string amount together with its currency
// code, e.g. Money{Amount: "10.00", Currency: "USDT"}. Amounts are ALWAYS strings
// (never floats) so no precision is lost on the money path.
type Money struct {
	// Amount is the decimal value as a string, e.g. "10.00". Never a float.
	Amount string `json:"amount"`
	// Currency is the asset/currency code, e.g. "USDT", "BTC", "USD".
	Currency string `json:"currency"`
}

// PaymentType enumerates the fee-bearing operation kinds. It is an alias for
// string; use the Payment* constants for the accepted values.
type PaymentType = string

// The set of PaymentType values accepted by fee/preview and related endpoints.
const (
	// PaymentCheckout is a pay-in (customer pays the merchant).
	PaymentCheckout PaymentType = "CHECKOUT"
	// PaymentWithdrawal is an on-chain payout/withdrawal.
	PaymentWithdrawal PaymentType = "WITHDRAWAL"
	// PaymentSubscription is a recurring subscription charge.
	PaymentSubscription PaymentType = "SUBSCRIPTION"
	// PaymentConversion is a currency-to-currency conversion.
	PaymentConversion PaymentType = "CONVERSION"
	// PaymentOffRamp is a crypto-to-fiat off-ramp withdrawal.
	PaymentOffRamp PaymentType = "OFFRAMP"
	// PaymentGiftCard is a gift-card issuance.
	PaymentGiftCard PaymentType = "GIFTCARD"
)

// Balance is a single asset balance for the workspace.
type Balance struct {
	// Currency is the asset code, e.g. "USDT".
	Currency string `json:"currency"`
	// Available is the spendable amount as a decimal string.
	Available string `json:"available"`
	// Locked is the amount reserved/held (unavailable) as a decimal string.
	Locked string `json:"locked"`
}

// FeePreview is the fee breakdown for a given amount: the platform fee is the
// network base fee plus the account-tier markup.
type FeePreview struct {
	// Amount is the input amount the fee was computed on, as a decimal string.
	Amount string `json:"amount"`
	// Currency is the currency code of Amount.
	Currency string `json:"currency"`
	// PaymentType is the operation kind the fee applies to (see the Payment* constants).
	PaymentType PaymentType `json:"paymentType"`
	// Fee is the total fee charged, as a decimal string (NetworkFee + Markup).
	Fee string `json:"fee"`
	// Net is the amount remaining after the fee, as a decimal string.
	Net string `json:"net"`
	// Markup is the account-tier margin portion of the fee, as a decimal string.
	Markup string `json:"markup"`
	// NetworkFee is the underlying network base fee, as a decimal string.
	NetworkFee string `json:"networkFee"`
}

// PageQuery holds keyset (cursor) pagination options for list endpoints such as
// Invoices.List, Subscriptions.List, GiftCards.List, and OffRamp.Orders.
type PageQuery struct {
	// Limit is the maximum number of items per page. 0 means the server default.
	Limit int
	// Before is the opaque cursor from a previous page's Page.NextCursor. Leave it
	// empty ("") to fetch the first page.
	Before string
	// Status is an optional, endpoint-specific status filter. "" means no filter.
	Status string
}

// Page is one page of a keyset-paginated list. Items are raw JSON objects that you
// unmarshal into the concrete type you expect from that endpoint.
type Page struct {
	// Items holds the page's rows as raw JSON; unmarshal each into your target type.
	Items []json.RawMessage `json:"items"`
	// NextCursor is a response-only opaque cursor. Pass it back as PageQuery.Before
	// to fetch the next page. It is nil on the last page (no more results).
	NextCursor *string `json:"nextCursor"`
}
