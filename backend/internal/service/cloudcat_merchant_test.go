package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type cloudcatMerchantTestEncryptor struct{}

func (cloudcatMerchantTestEncryptor) Encrypt(value string) (string, error) {
	return "cipher:" + base64.RawStdEncoding.EncodeToString([]byte(value)), nil
}

func (cloudcatMerchantTestEncryptor) Decrypt(value string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, "cipher:"))
	return string(raw), err
}

func TestCloudcatMerchantLoginStoresEncryptedSessionAndReadsOverview(t *testing.T) {
	redisServer := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	var checkTokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/merchantApi/user/login":
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatalf("decode login body: %v", err)
			}
			if input["username"] != "merchant@example.com" || input["password"] != "secret" {
				t.Fatalf("unexpected login payload: %#v", input)
			}
			http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: "session-cookie", Path: "/"})
			_, _ = w.Write([]byte(`{"code":1,"data":{"merchant_token":"merchant-token"}}`))
		case "/merchantApi/user/checkToken":
			checkTokenCalls.Add(1)
			if r.Header.Get("Merchant-Token") != "merchant-token" || r.Header.Get("Cookie") != "PHPSESSID=session-cookie" {
				t.Fatalf("session headers missing: token=%q cookie=%q", r.Header.Get("Merchant-Token"), r.Header.Get("Cookie"))
			}
			// The merchant frontend treats only code 0 as an invalid token.
			_, _ = w.Write([]byte(`{"code":2}`))
		case "/merchantApi/dashboard/workplace":
			_, _ = w.Write([]byte(`{"code":1,"data":{"statistics":{"today_amount":"12.50","today_count":"3"}}}`))
		case "/merchantApi/wallet/info":
			_, _ = w.Write([]byte(`{"code":1,"data":{"platform":{"available_money":"100.25","freeze_money":7.75}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := newCloudcatMerchantServiceWithBaseURL(rdb, cloudcatMerchantTestEncryptor{}, server.URL)
	svc.sessionTTL = 2 * time.Hour
	overview, err := svc.Login(context.Background(), 42, CloudcatMerchantLoginRequest{
		Username: "merchant@example.com",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if overview == nil || !overview.Connected || overview.TodayOrderCount != 3 || overview.TodayOrderAmount != 12.5 || overview.WithdrawableAmount != 100.25 || overview.FrozenAmount != 7.75 || overview.Currency != "CNY" {
		t.Fatalf("unexpected overview: %+v", overview)
	}
	if checkTokenCalls.Load() != 1 {
		t.Fatalf("checkToken calls = %d, want 1", checkTokenCalls.Load())
	}
	raw, err := rdb.Get(context.Background(), svc.sessionKey(42)).Result()
	if err != nil {
		t.Fatalf("read Redis session: %v", err)
	}
	if strings.Contains(raw, "merchant-token") || strings.Contains(raw, "session-cookie") {
		t.Fatalf("Redis session contains plaintext credentials: %q", raw)
	}

	read, err := svc.GetOverview(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetOverview returned error: %v", err)
	}
	if read.TodayOrderCount != 3 || read.WithdrawableAmount != 100.25 {
		t.Fatalf("unexpected refreshed overview: %+v", read)
	}
	if ttl := redisServer.TTL(svc.sessionKey(42)); ttl <= 0 || ttl > 2*time.Hour {
		t.Fatalf("session TTL = %v", ttl)
	}
}

func TestCloudcatMerchantOverviewWithoutSessionNeedsLogin(t *testing.T) {
	redisServer := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	svc := newCloudcatMerchantServiceWithBaseURL(rdb, cloudcatMerchantTestEncryptor{}, "https://catfk.com")
	overview, err := svc.GetOverview(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetOverview returned error: %v", err)
	}
	if overview == nil || !overview.NeedsLogin || overview.Connected {
		t.Fatalf("unexpected unauthenticated overview: %+v", overview)
	}
}

func TestCloudcatMerchantInvalidTokenClearsSession(t *testing.T) {
	redisServer := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/merchantApi/user/checkToken" {
			_, _ = w.Write([]byte(`{"code":0,"msg":"Token失效！"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	svc := newCloudcatMerchantServiceWithBaseURL(rdb, cloudcatMerchantTestEncryptor{}, server.URL)
	if err := svc.saveSession(context.Background(), 9, &cloudcatMerchantSession{MerchantToken: "stale-token"}); err != nil {
		t.Fatalf("saveSession returned error: %v", err)
	}
	overview, err := svc.GetOverview(context.Background(), 9)
	if err != nil {
		t.Fatalf("GetOverview returned error: %v", err)
	}
	if overview == nil || !overview.NeedsLogin {
		t.Fatalf("unexpected overview: %+v", overview)
	}
	if exists, err := rdb.Exists(context.Background(), svc.sessionKey(9)).Result(); err != nil || exists != 0 {
		t.Fatalf("stale session remains: exists=%d err=%v", exists, err)
	}
}

func TestCloudcatMerchantRejectsRedirects(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Store(true)
		_, _ = w.Write([]byte(`{"code":1,"data":{"merchant_token":"leaked"}}`))
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()
	redisServer := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	svc := newCloudcatMerchantServiceWithBaseURL(rdb, cloudcatMerchantTestEncryptor{}, source.URL)
	if _, err := svc.Login(context.Background(), 1, CloudcatMerchantLoginRequest{Username: "u", Password: "p"}); err == nil {
		t.Fatal("Login should reject redirect")
	}
	if redirected.Load() {
		t.Fatal("redirect target received merchant credentials")
	}
}
