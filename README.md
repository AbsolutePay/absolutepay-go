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
		Chain:       "MATIC", // mint the deposit address up front; omit to let the payer pick
		RedirectURL: "https://shop.example.com/thanks", // payer returns here after checkout (?token=…&status=…)
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(inv["token"])
}
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

Live lists use keyset pagination. Pass a prior page's `NextCursor` as `PageQuery.Before`; it's `nil` on the last page.

```go
var before string
for {
	page, err := ap.Invoices.List(ctx, absolutepay.PageQuery{Limit: 50, Before: before})
	if err != nil {
		log.Fatal(err)
	}
	for _, raw := range page.Items {
		// json.Unmarshal(raw, &yourType)
	}
	if page.NextCursor == nil {
		break
	}
	before = *page.NextCursor
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
- The `Idempotency-Key` header (on payouts) is intentionally **not** part of the signed canonical string.

## License

MIT
