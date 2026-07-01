package absolutepay

import "encoding/json"

// Money is a decimal-string amount plus a currency code, e.g.
// Money{Amount: "10.00", Currency: "USDT"}.
type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// PaymentType enumerates the fee-bearing operation kinds.
type PaymentType = string

const (
	PaymentCheckout     PaymentType = "CHECKOUT"
	PaymentWithdrawal   PaymentType = "WITHDRAWAL"
	PaymentSubscription PaymentType = "SUBSCRIPTION"
	PaymentConversion   PaymentType = "CONVERSION"
	PaymentOffRamp      PaymentType = "OFFRAMP"
	PaymentGiftCard     PaymentType = "GIFTCARD"
)

// Balance is one asset balance.
type Balance struct {
	Currency  string `json:"currency"`
	Available string `json:"available"`
	Locked    string `json:"locked"`
}

// FeePreview is the fee breakdown for an amount (network base + account-tier markup).
type FeePreview struct {
	Amount      string      `json:"amount"`
	Currency    string      `json:"currency"`
	PaymentType PaymentType `json:"paymentType"`
	Fee         string      `json:"fee"`
	Net         string      `json:"net"`
	Markup      string      `json:"markup"`
	NetworkFee  string      `json:"networkFee"`
}

// PageQuery holds keyset-pagination options for list endpoints. Before is the
// previous page's NextCursor; leave it empty for the first page.
type PageQuery struct {
	Limit  int    // max items per page (0 = server default)
	Before string // opaque cursor from the previous page's NextCursor
	Status string // optional status filter (endpoint-specific; "" = all)
}

// Page is one page of a keyset-paginated list. NextCursor is a response-only
// opaque cursor: pass it back as PageQuery.Before to fetch the next page. It is
// nil on the last page.
type Page struct {
	Items      []json.RawMessage `json:"items"`
	NextCursor *string           `json:"nextCursor"`
}
