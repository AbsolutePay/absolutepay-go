# absolutepay-go

Official AbsolutePay API client for Go. Server-side only — your API key and signing secret must never reach a browser.

> Every request from an app key is HMAC-signed automatically. Inbound webhooks are verified with one call. **Zero third-party dependencies** — standard library only.

## Install

```bash
go get github.com/AbsolutePay/absolutepay-go@latest
```

Requires **Go 1.18+**.

```go
import absolutepay "github.com/AbsolutePay/absolutepay-go"
```

## Environments

| Option | Base URL |
|---|---|
| default | `https://api.absolutepay.io` (production) |
| `WithSandbox(true)` | `https://sandbox-api.absolutepay.io` |
| `WithBaseURL("https://…")` | your override (wins over sandbox) |

## Quickstart

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	absolutepay "github.com/AbsolutePay/absolutepay-go"
)

func main() {
	ap, err := absolutepay.New(
		os.Getenv("ABSOLUTEPAY_API_KEY"), // ap_live_… / ap_test_…
		absolutepay.WithSigningSecret(os.Getenv("ABSOLUTEPAY_SIGNING_SECRET")), // apisign_…
		// absolutepay.WithSandbox(true),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	balances, err := ap.Balances.List(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(balances)

	inv, err := ap.Invoices.Create(ctx, absolutepay.InvoiceParams{
		Reference:   "order-123",
		Amount:      absolutepay.Money{Amount: "25.00", Currency: "USDT"},
		Chain:       "MATIC", // REQUIRED — mints the deposit address up front
		RedirectURL: "https://shop.example.com/thanks", // payer returns here after checkout (?token=…&status=…)
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(inv["token"], inv["address"])
}
```

## Checkouts vs. invoices

Two symmetric resources create a payable link:

- **`ap.Checkouts`** — a hosted page where the payer picks the asset **and** network at
  pay time. No `chain`; you get a `checkoutUrl` to redirect the customer to.
- **`ap.Invoices`** — an up-front flow: you pass a **required `chain`** and get a fixed
  deposit `address` back immediately.

Both expose the same lifecycle: `Create` / `List` / `Get` / `Update` / `Delete`.
`Update` pauses/resumes or edits the link; `Delete` voids it. Confirm payment via the
`payment.succeeded` webhook or by polling `Get(ctx, token)`.

```go
link, err := ap.Checkouts.Create(ctx, absolutepay.CheckoutParams{
	Reference: "order-123",
	Amount:    absolutepay.Money{Amount: "25.00", Currency: "USDT"},
})
if err != nil {
	log.Fatal(err)
}
fmt.Println(link["checkoutUrl"]) // send the payer here; they choose how to pay

// Pause the link, then void it.
paused := true
_, _ = ap.Checkouts.Update(ctx, link["token"].(string), absolutepay.ResourceUpdate{Paused: &paused})
_ = ap.Checkouts.Delete(ctx, link["token"].(string))
```

## Idempotency

Money-moving POSTs (`Payouts.Create`, `Refunds.Create`, `Conversions.Execute`,
`OffRamp.Withdraw`, `GiftCards.Create`, `Subscriptions.Create`, `Subscriptions.CreatePlan`)
accept `WithIdempotencyKey`, which sets the `Idempotency-Key` header. Replaying with the
same key returns the original result instead of acting twice; a `409` (in-progress or
conflicting replay) surfaces as a normal `*absolutepay.Error` you can inspect via `.Code`.

```go
_, err := ap.Refunds.Create(ctx, absolutepay.RefundParams{
	MerchantTradeNo: "order-123",
	Amount:          absolutepay.Money{Amount: "10.00", Currency: "USDT"},
}, absolutepay.WithIdempotencyKey("refund-order-123-1"))
```

## Errors

Non-2xx responses return an `*absolutepay.Error`:

```go
_, err := ap.Payouts.Create(ctx, items)
var apErr *absolutepay.Error
if errors.As(err, &apErr) {
	fmt.Println(apErr.Status, apErr.Code, apErr.Detail, apErr.RequestID)
	if apErr.IsRateLimited() { /* 429 — back off and retry */ }
	if apErr.IsAuth()        { /* 401/403 — check key, scope, or signature */ }
}
```

## Pagination

Lists return a generic `Page[T]` with `{ Items, NextCursor }` (the ledger lists —
`Refunds.List`, `Conversions.List` — and reconciliation also carry `Total`). Pass a
prior page's `NextCursor` back as the query's `Before`; it is `""` on the last page.
Loosely-typed lists come back as `Page[JSON]` (`JSON` = `map[string]any`); read fields
by key.

```go
var before string
for {
	page, err := ap.Invoices.List(ctx, absolutepay.ListQuery{
		Limit:  50,
		Before: before,
		Order:  absolutepay.OrderDesc,
		Status: "OPEN", // optional filter
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, item := range page.Items {
		fmt.Println(item["token"], item["amount"])
	}
	if page.NextCursor == "" {
		break
	}
	before = page.NextCursor
}
```

## Webhooks

Verify the signature and parse the event in one call. Pass the **raw** request body:

```go
func handler(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body) // RAW bytes — do not re-serialize
	event, err := absolutepay.ConstructEvent(raw, r.Header, os.Getenv("ABSOLUTEPAY_WEBHOOK_SECRET"))
	if err != nil {
		http.Error(w, "bad signature", http.StatusBadRequest)
		return
	}
	if event.Type == "payment.succeeded" {
		// json.Unmarshal(event.Data, &yourType)
	}
	w.WriteHeader(http.StatusOK)
}
```

The freshness (replay) window defaults to 5 minutes; pass `absolutepay.WithTolerance(0)` to disable it.

## Security

- **Server-side only.** The API key + signing secret authenticate as your workspace.
- Requests go over HTTPS only (except `localhost` for local development).
- The `Idempotency-Key` header (on money POSTs) is intentionally **not** part of the signed canonical string.

## License

MIT
