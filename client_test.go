package pan123

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func writeJSON(w http.ResponseWriter, code int, message string, data any) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"code":      code,
		"message":   message,
		"data":      data,
		"x-traceID": "test-trace",
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// newTestClient 返回指向 mock 服务器的客户端。
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New("test-id", "test-secret", WithBaseURL(srv.URL), WithoutRateLimit())
	return c, srv
}

func tokenHandler(counter *int32) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if counter != nil {
			atomic.AddInt32(counter, 1)
		}
		writeJSON(w, 0, "ok", map[string]any{
			"accessToken": "test-token",
			"expiredAt":   time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339),
		})
	}
}

func TestTokenAutoFetchAndReuse(t *testing.T) {
	var tokenCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/access_token", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Platform") != "open_platform" {
			t.Errorf("missing Platform header")
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["clientID"] != "test-id" || body["clientSecret"] != "test-secret" {
			t.Errorf("unexpected credentials: %v", body)
		}
		tokenHandler(&tokenCalls)(w, r)
	})
	mux.HandleFunc("/api/v1/user/info", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		writeJSON(w, 0, "ok", map[string]any{"uid": 42, "nickname": "tester", "vip": true})
	})
	c, _ := newTestClient(t, mux)

	for i := 0; i < 3; i++ {
		info, err := c.User.Info(context.Background())
		if err != nil {
			t.Fatalf("Info: %v", err)
		}
		if info.UID != 42 || info.Nickname != "tester" {
			t.Fatalf("unexpected info: %+v", info)
		}
	}
	if n := atomic.LoadInt32(&tokenCalls); n != 1 {
		t.Fatalf("token fetched %d times, want 1", n)
	}
}

func TestTokenRefreshWhenExpired(t *testing.T) {
	var tokenCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/access_token", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&tokenCalls, 1)
		// 首次返回即将过期的 token，触发第二次调用时刷新。
		expire := time.Now().Add(time.Minute)
		if n > 1 {
			expire = time.Now().Add(30 * 24 * time.Hour)
		}
		writeJSON(w, 0, "ok", map[string]any{
			"accessToken": fmt.Sprintf("token-%d", n),
			"expiredAt":   expire.Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/api/v1/user/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 0, "ok", map[string]any{"uid": 1})
	})
	c, _ := newTestClient(t, mux)
	ctx := context.Background()
	if _, err := c.User.Info(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.User.Info(ctx); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&tokenCalls); n != 2 {
		t.Fatalf("token fetched %d times, want 2 (expiring token must be refreshed)", n)
	}
	if got := c.Token(); got != "token-2" {
		t.Fatalf("Token() = %q, want token-2", got)
	}
}

func TestAPIErrorMapping(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/access_token", tokenHandler(nil))
	mux.HandleFunc("/api/v1/file/download_info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 5066, "文件不存在", nil)
	})
	c, _ := newTestClient(t, mux)

	_, err := c.File.DownloadInfo(context.Background(), 999)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != 5066 || apiErr.Message != "文件不存在" || apiErr.TraceID != "test-trace" {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
}

