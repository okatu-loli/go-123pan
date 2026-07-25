package pan123

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// OssService 提供图床相关接口：目录/图片上传、云盘图片复制、移动、删除、
// 图片信息查询、离线迁移。注意：图床的文件/目录 ID 为字符串类型。
type OssService struct {
	client *Client
}

// Mkdir 创建图床目录，parentID 为空字符串表示根目录。返回新目录 ID。
func (s *OssService) Mkdir(ctx context.Context, parentID, name string) (string, error) {
	var out struct {
		List []struct {
			Filename string `json:"filename"`
			DirID    string `json:"dirID"`
		} `json:"list"`
	}
	err := s.client.post(ctx, "/upload/v1/oss/file/mkdir", map[string]any{
		"name":     name,
		"parentID": parentID,
		"type":     1,
	}, &out)
	if err != nil {
		return "", err
	}
	if len(out.List) == 0 {
		return "", fmt.Errorf("pan123: oss mkdir returned empty list")
	}
	return out.List[0].DirID, nil
}

// OssCreateResult 是图床创建文件（预上传）的返回。
type OssCreateResult struct {
	// FileID 秒传成功时返回的文件 ID。
	FileID string `json:"fileID"`
	// PreuploadID 预上传 ID（Reuse 为 true 时不存在）。
	PreuploadID string `json:"preuploadID"`
	// Reuse 为 true 表示秒传成功。
	Reuse bool `json:"reuse"`
	// SliceSize 分片大小，必须按此大小切分。
	SliceSize int64 `json:"sliceSize"`
}

