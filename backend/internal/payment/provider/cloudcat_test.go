package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

func TestCloudcatCreatePaymentUsesMappedProductAndEmail(t *testing.T) {
	provider, requests, closeServer := newCloudcatTestProvider(t, map[string]string{
		"/shopApi/Shop/goodsInfo":      `{"code":1,"data":{"goods_type":"card","price":5,"status":1,"stock":8,"user":{"token":"merchant-1"}}}`,
		"/shopApi/Shop/getUserChannel": `{"code":1,"data":[{"id":1,"code":"AlipayPc","status":1}]}`,
		"/shopApi/Shop/getGoodsPrice":  `{"code":1,"data":{"total_amount":5}}`,
		"/shopApi/Pay/order":           `{"code":1,"data":{"trade_no":"CAT-1","payurl":"https://catfk.com/pay/CAT-1","total_amount":5}}`,
		"/pay/CAT-1":                   `<form action="https://openapi.alipay.com/gateway.do?charset=UTF-8" method="POST"><input name="method" value="alipay.trade.page.pay"><input name="sign" value="signed"></form>`,
		"/gateway.do":                  `<html><input value="https://qr.alipay.com/cloudcat-test-1" id="J_qrCode"></html>`,
	})
	defer closeServer()

	result, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		Amount: "5.00", PaymentType: payment.TypeAlipay, PayerEmail: "buyer@example.com",
	})
	if err != nil {
		t.Fatalf("CreatePayment returned error: %v", err)
	}
	if result.TradeNo != "CAT-1" || result.PayURL != "https://catfk.com/pay/CAT-1" || result.QRCode != "https://qr.alipay.com/cloudcat-test-1" {
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

func TestExtractCloudcatPaymentQRCodeValidatesAlipayPayload(t *testing.T) {
	qrCode, err := extractCloudcatPaymentQRCode(strings.NewReader(`<input value="https://qr.alipay.com/order?a=1&amp;b=2" id="J_qrCode">`))
	if err != nil {
		t.Fatalf("extractCloudcatPaymentQRCode returned error: %v", err)
	}
	if qrCode != "https://qr.alipay.com/order?a=1&b=2" {
		t.Fatalf("qr code = %q", qrCode)
	}

	_, err = extractCloudcatPaymentQRCode(strings.NewReader(`<input id="J_qrCode" value="https://evil.example/order">`))
	if err == nil {
		t.Fatal("extractCloudcatPaymentQRCode should reject an untrusted QR host")
	}
}

func newCloudcatTestProvider(t *testing.T, responses map[string]string) (*Cloudcat, map[string]map[string]any, func()) {
	t.Helper()
	requests := make(map[string]map[string]any)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
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
		}
		body, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/shopApi/") {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/html")
		}
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
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("parse test server URL: %v", err)
	}
	provider.checkoutClient.Transport = cloudcatTestRoundTripper(func(r *http.Request) (*http.Response, error) {
		cloned := r.Clone(r.Context())
		rewritten := *r.URL
		rewritten.Scheme = serverURL.Scheme
		rewritten.Host = serverURL.Host
		cloned.URL = &rewritten
		resp, err := http.DefaultTransport.RoundTrip(cloned)
		if resp != nil {
			resp.Request = r
		}
		return resp, err
	})
	provider.httpClient.Transport = provider.checkoutClient.Transport
	return provider, requests, server.Close
}

type cloudcatTestRoundTripper func(*http.Request) (*http.Response, error)

