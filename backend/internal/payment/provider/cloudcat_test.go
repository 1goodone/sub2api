package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestCloudcatCreatePaymentUsesMappedProductAndEmail(t *testing.T) {
	provider, requests, closeServer := newCloudcatTestProvider(t, map[string]string{
		"/shopApi/Shop/goodsInfo":      `{"code":1,"data":{"goods_type":"card","price":5,"status":1,"stock":8,"user":{"token":"merchant-1"}}}`,
		"/shopApi/Shop/getUserChannel": `{"code":1,"data":[{"id":1,"code":"AlipayPc","status":1}]}`,
		"/shopApi/Shop/getGoodsPrice":  `{"code":1,"data":{"total_amount":5}}`,
		"/shopApi/Pay/order":           `{"code":1,"data":{"trade_no":"CAT-1","payurl":"https://catfk.com/pay/CAT-1","total_amount":5}}`,
	})
	defer closeServer()

	result, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Amount: "5.00", PaymentType: payment.TypeAlipay, PayerEmail: "buyer@example.com",
	})
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	if result.TradeNo != "CAT-1" || result.PayURL != "https://catfk.com/pay/CAT-1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	orderRequest := requests["/shopApi/Pay/order"]
	if orderRequest["goods_key"] != "ee9c69" || orderRequest["contact"] != "buyer@example.com" {
		t.Fatalf("unexpected order payload: %#v", orderRequest)
	}
	if orderRequest["quantity"] != json.Number("1") || orderRequest["channel_id"] != json.Number("1") {
		t.Fatalf("unexpected quantity/channel payload: %#v", orderRequest)
	}
}

func TestCloudcatQueryOrderReturnsPaidBeforeAndAfterCardDelivery(t *testing.T) {
	tests := []struct {
		name     string
		infoBody string
		wantCode string
	}{
		{name: "awaiting delivery", infoBody: `{"code":1,"data":{"response":{"trade_no":"CAT-1","total_amount":5,"cards":[]}}}`},
		{name: "card delivered", infoBody: `{"code":1,"data":{"response":{"trade_no":"CAT-1","total_amount":5,"cards":["CDK-123"]}}}`, wantCode: "CDK-123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, _, closeServer := newCloudcatTestProvider(t, map[string]string{
				"/shopApi/Pay/query":  `{"code":1,"data":{"trade_no":"CAT-1","total_amount":5}}`,
				"/shopApi/Order/info": tt.infoBody,
			})
			defer closeServer()

			result, err := provider.QueryOrder(context.Background(), "CAT-1")
			if err != nil {
				t.Fatalf("QueryOrder returned error: %v", err)
			}
			if result.Status != payment.ProviderStatusPaid || result.Amount != 5 {
				t.Fatalf("unexpected result: %+v", result)
			}
			if got := result.Metadata[payment.MetadataCloudcatRechargeCode]; got != tt.wantCode {
				t.Fatalf("metadata code = %q, want %q", got, tt.wantCode)
			}
		})
	}
}

func TestCloudcatRejectsUntrustedPaymentURL(t *testing.T) {
	provider, _, closeServer := newCloudcatTestProvider(t, map[string]string{
		"/shopApi/Shop/goodsInfo":      `{"code":1,"data":{"goods_type":"card","price":5,"status":1,"stock":8,"user":{"token":"merchant-1"}}}`,
		"/shopApi/Shop/getUserChannel": `{"code":1,"data":[{"id":1,"code":"AlipayPc","status":1}]}`,
		"/shopApi/Shop/getGoodsPrice":  `{"code":1,"data":{"total_amount":5}}`,
		"/shopApi/Pay/order":           `{"code":1,"data":{"trade_no":"CAT-1","payurl":"https://evil.example/pay/CAT-1","total_amount":5}}`,
	})
	defer closeServer()

	if _, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{Amount: "5.00", PayerEmail: "buyer@example.com"}); err == nil {
		t.Fatal("CreatePayment should reject an untrusted payment URL")
	}
}

func TestCloudcatVisitorIDSurvivesProviderRecreation(t *testing.T) {
	config := map[string]string{
		"channelId": "1",
		"products":  `[{"amount":"5.00","itemUrl":"https://catfk.com/item/ee9c69"}]`,
	}
	first, err := NewCloudcat("1", config)
	if err != nil {
		t.Fatalf("NewCloudcat returned error: %v", err)
	}
	second, err := NewCloudcat("1", config)
	if err != nil {
		t.Fatalf("NewCloudcat returned error: %v", err)
	}
	other, err := NewCloudcat("2", config)
	if err != nil {
		t.Fatalf("NewCloudcat returned error: %v", err)
	}

	if first.visitorID != second.visitorID {
		t.Fatalf("visitor ID changed across provider recreation: %q != %q", first.visitorID, second.visitorID)
	}
	if first.visitorID == other.visitorID {
		t.Fatalf("different provider instances share visitor ID %q", first.visitorID)
	}
}

func newCloudcatTestProvider(t *testing.T, responses map[string]string) (*Cloudcat, map[string]map[string]any, func()) {
	t.Helper()
	requests := make(map[string]map[string]any)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			t.Errorf("decode %s request: %v", r.URL.Path, err)
		}
		requests[r.URL.Path] = payload
		if len(r.Header.Get("Visitorid")) != 9 {
			t.Errorf("Visitorid length = %d, want 9", len(r.Header.Get("Visitorid")))
		}
		body, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	provider, err := NewCloudcat("1", map[string]string{
		"channelId": "1",
		"products":  `[{"amount":"5.00","itemUrl":"https://catfk.com/item/ee9c69"}]`,
	})
	if err != nil {
		server.Close()
		t.Fatalf("NewCloudcat returned error: %v", err)
	}
	provider.apiBase = server.URL
	return provider, requests, server.Close
}