// CreateFile 图床创建文件（预上传）。parentFileID 为空字符串表示根目录；etag 为文件 MD5。
func (s *OssService) CreateFile(ctx context.Context, parentFileID, filename, etag string, size int64) (*OssCreateResult, error) {
	var out OssCreateResult
	err := s.client.post(ctx, "/upload/v1/oss/file/create", map[string]any{
		"parentFileID": parentFileID,
		"filename":     filename,
		"etag":         etag,
		"size":         size,
		"type":         1,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUploadURL 获取图床分片的预签名上传地址，sliceNo 从 1 开始。
// 获取后向该地址 PUT 分片二进制（可复用 Upload.PutSliceV1）。
func (s *OssService) GetUploadURL(ctx context.Context, preuploadID string, sliceNo int64) (string, error) {
	var out struct {
		PresignedURL string `json:"presignedURL"`
	}
	err := s.client.post(ctx, "/upload/v1/oss/file/get_upload_url", map[string]any{
		"preuploadID": preuploadID,
		"sliceNo":     sliceNo,
	}, &out)
	if err != nil {
		return "", err
	}
	return out.PresignedURL, nil
}

// OssUploadCompleteResult 是图床上传完毕的返回。
type OssUploadCompleteResult struct {
	FileID string `json:"fileID"`
	// Async 为 true 时需调用 UploadAsyncResult 轮询最终结果。
	Async     bool `json:"async"`
	Completed bool `json:"completed"`
}

// UploadComplete 通知图床上传完毕。
func (s *OssService) UploadComplete(ctx context.Context, preuploadID string) (*OssUploadCompleteResult, error) {
	var out OssUploadCompleteResult
	err := s.client.post(ctx, "/upload/v1/oss/file/upload_complete", map[string]string{"preuploadID": preuploadID}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// OssAsyncResult 是图床异步轮询上传结果的返回。
type OssAsyncResult struct {
	// Completed 为 false 时至少间隔 1 秒再轮询。
	Completed bool   `json:"completed"`
	FileID    string `json:"fileID"`
}

// UploadAsyncResult 图床异步轮询获取上传结果。
func (s *OssService) UploadAsyncResult(ctx context.Context, preuploadID string) (*OssAsyncResult, error) {
	var out OssAsyncResult
	err := s.client.post(ctx, "/upload/v1/oss/file/upload_async_result", map[string]string{"preuploadID": preuploadID}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UploadFromPath 上传本地图片到图床，返回文件 ID。
// parentFileID 为空字符串表示根目录。自动完成 MD5、秒传检测、分片上传与轮询。
func (s *OssService) UploadFromPath(ctx context.Context, parentFileID, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return "", err
	}
	return s.UploadFile(ctx, parentFileID, filepath.Base(path), f, stat.Size())
}

// UploadFile 上传图片到图床，返回文件 ID。r 需要支持随机读。
func (s *OssService) UploadFile(ctx context.Context, parentFileID, filename string, r io.ReaderAt, size int64) (string, error) {
	h := md5.New()
	if _, err := io.Copy(h, io.NewSectionReader(r, 0, size)); err != nil {
		return "", fmt.Errorf("pan123: compute md5: %w", err)
	}
	created, err := s.CreateFile(ctx, parentFileID, filename, hex.EncodeToString(h.Sum(nil)), size)
	if err != nil {
		return "", err
	}
	if created.Reuse {
		return created.FileID, nil
	}
	sliceSize := created.SliceSize
	if sliceSize <= 0 {
		return "", fmt.Errorf("pan123: oss create returned invalid sliceSize %d", sliceSize)
	}
	numSlices := (size + sliceSize - 1) / sliceSize
	if numSlices == 0 {
		numSlices = 1
	}
	for no := int64(1); no <= numSlices; no++ {
		presigned, err := s.GetUploadURL(ctx, created.PreuploadID, no)
		if err != nil {
			return "", err
		}
		offset := (no - 1) * sliceSize
		length := sliceSize
		if offset+length > size {
			length = size - offset
		}
		part := io.NewSectionReader(r, offset, length)
		if err := s.client.Upload.PutSliceV1(ctx, presigned, part, length); err != nil {
			return "", fmt.Errorf("pan123: oss upload slice %d: %w", no, err)
		}
	}
	done, err := s.UploadComplete(ctx, created.PreuploadID)
	if err != nil {
		return "", err
	}
	if !done.Async && done.FileID != "" {
		return done.FileID, nil
	}
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
		res, err := s.UploadAsyncResult(ctx, created.PreuploadID)
		if err != nil {
			return "", err
		}
		if res.Completed {
			return res.FileID, nil
		}
	}
}

// CopyFromDisk 创建"复制云盘图片到图床"任务，返回任务 ID。
// fileIDs 为云盘文件/目录 ID（最多 100 个）；toParentFileID 为空表示图床根目录。
// 限制：并发任务 3 个，单任务图片总数 1000 张，单图 100M。
func (s *OssService) CopyFromDisk(ctx context.Context, fileIDs []int64, toParentFileID string) (taskID string, err error) {
	ids := make([]string, len(fileIDs))
	for i, id := range fileIDs {
		ids[i] = strconv.FormatInt(id, 10)
	}
	var out struct {
		TaskID string `json:"taskID"`
	}
	err = s.client.post(ctx, "/api/v1/oss/source/copy", map[string]any{
		"fileIDs":        ids,
		"toParentFileID": toParentFileID,
		"sourceType":     1,
		"type":           1,
	}, &out)
	if err != nil {
		return "", err
	}
	return out.TaskID, nil
}

// OssCopyStatus 复制任务状态：1 进行中，2 结束，3 失败，4 等待。
type OssCopyStatus int

const (
	OssCopyRunning OssCopyStatus = 1
	OssCopyDone    OssCopyStatus = 2
	OssCopyFailed  OssCopyStatus = 3
	OssCopyWaiting OssCopyStatus = 4
)

// CopyProcess 查询复制任务状态与失败原因。
func (s *OssService) CopyProcess(ctx context.Context, taskID string) (OssCopyStatus, string, error) {
	q := url.Values{}
	q.Set("taskID", taskID)
	var out struct {
		Status  int    `json:"status"`
		FailMsg string `json:"failMsg"`
	}
	if err := s.client.get(ctx, "/api/v1/oss/source/copy/process", q, &out); err != nil {
		return 0, "", err
	}
	return OssCopyStatus(out.Status), out.FailMsg, nil
}

// OssCopyFailResult 是复制失败文件列表的返回。
type OssCopyFailResult struct {
	Total int64 `json:"total"`
	List  []struct {
		FileID   int64  `json:"fileId"`
		Filename string `json:"filename"`
	} `json:"list"`
}

// CopyFailList 分页查询复制失败的文件列表，page 从 1 开始，limit 最大 100。
func (s *OssService) CopyFailList(ctx context.Context, taskID string, page, limit int) (*OssCopyFailResult, error) {
	q := url.Values{}
	q.Set("taskID", taskID)
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))
	var out OssCopyFailResult
	if err := s.client.get(ctx, "/api/v1/oss/source/copy/fail", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Move 批量移动图片到目标目录（toParentFileID 不能为空），单级最多 100 个。
func (s *OssService) Move(ctx context.Context, fileIDs []string, toParentFileID string) error {
	return s.client.post(ctx, "/api/v1/oss/file/move", map[string]any{
		"fileIDs":        fileIDs,
		"toParentFileID": toParentFileID,
	}, nil)
}

// Delete 批量删除图片，一次最多 100 个。
func (s *OssService) Delete(ctx context.Context, fileIDs []string) error {
	return s.client.post(ctx, "/api/v1/oss/file/delete", map[string]any{"fileIDs": fileIDs}, nil)
}

// OssFile 是图床文件信息。
type OssFile struct {
	FileID   string `json:"fileId"`
	Filename string `json:"filename"`
	// Type 0-文件 1-文件夹。
	Type int    `json:"type"`
	Size int64  `json:"size"`
	Etag string `json:"etag"`
	// Status 文件审核状态，大于 100 为审核驳回文件。
	Status   int    `json:"status"`
	CreateAt string `json:"createAt"`
	UpdateAt string `json:"updateAt"`
	// DownloadURL 图片下载/外链地址。
	DownloadURL string `json:"downloadURL"`
	// UserSelfURL 自定义域名链接。
	UserSelfURL string `json:"userSelfURL"`
	// TotalTraffic 流量统计（字节）。
	TotalTraffic   int64  `json:"totalTraffic"`
	ParentFileID   string `json:"parentFileId"`
	ParentFilename string `json:"parentFilename"`
	// Extension 后缀名，如 jpg。
	Extension string `json:"extension"`
}

// Detail 获取图片详情。
func (s *OssService) Detail(ctx context.Context, fileID string) (*OssFile, error) {
	q := url.Values{}
	q.Set("fileID", fileID)
	var out OssFile
	if err := s.client.get(ctx, "/api/v1/oss/file/detail", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// OssListRequest 是图片列表的查询参数。
type OssListRequest struct {
	// ParentFileID 父目录 ID，空表示根目录。
	ParentFileID string `json:"parentFileId,omitempty"`
	// Limit 每页数量，最大 100。
	Limit int `json:"limit"`
	// StartTime/EndTime 按创建时间筛选（Unix 时间戳，可选）。
	StartTime int64 `json:"startTime,omitempty"`
	EndTime   int64 `json:"endTime,omitempty"`
	// LastFileID 翻页游标，取上一页返回值；返回 "-1" 表示最后一页。
	LastFileID string `json:"lastFileId,omitempty"`
}

// OssListResult 是图片列表的返回。
type OssListResult struct {
	// LastFileID 为 "-1" 表示最后一页。
	LastFileID string    `json:"lastFileId"`
	FileList   []OssFile `json:"fileList"`
}

// List 获取图片列表（lastFileId 游标翻页，注意游标为字符串，"-1" 表示结束）。
func (s *OssService) List(ctx context.Context, req *OssListRequest) (*OssListResult, error) {
	if req == nil {
		req = &OssListRequest{}
	}
	if req.Limit <= 0 {
		req.Limit = 100
	}
	body := map[string]any{
		"limit": req.Limit,
		"type":  1,
	}
	if req.ParentFileID != "" {
		body["parentFileId"] = req.ParentFileID
	}
	if req.StartTime > 0 {
		body["startTime"] = req.StartTime
	}
	if req.EndTime > 0 {
		body["endTime"] = req.EndTime
	}
	if req.LastFileID != "" {
		body["lastFileId"] = req.LastFileID
	}
	var out OssListResult
	if err := s.client.post(ctx, "/api/v1/oss/file/list", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// OfflineDownload 创建图床离线迁移任务（从 URL 拉取图片到图床），返回任务 ID。
// 仅支持 http/https；支持 png/gif/jpeg/tiff/webp/jpg/tif/svg/bmp，单图 100M。
func (s *OssService) OfflineDownload(ctx context.Context, req *OfflineDownloadRequest) (taskID int64, err error) {
	body := map[string]any{
		"url":  req.URL,
		"type": 1,
	}
	if req.FileName != "" {
		body["fileName"] = req.FileName
	}
	if req.DirID != 0 {
		body["dirID"] = strconv.FormatInt(req.DirID, 10)
	}
	if req.CallBackURL != "" {
		body["callBackUrl"] = req.CallBackURL
	}
	var out struct {
		TaskID int64 `json:"taskID"`
	}
	if err := s.client.post(ctx, "/api/v1/oss/offline/download", body, &out); err != nil {
		return 0, err
	}
	return out.TaskID, nil
}

// OfflineProcess 查询图床离线迁移任务进度。
// 状态枚举与云盘离线下载一致：0 进行中、1 失败、2 成功、3 重试中。
func (s *OssService) OfflineProcess(ctx context.Context, taskID int64) (*OfflineProcessResult, error) {
	q := url.Values{}
	q.Set("taskID", strconv.FormatInt(taskID, 10))
	var out OfflineProcessResult
	if err := s.client.get(ctx, "/api/v1/oss/offline/download/process", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
