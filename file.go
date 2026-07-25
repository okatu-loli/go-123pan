package pan123

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// FileService 提供文件管理相关接口：目录、重命名、删除/还原、复制、移动、详情、列表、保险箱、下载。
type FileService struct {
	client *Client
}

// FileInfo 是 v2 文件列表及多文件详情返回的文件信息。
type FileInfo struct {
	FileID   int64  `json:"fileId"`
	Filename string `json:"filename"`
	// Type 0-文件 1-文件夹。
	Type int    `json:"type"`
	Size int64  `json:"size"`
	Etag string `json:"etag"`
	// Status 文件审核状态，大于 100 为审核驳回文件。
	Status       int   `json:"status"`
	ParentFileID int64 `json:"parentFileId"`
	// Category 文件分类：0-未知 1-音频 2-视频 3-图片。
	Category int `json:"category"`
	// Trashed 是否在回收站：0-否 1-是。
	Trashed     int    `json:"trashed"`
	PunishFlag  int    `json:"punishFlag"`
	S3KeyFlag   string `json:"s3KeyFlag"`
	StorageNode string `json:"storageNode"`
	CreateAt    string `json:"createAt"`
	UpdateAt    string `json:"updateAt"`
}

// Mkdir 创建目录，返回新目录 ID。上传到根目录时 parentID 填 0。目录名不能与同级重名。
func (s *FileService) Mkdir(ctx context.Context, parentID int64, name string) (int64, error) {
	var out struct {
		DirID int64 `json:"dirID"`
	}
	err := s.client.post(ctx, "/upload/v1/file/mkdir", map[string]any{
		"name":     name,
		"parentID": parentID,
	}, &out)
	if err != nil {
		return 0, err
	}
	return out.DirID, nil
}

// Rename 重命名单个文件。
func (s *FileService) Rename(ctx context.Context, fileID int64, newName string) error {
	return s.client.put(ctx, "/api/v1/file/name", map[string]any{
		"fileId":   fileID,
		"fileName": newName,
	}, nil)
}

// BatchRenameResult 是批量重命名的返回（可能为空）。
type BatchRenameResult struct {
	SuccessList []struct {
		FileID   int64  `json:"fileID"`
		UpdateAt string `json:"updateAt"`
	} `json:"successList"`
	FailList []struct {
		FileID  int64  `json:"fileID"`
		Message string `json:"message"`
	} `json:"failList"`
}

