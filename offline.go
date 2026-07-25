package pan123

import (
	"context"
	"net/url"
	"strconv"
)

// OfflineService 提供离线下载相关接口：创建离线下载任务并查询任务进度。
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

// Download 创建离线下载任务，返回任务 ID（可用于 Process 查询进度）。
//
// 接口: POST /api/v1/offline/download
//
// 参数说明（见 OfflineDownloadRequest 各字段）：
//   - URL：下载资源地址，必填，仅支持 http/https 协议。
//   - FileName：自定义文件名称，可选。
//   - DirID：下载到的目标目录 ID，可选；不支持根目录，
//     不填时默认下载到名为"来自:离线下载"的目录。
//   - CallBackURL：回调地址，可选；任务结束（成功或失败）时以 POST 通知，
//     回调体形如 {"url":"...","status":0,"failReason":"","fileID":100}，
//     其中 status 0 成功、1 失败。
//
// 注意事项：接口返回成功仅表示任务创建成功，下载结果需通过 Process
// 轮询或回调地址获知。
func (s *OfflineService) Download(ctx context.Context, req *OfflineDownloadRequest) (taskID int64, err error) {
	var out struct {
		TaskID int64 `json:"taskID"`
	}
	if err := s.client.post(ctx, "/api/v1/offline/download", req, &out); err != nil {
		return 0, err
	}
	return out.TaskID, nil
}

// OfflineStatus 离线下载状态。
// 枚举值：0 进行中；1 下载失败；2 下载成功；3 重试中。
// 其中 3（重试中）不是终态，任务仍可能转为成功或失败。
type OfflineStatus int

const (
	// OfflineRunning 进行中。
	OfflineRunning OfflineStatus = 0
	// OfflineFailed 下载失败（终态）。
	OfflineFailed OfflineStatus = 1
	// OfflineSuccess 下载成功（终态）。
	OfflineSuccess OfflineStatus = 2
	// OfflineRetrying 重试中，仍未终结。
	OfflineRetrying OfflineStatus = 3
)

// OfflineProcessResult 是离线下载进度。
type OfflineProcessResult struct {
	// Process 下载进度百分比（0-100）；下载失败时会归零。
	Process float64 `json:"process"`
	// Status 任务状态：0 进行中；1 下载失败；2 下载成功；3 重试中。
	Status OfflineStatus `json:"status"`
}

// Process 查询离线下载任务的进度与状态。
//
// 接口: GET /api/v1/offline/download/process
//
// 参数说明：
//   - taskID：离线下载任务 ID，由 Download 返回。
//
// 注意事项：应以 Status 判断任务结果，不能仅凭 Process 判断——
// 失败时 Process 会归零；Status 为 3（重试中）时任务仍未终结，
// 需要继续轮询直至变为 1（失败）或 2（成功）。
func (s *OfflineService) Process(ctx context.Context, taskID int64) (*OfflineProcessResult, error) {
	q := url.Values{}
	q.Set("taskID", strconv.FormatInt(taskID, 10))
	var out OfflineProcessResult
	if err := s.client.get(ctx, "/api/v1/offline/download/process", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
