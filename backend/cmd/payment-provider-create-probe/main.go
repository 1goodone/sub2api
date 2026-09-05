package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/payment/provider"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: payment-provider-create-probe <out-trade-no>")
		os.Exit(2)
	}

	var config map[string]string
	if err := json.NewDecoder(os.Stdin).Decode(&config); err != nil {
		fmt.Fprintln(os.Stderr, "decode config:", err)
		os.Exit(1)
	}
	p, err := provider.NewAlipay("probe", config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create provider:", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := p.CreatePayment(ctx, payment.CreatePaymentRequest{
		OrderID: os.Args[1],
		Amount:  "0.01",
		Subject: "Payment channel verification",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "create payment:", err)
		os.Exit(1)
	}
	fmt.Printf("qr=%t pay_url=%t\n", strings.TrimSpace(result.QRCode) != "", strings.TrimSpace(result.PayURL) != "")
}