func (f cloudcatTestRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCloudcatCheckoutSeparatesCatFKAndAlipayTransports(t *testing.T) {
	for _, redirect := range []bool{false, true} {
		t.Run(fmt.Sprintf("untrusted_redirect=%t", redirect), func(t *testing.T) {
			catCalls, alipayCalls := 0, 0
			p := &Cloudcat{
				httpClient: &http.Client{Transport: cloudcatTestRoundTripper(func(r *http.Request) (*http.Response, error) {
					catCalls++
					if r.URL.Host != "catfk.com" || r.Method != http.MethodGet {
						t.Fatalf("unexpected CatFK request: %s %s", r.Method, r.URL)
					}
					if redirect {
						return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"https://evil.example/pay"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
					}
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<form action="https://openapi.alipay.com/gateway.do" method="POST"><input name="sign" value="signed"></form>`)), Request: r}, nil
				})},
				checkoutClient: &http.Client{Transport: cloudcatTestRoundTripper(func(r *http.Request) (*http.Response, error) {
					alipayCalls++
					if r.URL.Host != "openapi.alipay.com" || r.Method != http.MethodPost {
						t.Fatalf("unexpected Alipay request: %s %s", r.Method, r.URL)
					}
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`<input id="J_qrCode" value="https://qr.alipay.com/checkout-test">`)), Request: r}, nil
				})},
			}
			qrCode, err := p.ResolvePaymentQRCode(context.Background(), "https://catfk.com/pay/CAT-1")
			if catCalls != 1 {
				t.Fatalf("CatFK calls = %d, want 1", catCalls)
			}
			if redirect {
				if err == nil || alipayCalls != 0 {
					t.Fatalf("untrusted redirect: error=%v, Alipay calls=%d", err, alipayCalls)
				}
			} else if err != nil || qrCode != "https://qr.alipay.com/checkout-test" || alipayCalls != 1 {
				t.Fatalf("checkout: QR=%q, error=%v, Alipay calls=%d", qrCode, err, alipayCalls)
			}
		})
	}
}

func TestCloudcatRetriesOnlyTransientReadFailures(t *testing.T) {
	for _, tt := range []struct {
		name           string
		path           string
		status         int
		transportError bool
		wantCalls      int
		wantError      bool
	}{
		{"read EOF", "/shopApi/Shop/goodsInfo", 0, true, 3, false},
		{"read unavailable", "/shopApi/Pay/query", 503, false, 3, false},
		{"read rate limited", "/shopApi/Order/info", 429, false, 3, false},
		{"read rejected", "/shopApi/Shop/goodsInfo", 400, false, 1, true},
		{"create EOF is ambiguous", "/shopApi/Pay/order", 0, true, 1, true},
		{"create unavailable is not replayed", "/shopApi/Pay/order", 503, false, 1, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			p := &Cloudcat{apiBase: cloudcatAPIBase, httpClient: &http.Client{Transport: cloudcatTestRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls < 3 && tt.transportError {
					return nil, io.EOF
				}
				status := http.StatusOK
				if calls < 3 {
					status = tt.status
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"code":1}`)), Header: make(http.Header), Request: r}, nil
			})}}
			var result cloudcatOrderData
			err := p.post(context.Background(), tt.path, map[string]string{"trade_no": "CAT-1"}, &result)
			if (err != nil) != tt.wantError || calls != tt.wantCalls {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestCloudcatReadRetryStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	p := &Cloudcat{apiBase: cloudcatAPIBase, httpClient: &http.Client{Transport: cloudcatTestRoundTripper(func(*http.Request) (*http.Response, error) {
		calls++
		cancel()
		return nil, context.Canceled
	})}}
	var result cloudcatOrderData
	start := time.Now()
	err := p.post(ctx, "/shopApi/Pay/query", nil, &result)
	if !errors.Is(err, context.Canceled) || calls != 1 || time.Since(start) > time.Second {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestCloudcatDoesNotStartAnOrderWithAnExhaustedPreflightBudget(t *testing.T) {
	p, requests, closeServer := newCloudcatTestProvider(t, map[string]string{
		"/shopApi/Shop/goodsInfo":      `{"code":1,"data":{"goods_type":"card","price":5,"status":1,"stock":8,"user":{"token":"merchant-1"}}}`,
		"/shopApi/Shop/getUserChannel": `{"code":1,"data":[{"id":1,"code":"AlipayPc","status":1}]}`,
		"/shopApi/Shop/getGoodsPrice":  `{"code":1,"data":{"total_amount":5}}`,
	})
	defer closeServer()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := p.CreatePayment(ctx, payment.CreatePaymentRequest{Amount: "5.00", PayerEmail: "buyer@example.com"})
	if err == nil || !strings.Contains(err.Error(), "preflight") {
		t.Fatalf("error=%v", err)
	}
	if _, created := requests["/shopApi/Pay/order"]; created {
		t.Fatal("order started without enough time to receive its trade number")
	}
}

func TestCloudcatPaidQueryRejectsInvalidOrderDetails(t *testing.T) {
	for _, info := range []string{
		`{"code":0,"data":{"total_amount":5,"cards":["CDK-123"]}}`,
		`{"code":1,"data":{"trade_no":"CAT-OTHER","total_amount":5,"cards":["CDK-123"]}}`,
	} {
		p, _, closeServer := newCloudcatTestProvider(t, map[string]string{
			"/shopApi/Pay/query":  `{"code":1,"data":{"trade_no":"CAT-1","total_amount":5}}`,
			"/shopApi/Order/info": info,
		})
		_, err := p.QueryOrder(context.Background(), "CAT-1")
		closeServer()
		if err == nil {
			t.Fatal("invalid paid order details accepted")
		}
	}
}
