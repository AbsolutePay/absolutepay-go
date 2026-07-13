package absolutepay

import (
	"context"
	"errors"
	"testing"
)

// platform-179: Preview must forward chain and require it for withdrawal/payout previews.
func TestFeesPreviewForwardsChain(t *testing.T) {
	c, cap := newStub(t, 200, `{"amount":"4.000000","currency":"USDT","paymentType":"WITHDRAWAL","fee":"0.10","net":"3.90"}`)
	if _, err := c.Fees.Preview(context.Background(), "4.000000", "USDT", PaymentWithdrawal, "MATIC"); err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want := "/v1/fees/preview?amount=4.000000&chain=MATIC&currency=USDT&paymentType=WITHDRAWAL"
	if cap.path != want {
		t.Fatalf("path = %q, want %q", cap.path, want)
	}
}

func TestFeesPreviewChainRequired(t *testing.T) {
	c, cap := newStub(t, 200, `{}`)
	for _, pt := range []PaymentType{PaymentWithdrawal, PaymentPayout} {
		_, err := c.Fees.Preview(context.Background(), "4", "USDT", pt, "")
		var apErr *Error
		if !errors.As(err, &apErr) || apErr.Code != "chain_required" || apErr.Status != 400 {
			t.Fatalf("%s: want *Error chain_required/400, got %v", pt, err)
		}
	}
	if cap.method != "" {
		t.Fatalf("no HTTP request should be made, got method %q", cap.method)
	}
}

func TestFeesPreviewOmitsChainForPayIn(t *testing.T) {
	c, cap := newStub(t, 200, `{"amount":"4","currency":"USDT","paymentType":"CHECKOUT","fee":"0.04","net":"3.96"}`)
	if _, err := c.Fees.Preview(context.Background(), "4", "USDT", "", ""); err != nil {
		t.Fatalf("Preview: %v", err)
	}
	want := "/v1/fees/preview?amount=4&currency=USDT"
	if cap.path != want {
		t.Fatalf("path = %q, want %q", cap.path, want)
	}
}