func TestRateLimitRetry(t *testing.T) {
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/access_token", tokenHandler(nil))
	mux.HandleFunc("/api/v1/user/info", func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			writeJSON(w, 429, "请求太频繁", nil)
			return
		}
		writeJSON(w, 0, "ok", map[string]any{"uid": 7})
	})
	c, _ := newTestClient(t, mux)

	info, err := c.User.Info(context.Background())
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if info.UID != 7 {
		t.Fatalf("unexpected uid %d", info.UID)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestListAllPagination(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/access_token", tokenHandler(nil))
	mux.HandleFunc("/api/v2/file/list", func(w http.ResponseWriter, r *http.Request) {
		last := r.URL.Query().Get("lastFileId")
		switch last {
		case "":
			writeJSON(w, 0, "ok", map[string]any{
				"lastFileId": 2,
				"fileList": []map[string]any{
					{"fileId": 1, "filename": "a.txt", "trashed": 0},
					{"fileId": 2, "filename": "in-trash.txt", "trashed": 1},
				},
			})
		case "2":
			writeJSON(w, 0, "ok", map[string]any{
				"lastFileId": -1,
				"fileList": []map[string]any{
					{"fileId": 3, "filename": "b.txt", "trashed": 0},
				},
			})
		default:
			t.Errorf("unexpected lastFileId %q", last)
		}
	})
	c, _ := newTestClient(t, mux)

	files, err := c.File.ListAll(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files, want 2 (trashed filtered): %+v", len(files), files)
	}
	if files[0].Filename != "a.txt" || files[1].Filename != "b.txt" {
		t.Fatalf("unexpected files: %+v", files)
	}
}

func TestUploadFileSliced(t *testing.T) {
	content := strings.Repeat("x", 2500) // sliceSize 1000 → 3 个分片
	sum := md5.Sum([]byte(content))
	wantEtag := hex.EncodeToString(sum[:])

	var completeCalls, slicesUploaded int32
	mux := http.NewServeMux()
	var srvURL string
	mux.HandleFunc("/api/v1/access_token", tokenHandler(nil))
	mux.HandleFunc("/upload/v2/file/create", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["etag"] != wantEtag {
			t.Errorf("etag = %v, want %s", body["etag"], wantEtag)
		}
		if body["size"] != float64(len(content)) {
			t.Errorf("size = %v", body["size"])
		}
		writeJSON(w, 0, "ok", map[string]any{
			"reuse":       false,
			"preuploadID": "pre-1",
			"sliceSize":   1000,
			"servers":     []string{srvURL},
		})
	})
	mux.HandleFunc("/upload/v2/file/slice", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			writeJSON(w, 1, "bad form", nil)
			return
		}
		if r.FormValue("preuploadID") != "pre-1" {
			t.Errorf("preuploadID = %q", r.FormValue("preuploadID"))
		}
		f, _, err := r.FormFile("slice")
		if err != nil {
			t.Errorf("form file: %v", err)
			writeJSON(w, 1, "no file", nil)
			return
		}
		data, _ := io.ReadAll(f)
		sum := md5.Sum(data)
		if hex.EncodeToString(sum[:]) != r.FormValue("sliceMD5") {
			t.Errorf("slice md5 mismatch for sliceNo %s", r.FormValue("sliceNo"))
		}
		atomic.AddInt32(&slicesUploaded, 1)
		writeJSON(w, 0, "ok", nil)
	})
	mux.HandleFunc("/upload/v2/file/upload_complete", func(w http.ResponseWriter, r *http.Request) {
		// 第一次未完成，验证轮询逻辑。
		if atomic.AddInt32(&completeCalls, 1) == 1 {
			writeJSON(w, 0, "ok", map[string]any{"completed": false, "fileID": 0})
			return
		}
		writeJSON(w, 0, "ok", map[string]any{"completed": true, "fileID": 12345})
	})
	c, srv := newTestClient(t, mux)
	srvURL = srv.URL

	fileID, err := c.Upload.UploadFile(context.Background(), 0, "test.txt", strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if fileID != 12345 {
		t.Fatalf("fileID = %d, want 12345", fileID)
	}
	if n := atomic.LoadInt32(&slicesUploaded); n != 3 {
		t.Fatalf("uploaded %d slices, want 3", n)
	}
	if n := atomic.LoadInt32(&completeCalls); n != 2 {
		t.Fatalf("complete called %d times, want 2 (poll once)", n)
	}
}

func TestUploadFileInstant(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/access_token", tokenHandler(nil))
	mux.HandleFunc("/upload/v2/file/create", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 0, "ok", map[string]any{"reuse": true, "fileID": 777})
	})
	c, _ := newTestClient(t, mux)

	fileID, err := c.Upload.UploadFile(context.Background(), 0, "dup.txt", strings.NewReader("hello"), 5)
	if err != nil {
		t.Fatal(err)
	}
	if fileID != 777 {
		t.Fatalf("fileID = %d, want 777 (秒传)", fileID)
	}
}

func TestUploadDomainsArrayData(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/access_token", tokenHandler(nil))
	mux.HandleFunc("/upload/v2/file/domain", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 0, "ok", []string{"https://openapi-upload.123pan.com"})
	})
	c, _ := newTestClient(t, mux)

	domains, err := c.Upload.Domains(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(domains) != 1 || domains[0] != "https://openapi-upload.123pan.com" {
		t.Fatalf("unexpected domains: %v", domains)
	}
}

func TestOAuthFlatResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/oauth2/access_token", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("grant_type") != "authorization_code" || q.Get("code") != "the-code" {
			t.Errorf("unexpected query: %v", q)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token_type":    "Bearer",
			"access_token":  "oauth-token",
			"refresh_token": "refresh-1",
			"expires_in":    7200,
			"scope":         OAuthScope,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New("id", "secret", WithBaseURL(srv.URL), WithoutRateLimit())

	token, err := c.OAuth.TokenByCode(context.Background(), "app-id", "app-secret", "the-code", "https://cb.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "oauth-token" || token.RefreshToken != "refresh-1" || token.ExpiresIn != 7200 {
		t.Fatalf("unexpected token: %+v", token)
	}
}

func TestNewWithTokenSkipsCredentialFetch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/access_token", func(w http.ResponseWriter, r *http.Request) {
		t.Error("access_token endpoint must not be called in NewWithToken mode")
	})
	mux.HandleFunc("/api/v1/user/info", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer static-token" {
			t.Errorf("Authorization = %q", got)
		}
		writeJSON(w, 0, "ok", map[string]any{"uid": 9})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := NewWithToken("static-token", WithBaseURL(srv.URL), WithoutRateLimit())

	info, err := c.User.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.UID != 9 {
		t.Fatalf("uid = %d", info.UID)
	}
}
