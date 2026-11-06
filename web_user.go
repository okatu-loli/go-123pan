package pan123

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultWebBaseURL is the website API host observed on yun.123pan.cn. This
// private website interface is separate from the documented Open Platform API.
const DefaultWebBaseURL = "https://api.123278.com"

const webUserInfoPath = "/b/api/user/info"

// WithWebBaseURL overrides the website API base URL for a proxy or gateway.
// It does not change the Open Platform base URL configured with WithBaseURL.
func WithWebBaseURL(baseURL string) Option {
	return func(c *Client) { c.webBaseURL = strings.TrimRight(baseURL, "/") }
}

// WebUserInfo preserves capacity fields returned by the logged-in website.
// Nil numeric fields mean unknown, while a non-nil zero is a reported zero.
// Total strings are retained verbatim, including values beyond int64. No total
// is synthesized by adding permanent and temporary capacity.
type WebUserInfo struct {
	SpacePermanent      *int64          `json:"SpacePermanent"`
	SpacePermanentTotal *string         `json:"SpacePermanentTotal"`
	SpaceTemp           *int64          `json:"SpaceTemp"`
	SpaceUsed           *int64          `json:"SpaceUsed"`
	SpaceUsedTotal      *string         `json:"SpaceUsedTotal"`
	SpaceTempExpr       json.RawMessage `json:"SpaceTempExpr"`
}

// WebInfo reads GET /b/api/user/info with an explicitly supplied website login
// token (without the Bearer prefix). An Open Platform token is not a substitute.
// It never reads, updates or refreshes the client's Open credentials, and never
// logs in or requests a capacity recalculation. HTTP client, user agent and
// rate-limit retry options are shared with other SDK calls. The website's
// private protocol may change independently of the Open Platform API.
func (s *UserService) WebInfo(ctx context.Context, webToken string) (*WebUserInfo, error) {
	if strings.TrimSpace(webToken) == "" {
		return nil, fmt.Errorf("pan123: website login token is required")
	}
	var result *WebUserInfo
	err := s.client.retry(ctx, func() error {
		var err error
		result, err = s.client.webUserInfo(ctx, webToken)
		return err
	})
	return result, err
}

func (c *Client) webUserInfo(ctx context.Context, webToken string) (*WebUserInfo, error) {
	base, err := url.Parse(c.webBaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, fmt.Errorf("pan123: invalid website API base URL")
	}
	// A proxy may retain a path prefix while forwarding the original API path.
	base.Path = strings.TrimRight(base.Path, "/") + webUserInfoPath
	nonce, err := rand.Int(rand.Reader, big.NewInt(10_000_001))
	if err != nil {
		return nil, fmt.Errorf("pan123: generate website request nonce: %w", err)
	}
	key, value := webSignature(time.Now().Round(time.Second).Unix(), uint32(nonce.Uint64()), webUserInfoPath)
	base.RawQuery = url.Values{key: {value}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("pan123: create website user info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+webToken)
	req.Header.Set("Platform", "web")
	req.Header.Set("App-Version", "3")
	req.Header.Set("Origin", "https://yun.123pan.cn")
	req.Header.Set("Referer", "https://yun.123pan.cn/")
	req.Header.Set("User-Agent", c.userAgent)
	response, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("pan123: read website user info response: %w", err)
	}
	var envelope struct {
		Code    *int            `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
		TraceID string          `json:"trace_id"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Code == nil {
		// A website response may contain account data. Do not embed it in errors.
		return nil, fmt.Errorf("pan123: invalid website user info response (http %d)", response.StatusCode)
	}
	if *envelope.Code != 0 {
		return nil, &APIError{Code: *envelope.Code, Message: envelope.Message, TraceID: envelope.TraceID}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("pan123: website user info returned http %d", response.StatusCode)
	}
	var result *WebUserInfo
	if len(envelope.Data) == 0 || json.Unmarshal(envelope.Data, &result) != nil || result == nil {
		return nil, fmt.Errorf("pan123: invalid website user info data")
	}
	return result, nil
}

// Independent implementation of the website's published request protocol:
// https://statics.123pan.com/123pan-resources/prod/dist/app.5bf67275c7141b4e31a5.1793948552.js
// Module 33755 export L, with web/3 headers from module 84532 (2026-11-06).
// CRC32 is IEEE, unsigned and decimal. The date key uses the signed timestamp's
// Beijing minute rather than the host timezone; this also handles minute edges.
func webSignature(seconds int64, nonce uint32, path string) (string, string) {
	minute := time.Unix(seconds, 0).In(time.FixedZone("UTC+8", 8*60*60)).Format("200601021504")
	digits := []byte(minute)
	for i := range digits {
		digits[i] = "adefghlmyi"[digits[i]-'0']
	}
	key := strconv.FormatUint(uint64(crc32.ChecksumIEEE(digits)), 10)
	stamp := strconv.FormatInt(seconds, 10)
	random := strconv.FormatUint(uint64(nonce), 10)
	payload := strings.Join([]string{stamp, random, path, "web", "3", key}, "|")
	checksum := strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(payload))), 10)
	return key, stamp + "-" + random + "-" + checksum
}
