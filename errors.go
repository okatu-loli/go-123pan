package pan123

import (
	"errors"
	"fmt"
)

// APIError 表示开放平台返回的业务错误（响应 code != 0）。
type APIError struct {
	// Code 是响应体中的业务错误码。
	Code int
	// Message 是平台返回的错误描述。
	Message string
	// TraceID 用于联系官方技术支持时定位问题。
	TraceID string
}

func (e *APIError) Error() string {
	if e.TraceID != "" {
		return fmt.Sprintf("pan123: api error code=%d message=%q traceID=%s", e.Code, e.Message, e.TraceID)
	}
	return fmt.Sprintf("pan123: api error code=%d message=%q", e.Code, e.Message)
}

// IsTokenExpired 判断错误是否为 access_token 无效（code=401）。
func IsTokenExpired(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == 401
}

// IsRateLimited 判断错误是否为请求过于频繁（code=429）。
func IsRateLimited(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == 429
}
