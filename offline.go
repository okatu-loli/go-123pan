package pan123

import (
	"context"
	"net/url"
	"strconv"
)

// OfflineService 提供离线下载相关接口。
type OfflineService struct {
	client *Client
}

// OfflineDownloadRequest 是创建离线下载任务的参数。
type OfflineDownloadRequest struct {
	// URL 下载资源地址，仅支持 http/https。
	URL string `json:"url"`
	// FileName 自定义文件名称（可选）。
	FileName string `json:"fileName,omitempty"`
	// DirID 下载到的目标目录 ID（可选）。不支持根目录，
	// 默认下载到名为"来自:离线下载"的目录。
	DirID int64 `json:"dirID,omitempty"`
	// CallBackURL 回调地址（可选）。下载成功或失败时以 POST 通知：
	// {"url":"...","status":0,"failReason":"","fileID":100}，status 0 成功 1 失败。
	CallBackURL string `json:"callBackUrl,omitempty"`
}

// Download 创建离线下载任务，返回任务 ID。
func (s *OfflineService) Download(ctx context.Context, req *OfflineDownloadRequest) (taskID int64, err error) {
	var out struct {
		TaskID int64 `json:"taskID"`
	}
	if err := s.client.post(ctx, "/api/v1/offline/download", req, &out); err != nil {
		return 0, err
	}
	return out.TaskID, nil
}

// OfflineStatus 离线下载状态：0 进行中、1 下载失败、2 下载成功、3 重试中。
type OfflineStatus int

const (
	OfflineRunning  OfflineStatus = 0
	OfflineFailed   OfflineStatus = 1
	OfflineSuccess  OfflineStatus = 2
	OfflineRetrying OfflineStatus = 3
)

// OfflineProcessResult 是离线下载进度。
type OfflineProcessResult struct {
	// Process 下载进度百分比；下载失败时会归零。
	Process float64       `json:"process"`
	Status  OfflineStatus `json:"status"`
}

// Process 查询离线下载任务进度。
// 注意 Status 为 3（重试中）时仍未终结，不能仅凭 Process 判断结果。
func (s *OfflineService) Process(ctx context.Context, taskID int64) (*OfflineProcessResult, error) {
	q := url.Values{}
	q.Set("taskID", strconv.FormatInt(taskID, 10))
	var out OfflineProcessResult
	if err := s.client.get(ctx, "/api/v1/offline/download/process", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
