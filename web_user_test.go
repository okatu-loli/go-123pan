package pan123_test

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pan123 "github.com/okatu-loli/go-123pan"
)

// Public SDK -> real HTTP provider scenarios. The independent provider verifies
// the observed website protocol; no client/token internals are replaced.
// Failure modes: wrong identity/host/platform/path, invalid signature, response
// envelope/status failures, lost missing/zero precision, transport cancellation,
// accidental login/recalculation, and leaked input in errors.
func checkWebRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodGet || r.URL.Path != "/b/api/user/info" || r.ContentLength > 0 {
		t.Errorf("unexpected website request method/path/body")
	}
	if r.Header.Get("Authorization") != "Bearer web-fixture-token" || r.Header.Get("Platform") != "web" || r.Header.Get("App-Version") != "3" {
		t.Errorf("website authentication headers do not match the web identity")
	}
	if r.Header.Get("Origin") != "https://yun.123pan.cn" || r.Header.Get("Referer") != "https://yun.123pan.cn/" {
		t.Errorf("website origin/referer mismatch")
	}
	query := r.URL.Query()
	if len(query) != 1 {
		t.Error("website request requires exactly one dynamic signature parameter")
		return
	}
	for key, values := range query {
		if len(values) != 1 {
			t.Error("duplicate signature value")
			continue
		}
		parts := strings.Split(values[0], "-")
		if len(parts) != 3 {
			t.Error("invalid signature components")
			continue
		}
		seconds, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || time.Since(time.Unix(seconds, 0)).Abs() > time.Minute {
			t.Error("signature timestamp is not current")
		}
		nonce, err := strconv.Atoi(parts[1])
		if err != nil || nonce < 0 || nonce > 10_000_000 {
			t.Error("signature nonce is outside the website range")
		}
		minute := time.Unix(seconds, 0).UTC().Add(8 * time.Hour).Format("200601021504")
		var encoded strings.Builder
		for _, digit := range minute {
			encoded.WriteByte("adefghlmyi"[digit-'0'])
		}
		expectedKey := strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(encoded.String()))), 10)
		payload := strings.Join([]string{parts[0], parts[1], "/b/api/user/info", "web", "3", expectedKey}, "|")
		expectedCRC := strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(payload))), 10)
		if key != expectedKey || parts[2] != expectedCRC {
			t.Error("website signature rejected by the provider")
		}
	}
}

func TestUserWebInfoUsesSeparateIdentityAndCapturedProtocolFields(t *testing.T) {
	var openCalls, openTokenCalls, webCalls atomic.Int32
	open := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/access_token" {
			openTokenCalls.Add(1)
			fmt.Fprintf(w, `{"code":0,"data":{"accessToken":"open-fixture-token","expiredAt":%q}}`, time.Now().Add(time.Hour).Format(time.RFC3339))
			return
		}
		openCalls.Add(1)
		if r.Method != "GET" || r.URL.Path != "/api/v1/user/info" || r.Header.Get("Platform") != "open_platform" || r.Header.Get("Authorization") != "Bearer open-fixture-token" || len(r.URL.Query()) != 0 {
			t.Error("existing Open user info semantics changed")
		}
		fmt.Fprint(w, `{"code":0,"data":{"uid":42,"nickname":"Open fixture"}}`)
	}))
	defer open.Close()
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webCalls.Add(1)
		checkWebRequest(t, r)
		if r.UserAgent() != "SDK web fixture" {
			t.Error("WithUserAgent was not applied")
		}
		fmt.Fprint(w, `{"code":0,"message":"ok","data":{"SpacePermanent":12345678901234,"SpacePermanentTotal":"12345678901234","SpaceTemp":0,"SpaceUsed":123456789,"SpaceUsedTotal":"123456789","SpaceTempExpr":"2030-01-02 03:04:05"}}`)
	}))
	defer web.Close()
	client := pan123.New("open-fixture-id", "open-fixture-secret", pan123.WithBaseURL(open.URL), pan123.WithWebBaseURL(web.URL), pan123.WithHTTPClient(web.Client()), pan123.WithUserAgent("SDK web fixture"))
	for i := range 2 {
		info, err := client.User.WebInfo(context.Background(), "web-fixture-token")
		if err != nil {
			t.Fatalf("WebInfo: %v", err)
		}
		if info.SpacePermanent == nil || *info.SpacePermanent != 12345678901234 || info.SpaceUsed == nil || *info.SpaceUsed != 123456789 || info.SpaceTemp == nil || *info.SpaceTemp != 0 || info.SpacePermanentTotal == nil || *info.SpacePermanentTotal != "12345678901234" || info.SpaceUsedTotal == nil || *info.SpaceUsedTotal != "123456789" || string(info.SpaceTempExpr) != `"2030-01-02 03:04:05"` {
			t.Fatal("captured capacity fields were not preserved")
		}
		if i == 0 && (client.Token() != "" || openTokenCalls.Load() != 0 || openCalls.Load() != 0) {
			t.Fatal("website query fetched or changed Open credentials")
		}
		openInfo, err := client.User.Info(context.Background())
		if err != nil || openInfo.UID != 42 || client.Token() != "open-fixture-token" {
			t.Fatal("website query affected Open token state")
		}
	}
	if webCalls.Load() != 2 || openCalls.Load() != 2 || openTokenCalls.Load() != 1 {
		t.Fatal("unexpected login, refresh, recalculation, or extra API requests")
	}
}