// BatchRename 批量重命名，key 为文件 ID、value 为新文件名，一次最多 30 个。
func (s *FileService) BatchRename(ctx context.Context, renames map[int64]string) (*BatchRenameResult, error) {
	list := make([]string, 0, len(renames))
	for id, name := range renames {
		list = append(list, fmt.Sprintf("%d|%s", id, name))
	}
	var out BatchRenameResult
	err := s.client.post(ctx, "/api/v1/file/rename", map[string]any{"renameList": list}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Trash 删除文件至回收站，一次最多 100 个。
func (s *FileService) Trash(ctx context.Context, fileIDs []int64) error {
	return s.client.post(ctx, "/api/v1/file/trash", map[string]any{"fileIDs": fileIDs}, nil)
}

// Recover 从回收站恢复文件至原位置，一次最多 100 个。
// 返回父级目录已不存在的异常文件 ID，可用 RecoverTo 还原到指定目录。
func (s *FileService) Recover(ctx context.Context, fileIDs []int64) (abnormalFileIDs []int64, err error) {
	var out struct {
		AbnormalFileIDs []int64 `json:"abnormalFileIDs"`
	}
	err = s.client.post(ctx, "/api/v1/file/recover", map[string]any{"fileIDs": fileIDs}, &out)
	if err != nil {
		return nil, err
	}
	return out.AbnormalFileIDs, nil
}

// RecoverTo 将回收站文件还原到指定目录，一次最多 100 个。
func (s *FileService) RecoverTo(ctx context.Context, fileIDs []int64, parentFileID int64) error {
	return s.client.post(ctx, "/api/v1/file/recover/by_path", map[string]any{
		"fileIDs":      fileIDs,
		"parentFileID": parentFileID,
	}, nil)
}

// CopyResult 是复制单个文件的返回。
type CopyResult struct {
	SourceFileID int64 `json:"sourceFileId"`
	TargetFileID int64 `json:"targetFileId"`
}

// Copy 复制单个文件到目标目录（同步接口）。
func (s *FileService) Copy(ctx context.Context, fileID, targetDirID int64) (*CopyResult, error) {
	var out CopyResult
	// 文档参数表为 targetDirId，官方示例实际发送 targetDirID，两者都带以兼容。
	err := s.client.post(ctx, "/api/v1/file/copy", map[string]any{
		"fileId":      fileID,
		"targetDirId": targetDirID,
		"targetDirID": targetDirID,
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AsyncCopy 批量复制文件（异步），单级最多 3000 个，返回任务 ID。
// 用 CopyProcess 轮询进度。
func (s *FileService) AsyncCopy(ctx context.Context, fileIDs []int64, targetDirID int64) (taskID int64, err error) {
	var out struct {
		TaskID int64 `json:"taskId"`
	}
	err = s.client.post(ctx, "/api/v1/file/async/copy", map[string]any{
		"fileIds":     fileIDs,
		"targetDirId": targetDirID,
		"targetDirID": targetDirID,
	}, &out)
	if err != nil {
		return 0, err
	}
	return out.TaskID, nil
}

// CopyTaskStatus 是批量复制任务状态：0-待处理 1-进行中 2-已完成 3-失败。
type CopyTaskStatus int

const (
	CopyTaskPending CopyTaskStatus = 0
	CopyTaskRunning CopyTaskStatus = 1
	CopyTaskDone    CopyTaskStatus = 2
	CopyTaskFailed  CopyTaskStatus = 3
)

// CopyProcess 查询批量复制任务进度。
func (s *FileService) CopyProcess(ctx context.Context, taskID int64) (CopyTaskStatus, error) {
	var out struct {
		TaskID int64 `json:"taskId"`
		Status int   `json:"status"`
	}
	q := url.Values{}
	q.Set("taskId", strconv.FormatInt(taskID, 10))
	// 官方文档此接口为 GET 携带 JSON body 的非常规设计，同时附带 query 以兼容。
	err := s.client.do(ctx, http.MethodGet, "/api/v1/file/async/copy/process", q, map[string]int64{"taskId": taskID}, &out, true)
	if err != nil {
		return 0, err
	}
	return CopyTaskStatus(out.Status), nil
}

// Move 批量移动文件，单级最多 100 个。移动到根目录时 toParentFileID 填 0。
func (s *FileService) Move(ctx context.Context, fileIDs []int64, toParentFileID int64) error {
	return s.client.post(ctx, "/api/v1/file/move", map[string]any{
		"fileIDs":        fileIDs,
		"toParentFileID": toParentFileID,
	}, nil)
}

// FileDetail 是单个文件详情。
type FileDetail struct {
	FileID       int64  `json:"fileID"`
	Filename     string `json:"filename"`
	Type         int    `json:"type"`
	Size         int64  `json:"size"`
	Etag         string `json:"etag"`
	Status       int    `json:"status"`
	ParentFileID int64  `json:"parentFileID"`
	CreateAt     string `json:"createAt"`
	Trashed      int    `json:"trashed"`
}

// Detail 获取单个文件详情；查询文件夹时 size 为文件夹累计大小。
func (s *FileService) Detail(ctx context.Context, fileID int64) (*FileDetail, error) {
	q := url.Values{}
	q.Set("fileID", strconv.FormatInt(fileID, 10))
	var out FileDetail
	if err := s.client.get(ctx, "/api/v1/file/detail", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Infos 批量获取文件详情。
func (s *FileService) Infos(ctx context.Context, fileIDs []int64) ([]FileInfo, error) {
	var out struct {
		FileList []FileInfo `json:"fileList"`
	}
	err := s.client.post(ctx, "/api/v1/file/infos", map[string]any{"fileIds": fileIDs}, &out)
	if err != nil {
		return nil, err
	}
	return out.FileList, nil
}

// FileListRequest 是 v2 文件列表（推荐）的查询参数。
type FileListRequest struct {
	// ParentFileID 文件夹 ID，根目录为 0。
	ParentFileID int64
	// Limit 每页数量，最大 100；0 时默认 100。
	Limit int
	// SearchData 搜索关键字；非空时忽略 ParentFileID 做全局搜索。
	SearchData string
	// SearchMode 0-模糊搜索 1-精准搜索。
	SearchMode int
	// LastFileID 翻页游标，取上一页返回的 LastFileID。
	LastFileID int64
}

func (r *FileListRequest) values() url.Values {
	q := url.Values{}
	if r == nil {
		r = &FileListRequest{}
	}
	limit := r.Limit
	if limit <= 0 {
		limit = 100
	}
	q.Set("parentFileId", strconv.FormatInt(r.ParentFileID, 10))
	q.Set("limit", strconv.Itoa(limit))
	if r.SearchData != "" {
		q.Set("searchData", r.SearchData)
		q.Set("searchMode", strconv.Itoa(r.SearchMode))
	}
	if r.LastFileID > 0 {
		q.Set("lastFileId", strconv.FormatInt(r.LastFileID, 10))
	}
	return q
}

// FileListResult 是 v2 文件列表的返回。
type FileListResult struct {
	// LastFileID 为 -1 表示最后一页；否则作为下一页的翻页游标。
	LastFileID int64      `json:"lastFileId"`
	FileList   []FileInfo `json:"fileList"`
}

// List 获取文件列表（v2 推荐接口，lastFileId 游标翻页）。
// 注意：结果包含回收站文件，需按 Trashed 字段过滤。
func (s *FileService) List(ctx context.Context, req *FileListRequest) (*FileListResult, error) {
	var out FileListResult
	if err := s.client.get(ctx, "/api/v2/file/list", req.values(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListAll 自动翻页拉取目录下全部文件（已过滤回收站中的文件）。
func (s *FileService) ListAll(ctx context.Context, parentFileID int64) ([]FileInfo, error) {
	var all []FileInfo
	req := &FileListRequest{ParentFileID: parentFileID, Limit: 100}
	for {
		page, err := s.List(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, f := range page.FileList {
			if f.Trashed == 0 {
				all = append(all, f)
			}
		}
		if page.LastFileID == -1 || len(page.FileList) == 0 {
			return all, nil
		}
		req.LastFileID = page.LastFileID
	}
}

// FileInfoV1 是 v1 旧版文件列表的文件信息。
type FileInfoV1 struct {
	FileID       int64  `json:"fileID"`
	Filename     string `json:"filename"`
	Type         int    `json:"type"`
	Size         int64  `json:"size"`
	Etag         string `json:"etag"`
	Status       int    `json:"status"`
	ParentFileID int64  `json:"parentFileId"`
	ParentName   string `json:"parentName"`
	Category     int    `json:"category"`
	CreateAt     string `json:"createAt"`
	UpdateAt     string `json:"updateAt"`
	Thumbnail    string `json:"thumbnail"`
	DownloadURL  string `json:"downloadUrl"`
}

// FileListV1Request 是 v1 旧版文件列表的查询参数。
type FileListV1Request struct {
	ParentFileID int64
	// Page 页码，从 1 开始。
	Page  int
	Limit int
	// OrderBy 排序字段：file_id、size、file_name。
	OrderBy string
	// OrderDirection 排序方向：asc、desc。
	OrderDirection string
	// Trashed 是否查看回收站文件。
	Trashed    bool
	SearchData string
}

// FileListV1Result 是 v1 旧版文件列表的返回。
type FileListV1Result struct {
	Total    int          `json:"total"`
	FileList []FileInfoV1 `json:"fileList"`
}

// ListV1 获取文件列表（旧版 v1，page 页码翻页）。官方推荐使用 List（v2）。
func (s *FileService) ListV1(ctx context.Context, req *FileListV1Request) (*FileListV1Result, error) {
	if req == nil {
		req = &FileListV1Request{}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}
	orderBy := req.OrderBy
	if orderBy == "" {
		orderBy = "file_id"
	}
	dir := req.OrderDirection
	if dir == "" {
		dir = "asc"
	}
	q := url.Values{}
	q.Set("parentFileId", strconv.FormatInt(req.ParentFileID, 10))
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))
	q.Set("orderBy", orderBy)
	q.Set("orderDirection", dir)
	if req.Trashed {
		q.Set("trashed", "true")
	}
	if req.SearchData != "" {
		q.Set("searchData", req.SearchData)
	}
	var out FileListV1Result
	if err := s.client.get(ctx, "/api/v1/file/list", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SafeboxID 解锁保险箱并获取保险箱目录 ID，可作为 parentFileID 使用。
func (s *FileService) SafeboxID(ctx context.Context, password string) (int64, error) {
	q := url.Values{}
	q.Set("password", password)
	var out struct {
		FileID int64 `json:"fileId"`
	}
	if err := s.client.get(ctx, "/api/v1/safebox/id", q, &out); err != nil {
		return 0, err
	}
	return out.FileID, nil
}

// DownloadInfo 获取文件的临时下载直链。
// 免费账号每日自用下载流量 1GB，超限返回 code 5113；文件不存在返回 code 5066。
func (s *FileService) DownloadInfo(ctx context.Context, fileID int64) (string, error) {
	q := url.Values{}
	q.Set("fileId", strconv.FormatInt(fileID, 10))
	var out struct {
		DownloadURL string `json:"downloadUrl"`
	}
	if err := s.client.get(ctx, "/api/v1/file/download_info", q, &out); err != nil {
		return "", err
	}
	return out.DownloadURL, nil
}

// DownloadTo 获取下载直链并把文件内容写入 w，返回写入的字节数。
func (s *FileService) DownloadTo(ctx context.Context, fileID int64, w io.Writer) (int64, error) {
	downloadURL, err := s.DownloadInfo(ctx, fileID)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("pan123: download failed with http %d", resp.StatusCode)
	}
	return io.Copy(w, resp.Body)
}
