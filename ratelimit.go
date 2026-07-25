package pan123

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// apiQPS 是官方文档《开发须知》公布的各接口 QPS 限制。
// 未列出的接口不做客户端限流。
var apiQPS = map[string]int{
	"/api/v1/user/info":                        10,
	"/api/v1/file/move":                        20,
	"/api/v1/file/delete":                      10,
	"/api/v1/file/list":                        10,
	"/api/v2/file/list":                        15,
	"/upload/v1/file/mkdir":                    20,
	"/api/v1/access_token":                     10,
	"/api/v1/transcode/folder/info":            20,
	"/api/v1/transcode/upload/from_cloud_disk": 1,
	"/api/v1/transcode/delete":                 10,
	"/api/v1/transcode/video/resolutions":      1,
	"/api/v1/transcode/video":                  3,
	"/api/v1/transcode/video/record":           20,
	"/api/v1/transcode/video/result":           20,
	"/api/v1/transcode/file/download":          10,
	"/api/v1/transcode/m3u8_ts/download":       20,
	"/api/v1/transcode/file/download/all":      1,
}

// rateLimiter 按接口路径做客户端限流，避免触发服务端 429。
type rateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{limiters: make(map[string]*rate.Limiter)}
}

func (r *rateLimiter) wait(ctx context.Context, path string) error {
	qps, ok := apiQPS[path]
	if !ok {
		return nil
	}
	r.mu.Lock()
	l, ok := r.limiters[path]
	if !ok {
		l = rate.NewLimiter(rate.Limit(qps), qps)
		r.limiters[path] = l
	}
	r.mu.Unlock()
	return l.Wait(ctx)
}
