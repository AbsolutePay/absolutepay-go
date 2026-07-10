package absolutepay

// Money is a monetary value: a decimal-string amount together with its currency
// code, e.g. Money{Amount: "10.00", Currency: "USDT"}. Amounts are ALWAYS strings
// (never floats) so no precision is lost on the money path.
type Money struct {
	// Amount is the decimal value as a string, e.g. "10.00" (≤6 dp). Never a float.
	Amount string `json:"amount"`
	// Currency is the asset/currency code, e.g. "USDT", "BTC", "USD".
	Currency string `json:"currency"`
}

// PaymentType enumerates the fee-bearing operation kinds. It is an alias for
// string; use the Payment* constants for the accepted values.
type PaymentType = string

// The set of PaymentType values accepted by fee/preview. Conversions and subscriptions are not
// previewable (their cost is a live quote / settled per cycle) and are rejected with 400.
const (
	// PaymentCheckout is a pay-in (customer pays the merchant).
	PaymentCheckout PaymentType = "CHECKOUT"
	// PaymentWithdrawal is an on-chain payout/withdrawal.
	PaymentWithdrawal PaymentType = "WITHDRAWAL"
	// PaymentPayout is an alias for PaymentWithdrawal (payouts and withdrawals share one fee).
	PaymentPayout PaymentType = "PAYOUT"
	// PaymentOffRamp is a crypto-to-fiat off-ramp withdrawal.
	PaymentOffRamp PaymentType = "OFFRAMP"
	// PaymentGiftCard is a gift-card issuance.
	PaymentGiftCard PaymentType = "GIFTCARD"
)

// Order is the sort direction for list endpoints. It is a string alias; use
// OrderAsc / OrderDesc (or pass "asc" / "desc" directly).
type Order = string

// Sort directions accepted by list endpoints via the order query parameter.
const (
	// OrderAsc sorts oldest-first.
	OrderAsc Order = "asc"
	// OrderDesc sorts newest-first.
	OrderDesc Order = "desc"
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

// FeePreview is the total fee (and net) for a given amount.
type FeePreview struct {
	// Amount is the input amount the fee was computed on, as a decimal string.
	Amount string `json:"amount"`
	// Currency is the currency code of Amount.
	Currency string `json:"currency"`
	// PaymentType is the operation kind the fee applies to (see the Payment* constants).
	PaymentType PaymentType `json:"paymentType"`
	// Fee is the total fee charged on the amount, as a decimal string.
	Fee string `json:"fee"`
	// Net is the amount remaining after the fee, as a decimal string.
	Net string `json:"net"`
}

// Page is one page of a cursor-paginated list. T is the element type; loosely
// typed lists use Page[JSON]. Callers page by echoing NextCursor back as the
// query's Before; an empty NextCursor means the last page.
type Page[T any] struct {
	// Items holds this page's rows.
	Items []T `json:"items"`
	// NextCursor is the opaque cursor for the next page. Pass it back as the list
	// query's Before. It is "" (null on the wire) on the last page.
	NextCursor string `json:"nextCursor"`
	// Total is the true count for the filtered range, independent of paging. It is
	// only populated by the ledger lists (refunds, conversions) and reconciliation.
	Total int `json:"total,omitempty"`
}

// ListQuery holds the shared cursor-pagination + filter options for the resource
// lists that support a status/search filter: Checkouts, Invoices, GiftCards,
// Subscriptions, and OffRamp.Orders. All fields are optional.
type ListQuery struct {
	// Limit is the maximum number of items per page. 0 means the server default.
	Limit int
	// Before is the opaque cursor from a previous page's Page.NextCursor. "" fetches
	// the first page.
	Before string
	// Order is the sort direction, OrderAsc or OrderDesc. "" means the server default.
	Order Order
	// Status is an optional, endpoint-specific status filter. "" means no filter.
	Status string
	// Q is an optional free-text search filter. "" means no filter.
	Q string
}

func (q ListQuery) values() map[string]string {
	m := map[string]string{"before": q.Before, "order": q.Order, "status": q.Status, "q": q.Q}
	putInt(m, "limit", q.Limit)
	return m
}

// LedgerQuery filters the settled ledger-history lists (Refunds.List and
// Conversions.List). All fields are optional; zero values are omitted.
type LedgerQuery struct {
	// From is the inclusive start of the time range, in epoch milliseconds (0 = unbounded).
	From int64
	// To is the inclusive end of the time range, in epoch milliseconds (0 = unbounded).
	To int64
	// Currency filters to a single asset code, e.g. "USDT". "" = all currencies.
	Currency string
	// Limit is the maximum number of items per page. 0 means the server default.
	Limit int
	// Before is the opaque cursor from a previous page's Page.NextCursor.
	Before string
	// Order is the sort direction, OrderAsc or OrderDesc.
	Order Order
}

func (q LedgerQuery) values() map[string]string {
	m := map[string]string{"currency": q.Currency, "before": q.Before, "order": q.Order}
	putInt64(m, "from", q.From)
	putInt64(m, "to", q.To)
	putInt(m, "limit", q.Limit)
	return m
}

// DepositHistoryQuery filters the settled deposit history (Deposits.List). All
// fields are optional; zero values are omitted.
type DepositHistoryQuery struct {
	// Chain filters to a single blockchain network, e.g. "TRX". "" = all chains.
	Chain string
	// From is the inclusive start of the time range, in epoch milliseconds (0 = unbounded).
	From int64
	// To is the inclusive end of the time range, in epoch milliseconds (0 = unbounded).
	To int64
	// Before is the opaque cursor from a previous page's Page.NextCursor.
	Before string
	// Order is the sort direction, OrderAsc or OrderDesc.
	Order Order
}

func (q DepositHistoryQuery) values() map[string]string {
	m := map[string]string{"chain": q.Chain, "before": q.Before, "order": q.Order}
	putInt64(m, "from", q.From)
	putInt64(m, "to", q.To)
	return m
}

// AddressQuery filters the workspace's minted deposit addresses (Deposits.Addresses).
// All fields are optional; zero values are omitted.
type AddressQuery struct {
	// Chain filters to a single blockchain network, e.g. "TRX". "" = all chains.
	Chain string
	// Limit is the maximum number of items per page. 0 means the server default.
	Limit int
	// Before is the opaque cursor from a previous page's Page.NextCursor.
	Before string
	// Order is the sort direction, OrderAsc or OrderDesc.
	Order Order
}

func (q AddressQuery) values() map[string]string {
	m := map[string]string{"chain": q.Chain, "before": q.Before, "order": q.Order}
	putInt(m, "limit", q.Limit)
	return m
}

// ReconciliationQuery filters a settlement reconciliation report by time range and
// page. All fields are optional; zero values are omitted.
type ReconciliationQuery struct {
	// From is the inclusive start of the time range, in epoch milliseconds (0 = unbounded).
	From int64
	// To is the inclusive end of the time range, in epoch milliseconds (0 = unbounded).
	To int64
	// Limit is the maximum number of rows per page. 0 means the server default.
	Limit int
	// Before is the opaque cursor from a previous page's Page.NextCursor.
	Before string
	// Order is the sort direction, OrderAsc or OrderDesc.
	Order Order
}

func (q ReconciliationQuery) values() map[string]string {
	m := map[string]string{"before": q.Before, "order": q.Order}
	putInt64(m, "from", q.From)
	putInt64(m, "to", q.To)
	putInt(m, "limit", q.Limit)
	return m
}
