// Package pan123 是 123 云盘开放平台（https://www.123pan.com）的非官方 Go SDK。
//
// 快速开始：
//
//	client := pan123.New("your-client-id", "your-client-secret")
//	info, err := client.User.Info(ctx)
//
// SDK 自动管理 access_token 的获取、缓存与过期刷新，并内置官方 QPS 限流与 429 退避重试。
package pan123

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL 是开放平台的接口域名。
const DefaultBaseURL = "https://open-api.123pan.com"

const (
	platformHeader = "open_platform"
	defaultUA      = "go-123pan"
	// tokenSafetyMargin 在 token 过期前提前刷新的时间窗口。
	tokenSafetyMargin = 10 * time.Minute
)

// Client 是 123 云盘开放平台的 API 客户端。
// 通过 New 或 NewWithToken 创建；零值不可用。
type Client struct {
	clientID     string
	clientSecret string

	baseURL    string
	httpClient *http.Client
	userAgent  string
	maxRetries int
	limiter    *rateLimiter

	mu             sync.Mutex
	token          string
	tokenExpiredAt time.Time
	staticToken    bool

	// 服务分组
	File      *FileService
	Upload    *UploadService
	Share     *ShareService
	Offline   *OfflineService
	User      *UserService
	Link      *LinkService
	Oss       *OssService
	Transcode *TranscodeService
	OAuth     *OAuthService
}

// Option 配置 Client。
type Option func(*Client)

// WithHTTPClient 使用自定义 *http.Client（超时、代理等）。
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBaseURL 覆盖接口域名（用于测试或代理网关）。
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithUserAgent 设置请求 User-Agent。
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithoutRateLimit 关闭内置的客户端 QPS 限流。
func WithoutRateLimit() Option {
	return func(c *Client) { c.limiter = nil }
}

// WithMaxRetries 设置遇到限流（code=429）时的最大重试次数，0 表示不重试。
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.maxRetries = n }
}

// New 创建客户端。SDK 会在首次调用 API 时自动获取 access_token，
// 并在过期前自动刷新（并发安全，同一时刻只会发起一次获取请求）。
func New(clientID, clientSecret string, opts ...Option) *Client {
	c := newClient(opts...)
	c.clientID = clientID
	c.clientSecret = clientSecret
	return c
}

// NewWithToken 使用已有 access_token 创建客户端（例如三方挂载应用 OAuth 授权得到的 token）。
// 此模式下 SDK 不会自动刷新 token；过期后请调用 SetToken 更新。
func NewWithToken(accessToken string, opts ...Option) *Client {
	c := newClient(opts...)
	c.token = accessToken
	c.staticToken = true
	return c
}

func newClient(opts ...Option) *Client {
	c := &Client{
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		userAgent:  defaultUA,
		maxRetries: 3,
		limiter:    newRateLimiter(),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.File = &FileService{c}
	c.Upload = &UploadService{c}
	c.Share = &ShareService{c}
	c.Offline = &OfflineService{c}
	c.User = &UserService{c}
	c.Link = &LinkService{c}
	c.Oss = &OssService{c}
	c.Transcode = &TranscodeService{c}
	c.OAuth = &OAuthService{c}
	return c
}

// SetToken 手动设置 access_token 及其过期时间。
// 传入零值 expiredAt 表示永不主动刷新。
func (c *Client) SetToken(token string, expiredAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
	c.tokenExpiredAt = expiredAt
}

// Token 返回当前持有的 access_token（可能为空或已过期）。
func (c *Client) Token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.token
}

// AccessTokenResult 是获取 access_token 接口的返回。
type AccessTokenResult struct {
	AccessToken string    `json:"accessToken"`
	ExpiredAt   time.Time `json:"expiredAt"`
}

// RefreshToken 强制重新获取 access_token（clientID/clientSecret 模式）。
// 一般无需手动调用，SDK 会在过期前自动刷新。
func (c *Client) RefreshToken(ctx context.Context) (*AccessTokenResult, error) {
	if c.clientID == "" {
		return nil, fmt.Errorf("pan123: client created with NewWithToken cannot refresh token")
	}
	var out AccessTokenResult
	err := c.doNoAuth(ctx, http.MethodPost, "/api/v1/access_token", map[string]string{
		"clientID":     c.clientID,
		"clientSecret": c.clientSecret,
	}, &out)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.token = out.AccessToken
	c.tokenExpiredAt = out.ExpiredAt
	c.mu.Unlock()
	return &out, nil
}

// accessToken 返回有效 token，必要时自动获取/刷新。
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.staticToken || (c.token != "" && (c.tokenExpiredAt.IsZero() || time.Until(c.tokenExpiredAt) > tokenSafetyMargin)) {
		t := c.token
		c.mu.Unlock()
		return t, nil
	}
	c.mu.Unlock()
	if c.clientID == "" {
		return "", fmt.Errorf("pan123: no access token and no client credentials")
	}
	if _, err := c.RefreshToken(ctx); err != nil {
		return "", err
	}
	return c.Token(), nil
}

// apiResponse 是开放平台统一响应结构。
type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	TraceID string          `json:"x-traceID"`
}

// get 发起 GET 请求，query 为查询参数。
func (c *Client) get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, out, true)
}

// post 发起 JSON body 的 POST 请求。
func (c *Client) post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, out, true)
}

// put 发起 JSON body 的 PUT 请求。
func (c *Client) put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, path, nil, body, out, true)
}

// doNoAuth 用于无需 Authorization 的接口（获取 token、OAuth）。
func (c *Client) doNoAuth(ctx context.Context, method, path string, body, out any) error {
	return c.do(ctx, method, path, nil, body, out, false)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any, auth bool) error {
	var attempt int
	for {
		err := c.doOnce(ctx, method, path, query, body, out, auth)
		if err == nil {
			return nil
		}
		if !IsRateLimited(err) || attempt >= c.maxRetries {
			return err
		}
		attempt++
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff(attempt)):
		}
	}
}

func backoff(attempt int) time.Duration {
	d := 500 * time.Millisecond << (attempt - 1)
	if d > 8*time.Second {
		d = 8 * time.Second
	}
	return d
}

func (c *Client) doOnce(ctx context.Context, method, path string, query url.Values, body, out any, auth bool) error {
	if c.limiter != nil {
		if err := c.limiter.wait(ctx, path); err != nil {
			return err
		}
	}

	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("pan123: marshal request: %w", err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Platform", platformHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if auth {
		token, err := c.accessToken(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("pan123: read response: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(raw, &apiResp); err != nil {
		return fmt.Errorf("pan123: unexpected response (http %d): %s", resp.StatusCode, truncate(raw, 512))
	}
	if apiResp.Code != 0 {
		return &APIError{Code: apiResp.Code, Message: apiResp.Message, TraceID: apiResp.TraceID}
	}
	if out != nil && len(apiResp.Data) > 0 && string(apiResp.Data) != "null" {
		if err := json.Unmarshal(apiResp.Data, out); err != nil {
			return fmt.Errorf("pan123: decode data: %w", err)
		}
	}
	return nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
