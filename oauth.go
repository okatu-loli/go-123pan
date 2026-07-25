package pan123

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// OAuthService 提供第三方挂载应用的 OAuth 授权接口。
// 接入需先通过官方资质审核获得 appId 与 secretId（暂不支持个人开发者）。
type OAuthService struct {
	client *Client
}

// AuthBaseURL 是授权页面域名。
const AuthBaseURL = "https://yun.123pan.com"

// OAuthScope 是授权固定 scope。
const OAuthScope = "user:base,file:all:read,file:all:write"

// AuthURL 构造用户授权页面地址（浏览器跳转用，本方法不发起 HTTP 请求）。
//
// 接口: GET https://yun.123pan.com/auth（授权页面，域名固定为 yun.123pan.com）
//
// 参数:
//   - clientID: 应用的 appId。
//   - redirectURI: 授权后的回跳地址，须与应用注册的回调地址一致。
//   - state: 自定义透传参数，回跳时原样带回，可用于防 CSRF。
//
// 注意: scope 固定为 OAuthScope；用户授权后将携带 code（与 state）回跳
// redirectURI，随后用 TokenByCode 换取 access_token。
func (s *OAuthService) AuthURL(clientID, redirectURI, state string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", OAuthScope)
	q.Set("state", state)
	return AuthBaseURL + "/auth?" + q.Encode()
}

// OAuthToken 是 OAuth 接口返回的令牌。
type OAuthToken struct {
	TokenType   string `json:"token_type"`
	AccessToken string `json:"access_token"`
	// RefreshToken 单次有效，90 天有效期；每次刷新都会返回新的 refresh_token，必须持久化替换旧值。
	RefreshToken string `json:"refresh_token"`
	// ExpiresIn access_token 过期时间（秒）。
	ExpiresIn int64  `json:"expires_in"`
	Scope     string `json:"scope"`
}

// TokenByCode 用授权 code 换取 access_token。
//
// 接口: POST /api/v1/oauth2/access_token (QPS 限制 100 次/分钟)
//
// 参数:
//   - clientID: 应用的 appId。
//   - clientSecret: 应用的 secretId。
//   - code: 授权回跳携带的授权码，一次性使用。
//   - redirectURI: 必须与应用注册的回调地址一致。
//
// 注意: 该接口返回扁平 JSON，不包裹统一响应结构。access_token 有效期见
// ExpiresIn（秒）；refresh_token 单次有效、90 天有效期，务必持久化保存。
func (s *OAuthService) TokenByCode(ctx context.Context, clientID, clientSecret, code, redirectURI string) (*OAuthToken, error) {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("client_secret", clientSecret)
	q.Set("grant_type", "authorization_code")
	q.Set("code", code)
	q.Set("redirect_uri", redirectURI)
	return s.token(ctx, q)
}

// RefreshToken 用 refresh_token 刷新令牌。
//
// 接口: POST /api/v1/oauth2/access_token (QPS 限制 100 次/分钟)
//
// 参数:
//   - clientID: 应用的 appId。
//   - clientSecret: 应用的 secretId。
//   - refreshToken: 上次颁发的 refresh_token（单次有效，有效期 90 天）。
//
// 注意: 刷新成功后旧 access_token 立即失效，refresh_token 同时换新（旧值
// 作废），必须持久化新返回的 RefreshToken，否则后续将无法再次刷新。
// 该接口返回扁平 JSON，不包裹统一响应结构。
func (s *OAuthService) RefreshToken(ctx context.Context, clientID, clientSecret, refreshToken string) (*OAuthToken, error) {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("client_secret", clientSecret)
	q.Set("grant_type", "refresh_token")
	q.Set("refresh_token", refreshToken)
	return s.token(ctx, q)
}

// token 调用 OAuth 换取令牌接口。注意：该接口返回扁平 JSON，不包裹统一响应结构。
func (s *OAuthService) token(ctx context.Context, q url.Values) (*OAuthToken, error) {
	u := s.client.baseURL + "/api/v1/oauth2/access_token?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Platform", platformHeader)
	req.Header.Set("User-Agent", s.client.userAgent)
	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var token OAuthToken
	if err := json.Unmarshal(raw, &token); err != nil || token.AccessToken == "" {
		// 失败时可能返回统一错误结构，尝试解析给出可读错误。
		var apiResp apiResponse
		if jerr := json.Unmarshal(raw, &apiResp); jerr == nil && apiResp.Code != 0 {
			return nil, &APIError{Code: apiResp.Code, Message: apiResp.Message, TraceID: apiResp.TraceID}
		}
		return nil, fmt.Errorf("pan123: oauth token failed (http %d): %s", resp.StatusCode, truncate(raw, 512))
	}
	return &token, nil
}