func TestUserWebInfoPreservesUnknownZeroAndLargeValues(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		check      func(*testing.T, *pan123.WebUserInfo)
	}{
		{"missing", `{}`, func(t *testing.T, v *pan123.WebUserInfo) {
			if v.SpacePermanent != nil || v.SpaceUsed != nil || v.SpaceTemp != nil || v.SpacePermanentTotal != nil || v.SpaceUsedTotal != nil || v.SpaceTempExpr != nil {
				t.Fatal("missing fields became measured zero")
			}
		}},
		{"null", `{"SpacePermanent":null,"SpaceUsed":null,"SpaceTemp":null,"SpacePermanentTotal":null,"SpaceUsedTotal":null,"SpaceTempExpr":null}`, func(t *testing.T, v *pan123.WebUserInfo) {
			if v.SpacePermanent != nil || v.SpaceUsed != nil || v.SpaceTemp != nil || v.SpacePermanentTotal != nil || v.SpaceUsedTotal != nil || string(v.SpaceTempExpr) != "null" {
				t.Fatal("explicit null capacity fields were not preserved")
			}
		}},
		{"zero", `{"SpacePermanent":0,"SpaceUsed":0,"SpaceTemp":0,"SpacePermanentTotal":"0","SpaceUsedTotal":"0"}`, func(t *testing.T, v *pan123.WebUserInfo) {
			if v.SpacePermanent == nil || *v.SpacePermanent != 0 || v.SpaceUsed == nil || *v.SpaceUsed != 0 || v.SpaceTemp == nil || *v.SpaceTemp != 0 || v.SpacePermanentTotal == nil || *v.SpacePermanentTotal != "0" {
				t.Fatal("measured zero was lost")
			}
		}},
		{"integer precision", `{"SpacePermanent":9223372036854775807,"SpaceUsed":9007199254740993,"SpaceTemp":17,"SpacePermanentTotal":"9223372036854775808","SpaceUsedTotal":"9007199254740993","SpaceTempExpr":1793952000}`, func(t *testing.T, v *pan123.WebUserInfo) {
			if v.SpacePermanent == nil || *v.SpacePermanent != 9223372036854775807 || v.SpaceUsed == nil || *v.SpaceUsed != 9007199254740993 || v.SpaceTemp == nil || *v.SpaceTemp != 17 || v.SpacePermanentTotal == nil || *v.SpacePermanentTotal != "9223372036854775808" || string(v.SpaceTempExpr) != "1793952000" {
				t.Fatal("raw integer/string fields lost precision or were combined")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				checkWebRequest(t, r)
				fmt.Fprint(w, `{"code":0,"data":`+tc.data+`}`)
			}))
			defer server.Close()
			client := pan123.NewWithToken("open-fixture-token", pan123.WithWebBaseURL(server.URL))
			info, err := client.User.WebInfo(context.Background(), "web-fixture-token")
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, info)
		})
	}
}

