package pan123

import (
	"context"
	"net/url"
	"strconv"
)

// TranscodeService 提供视频转码相关接口。
//
// 本地上传视频到转码空间：先调用 FolderInfo 获取转码空间目录 ID，
// 再使用 Upload.UploadFile 将视频上传到该目录。
type TranscodeService struct {
	client *Client
}

// FolderInfo 获取转码空间文件夹 ID（本地上传视频时作为 parentFileID 使用）。
func (s *TranscodeService) FolderInfo(ctx context.Context) (int64, error) {
	var out struct {
		FileID int64 `json:"fileID"`
	}
	if err := s.client.post(ctx, "/api/v1/transcode/folder/info", struct{}{}, &out); err != nil {
		return 0, err
	}
	return out.FileID, nil
}

// CloudVideoFiles 获取云盘空间中的视频文件（category=2），用于后续 UploadFromCloudDisk。
func (s *TranscodeService) CloudVideoFiles(ctx context.Context, req *FileListRequest) (*FileListResult, error) {
	q := req.values()
	q.Set("category", "2")
	var out FileListResult
	if err := s.client.get(ctx, "/api/v2/file/list", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SpaceFiles 获取转码空间的文件列表（businessType=2）。
func (s *TranscodeService) SpaceFiles(ctx context.Context, req *FileListRequest) (*FileListResult, error) {
	q := req.values()
	q.Set("businessType", "2")
	var out FileListResult
	if err := s.client.get(ctx, "/api/v2/file/list", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UploadFromCloudDisk 将云盘空间的视频文件上传到转码空间，一次最多 100 个文件。
// 注意：官方 QPS 限制为 1。
func (s *TranscodeService) UploadFromCloudDisk(ctx context.Context, fileIDs []int64) error {
	body := make([]map[string]int64, 0, len(fileIDs))
	for _, id := range fileIDs {
		body = append(body, map[string]int64{"fileId": id})
	}
	return s.client.post(ctx, "/api/v1/transcode/upload/from_cloud_disk", body, nil)
}

// ResolutionsResult 是获取可转码分辨率接口的返回。
type ResolutionsResult struct {
	// IsGetResolution 为 true 表示服务端仍在解析，需继续轮询（官方建议间隔 10s）。
	IsGetResolution bool `json:"IsGetResolution"`
	// Resolutions 可转码的分辨率，逗号分隔，如 "480p,720p,1080p"。
	Resolutions string `json:"Resolutions"`
	// NowOrFinishedResolutions 正在或已完成转码的分辨率；转码时应排除，避免重复转码。
	NowOrFinishedResolutions string `json:"NowOrFinishedResolutions"`
	// CodecNames 编码方式，如 "H.264"。
	CodecNames string `json:"CodecNames"`
	// VideoTime 视频时长，单位秒。
	VideoTime int64 `json:"VideoTime"`
}

// Resolutions 获取视频文件可转码的分辨率。
// IsGetResolution 为 true 时结果尚未就绪，需轮询（建议 10s 间隔）。
func (s *TranscodeService) Resolutions(ctx context.Context, fileID int64) (*ResolutionsResult, error) {
	var out ResolutionsResult
	err := s.client.post(ctx, "/api/v1/transcode/video/resolutions", map[string]int64{"fileId": fileID}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// TranscodeRequest 是发起视频转码的参数。
type TranscodeRequest struct {
	FileID int64 `json:"fileId"`
	// CodecName 编码方式，取自 Resolutions 返回的 CodecNames。
	CodecName string `json:"codecName"`
	// VideoTime 视频时长（秒），取自 Resolutions 返回的 VideoTime。
	VideoTime int64 `json:"videoTime"`
	// Resolutions 要转码的分辨率，逗号分隔且 P 大写，如 "2160P,1080P,720P"。
	// 已转码过的分辨率无需再传。
	Resolutions string `json:"resolutions"`
}

// Transcode 发起视频转码操作。
func (s *TranscodeService) Transcode(ctx context.Context, req *TranscodeRequest) error {
	return s.client.post(ctx, "/api/v1/transcode/video", req, nil)
}

// TranscodeRecord 是单条转码记录。
type TranscodeRecord struct {
	CreateAt   string `json:"create_at"`
	Resolution string `json:"resolution"`
	// Status 1:准备转码 2:正在转码中 3-254:转码失败 255:转码成功
	Status int `json:"status"`
	// Link 转码成功后的 m3u8 链接（仅 Status=255 时有值）。
	Link string `json:"link"`
}

// Records 查询某个视频的转码记录。
func (s *TranscodeService) Records(ctx context.Context, fileID int64) ([]TranscodeRecord, error) {
	var out struct {
		List []TranscodeRecord `json:"UserTranscodeVideoRecordList"`
	}
	err := s.client.post(ctx, "/api/v1/transcode/video/record", map[string]int64{"fileId": fileID}, &out)
	if err != nil {
		return nil, err
	}
	return out.List, nil
}

// TranscodeFile 是转码产物文件。
type TranscodeFile struct {
	FileName string `json:"FileName"`
	// FileSize 为带单位的字符串，如 "497.17KB"。
	FileSize   string `json:"FileSize"`
	Resolution string `json:"Resolution"`
	CreateAt   string `json:"CreateAt"`
	// URL 播放地址，仅 m3u8 文件有值。
	URL string `json:"Url"`
}

// TranscodeResult 是某分辨率的转码结果。
type TranscodeResult struct {
	UID        int64  `json:"Uid"`
	Resolution string `json:"Resolution"`
	// Status 1:准备转码 2:正在转码中 3-254:转码失败 255:转码成功
	Status int             `json:"Status"`
	Files  []TranscodeFile `json:"Files"`
}

// Results 查询某个视频的转码结果（含产物文件列表）。
func (s *TranscodeService) Results(ctx context.Context, fileID int64) ([]TranscodeResult, error) {
	var out struct {
		List []TranscodeResult `json:"UserTranscodeVideoList"`
	}
	err := s.client.post(ctx, "/api/v1/transcode/video/result", map[string]int64{"fileId": fileID}, &out)
	if err != nil {
		return nil, err
	}
	return out.List, nil
}

// VideoTranscodeItem 是三方挂载应用授权场景的转码列表项。
type VideoTranscodeItem struct {
	URL        string  `json:"url"`
	Resolution string  `json:"resolution"`
	Duration   float64 `json:"duration"`
	Height     int     `json:"height"`
	Status     int     `json:"status"`
	MC         string  `json:"mc"`
	BitRate    int64   `json:"bitRate"`
	Progress   int     `json:"progress"`
	UpdateAt   string  `json:"updateAt"`
}

// VideoTranscodeList 是三方挂载应用授权场景的转码列表返回。
type VideoTranscodeList struct {
	// Status 转码状态：1 待转码；3 转码失败；254 部分成功；255 全部成功。
	Status int                  `json:"status"`
	List   []VideoTranscodeItem `json:"list"`
}

// List 视频转码列表（仅限三方挂载应用授权 access_token 调用）。
func (s *TranscodeService) List(ctx context.Context, fileID int64) (*VideoTranscodeList, error) {
	q := url.Values{}
	q.Set("fileId", strconv.FormatInt(fileID, 10))
	var out VideoTranscodeList
	if err := s.client.get(ctx, "/api/v1/video/transcode/list", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteMode 控制删除转码视频时的删除范围。
type DeleteMode int

const (
	// DeleteOriginal 仅删除原文件。
	DeleteOriginal DeleteMode = 1
	// DeleteOriginalAndTranscoded 删除原文件及转码后的文件（不可逆）。
	DeleteOriginalAndTranscoded DeleteMode = 2
)

// Delete 删除转码视频。
func (s *TranscodeService) Delete(ctx context.Context, fileID int64, mode DeleteMode) error {
	return s.client.post(ctx, "/api/v1/transcode/delete", map[string]any{
		"fileId":       fileID,
		"businessType": 2,
		"trashed":      int(mode),
	}, nil)
}

// TranscodeDownloadResult 是转码空间下载类接口的返回。
type TranscodeDownloadResult struct {
	// DownloadURL 下载地址；转码空间已满时为空。
	DownloadURL string `json:"downloadUrl"`
	// IsFull 转码空间容量是否已满。
	IsFull bool `json:"isFull"`
}

// DownloadOriginal 获取转码空间原文件的下载地址。
func (s *TranscodeService) DownloadOriginal(ctx context.Context, fileID int64) (*TranscodeDownloadResult, error) {
	var out TranscodeDownloadResult
	err := s.client.post(ctx, "/api/v1/transcode/file/download", map[string]int64{"fileId": fileID}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadM3U8 获取某分辨率 m3u8 文件的下载地址。分辨率 P 大写，如 "1080P"。
func (s *TranscodeService) DownloadM3U8(ctx context.Context, fileID int64, resolution string) (*TranscodeDownloadResult, error) {
	var out TranscodeDownloadResult
	err := s.client.post(ctx, "/api/v1/transcode/m3u8_ts/download", map[string]any{
		"fileId":     fileID,
		"resolution": resolution,
		"type":       1,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadTS 获取某分辨率单个 ts 分片的下载地址。
// tsName 不含 ".ts" 后缀（Results 返回的 FileName 为 "000.ts" 时传 "000"）。
func (s *TranscodeService) DownloadTS(ctx context.Context, fileID int64, resolution, tsName string) (*TranscodeDownloadResult, error) {
	var out TranscodeDownloadResult
	err := s.client.post(ctx, "/api/v1/transcode/m3u8_ts/download", map[string]any{
		"fileId":     fileID,
		"resolution": resolution,
		"type":       2,
		"tsName":     tsName,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DownloadAllResult 是打包下载全部转码文件接口的返回。
type DownloadAllResult struct {
	// IsDownloading 为 true 表示服务端仍在打包，需继续轮询（官方建议间隔 10s）。
	IsDownloading bool `json:"isDownloading"`
	// IsFull 转码空间容量是否已满（满则无法下载）。
	IsFull bool `json:"isFull"`
	// DownloadURL 打包 zip 的下载地址，仅在未满且打包完成时有值。
	DownloadURL string `json:"downloadUrl"`
}

// DownloadAll 打包下载某个视频的全部转码文件。
// IsDownloading 为 true 时需轮询；返回的 URL 含 access_token，注意日志脱敏。
func (s *TranscodeService) DownloadAll(ctx context.Context, fileID int64, zipName string) (*DownloadAllResult, error) {
	var out DownloadAllResult
	err := s.client.post(ctx, "/api/v1/transcode/file/download/all", map[string]any{
		"fileId":  fileID,
		"zipName": zipName,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}