func TestUserWebInfoRejectsBadResponsesWithoutOpenRefresh(t *testing.T) {
	for _, tc := range []struct {
		name, body      string
		status, apiCode int
	}{
		{"expired web identity", `{"code":401,"message":"expired","trace_id":"web-trace-fixture","data":null}`, 200, 401},
		{"HTTP unauthorized", `{"code":401,"message":"expired","trace_id":"web-trace-fixture"}`, 401, 401},
		{"rate limited", `{"code":429,"message":"limited","trace_id":"web-trace-fixture"}`, 200, 429},
		{"HTTP rate limited", `{"code":429,"message":"limited","trace_id":"web-trace-fixture"}`, 429, 429},
		{"malformed JSON", `private-response-is-not-json`, 200, 0},
		{"missing code", `{"data":{}}`, 200, 0},
		{"null data", `{"code":0,"data":null}`, 200, 0},
		{"missing data", `{"code":0}`, 200, 0},
		{"array data", `{"code":0,"data":[]}`, 200, 0},
		{"integer overflow", `{"code":0,"data":{"SpaceUsed":9223372036854775808}}`, 200, 0},
		{"invalid Total type", `{"code":0,"data":{"SpaceUsedTotal":123}}`, 200, 0},
		{"HTTP failure with zero business code", `{"code":0,"data":{}}`, 503, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				checkWebRequest(t, r)
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			client := pan123.New("open-fixture-id", "open-fixture-secret", pan123.WithWebBaseURL(server.URL), pan123.WithMaxRetries(0))
			info, err := client.User.WebInfo(context.Background(), "web-fixture-token")
			if err == nil || info != nil || calls.Load() != 1 || client.Token() != "" {
				t.Fatal("failed web response was accepted or refreshed Open identity")
			}
			var apiErr *pan123.APIError
			if tc.apiCode != 0 && (!errors.As(err, &apiErr) || apiErr.Code != tc.apiCode || apiErr.TraceID != "web-trace-fixture") {
				t.Fatal("business error lost stable SDK type")
			}
			if tc.apiCode == 401 && !pan123.IsTokenExpired(err) || tc.apiCode == 429 && !pan123.IsRateLimited(err) {
				t.Fatal("business error lost stable SDK classification")
			}
			if strings.Contains(err.Error(), "web-fixture-token") || strings.Contains(err.Error(), "private-response-is-not-json") {
				t.Fatal("error leaked token or raw response")
			}
		})
	}
}

func TestUserWebInfoRequiresExplicitTokenAndHonorsContext(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		checkWebRequest(t, r)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := pan123.NewWithToken("open-fixture-token", pan123.WithWebBaseURL(server.URL), pan123.WithHTTPClient(server.Client()))
	for _, token := range []string{"", "   "} {
		if _, err := client.User.WebInfo(context.Background(), token); err == nil {
			t.Fatal("missing web token fell back to Open token")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("empty token made an HTTP request")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := client.User.WebInfo(ctx, "web-fixture-token"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("context not preserved: %v", err)
	}
	if client.Token() != "open-fixture-token" {
		t.Fatal("web cancellation changed Open token")
	}
}

func TestUserWebInfoTransportErrorAndRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close()
	client := pan123.NewWithToken("open-fixture-token", pan123.WithWebBaseURL(server.URL), pan123.WithMaxRetries(0))
	if _, err := client.User.WebInfo(context.Background(), "web-fixture-token"); err == nil || strings.Contains(err.Error(), "web-fixture-token") {
		t.Fatal("transport failure was lost or leaked identity")
	}
	var calls atomic.Int32
	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkWebRequest(t, r)
		if calls.Add(1) == 1 {
			fmt.Fprint(w, `{"code":429,"message":"limited"}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"SpaceUsed":0}}`)
	}))
	defer limited.Close()
	client = pan123.NewWithToken("open-fixture-token", pan123.WithWebBaseURL(limited.URL), pan123.WithMaxRetries(1))
	if _, err := client.User.WebInfo(context.Background(), "web-fixture-token"); err != nil || calls.Load() != 2 {
		t.Fatal("existing retry option was not respected")
	}
}
