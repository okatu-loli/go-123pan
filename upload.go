package pan123

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UploadService 提供文件上传相关接口：V2 分片上传（推荐）、单步上传、V1 旧版流程、sha1 秒传，
// 以及一行代码上传的高级封装 UploadFile / UploadFromPath。
type UploadService struct {
	client *Client
}

// UploadCreateRequest 是创建文件（预上传）的参数。
type UploadCreateRequest struct {
	// ParentFileID 父目录 ID，根目录为 0。
	ParentFileID int64 `json:"parentFileID"`
	// Filename 文件名，小于 255 字符且不能包含 "\/:*?|>< 。
	// ContainDir 为 true 时传入"路径+文件名"，如 /你好/123/测试文件.mp4。
	Filename string `json:"filename"`
	// Etag 文件 MD5。
	Etag string `json:"etag"`
	// Size 文件大小（字节）。
	Size int64 `json:"size"`
	// Duplicate 重名处理策略：0 默认，1 保留两者（自动加后缀），2 覆盖原文件。
	Duplicate int `json:"duplicate,omitempty"`
	// ContainDir 文件名是否携带路径。
	ContainDir bool `json:"containDir,omitempty"`
}

// UploadCreateResult 是创建文件（V2）的返回。
type UploadCreateResult struct {
	// FileID 秒传成功时返回的文件 ID。
	FileID int64 `json:"fileID"`
	// PreuploadID 预上传 ID（Reuse 为 true 时不存在）。
	PreuploadID string `json:"preuploadID"`
	// Reuse 为 true 表示秒传成功，上传结束。
	Reuse bool `json:"reuse"`
	// SliceSize 分片大小，必须按此大小切分文件。
	SliceSize int64 `json:"sliceSize"`
	// Servers 上传域名（V2），后续上传分片必须使用其中之一。
	Servers []string `json:"servers"`
}

// Create 创建文件（V2 预上传）。单文件上限 10GB。
// 返回 Reuse 为 true 时秒传成功；否则按 SliceSize 切分后调用 UploadSlice。
func (s *UploadService) Create(ctx context.Context, req *UploadCreateRequest) (*UploadCreateResult, error) {
	var out UploadCreateResult
	if err := s.client.post(ctx, "/upload/v2/file/create", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UploadSlice 上传单个分片（V2）。server 取 Create 返回的 Servers 之一；sliceNo 从 1 开始。
func (s *UploadService) UploadSlice(ctx context.Context, server, preuploadID string, sliceNo int64, sliceMD5 string, slice io.Reader) error {
	fields := map[string]string{
		"preuploadID": preuploadID,
		"sliceNo":     fmt.Sprintf("%d", sliceNo),
		"sliceMD5":    sliceMD5,
	}
	return s.client.postMultipart(ctx, server+"/upload/v2/file/slice", fields, "slice", "slice", slice, nil)
}

// UploadCompleteResult 是上传完毕（V2）的返回。
type UploadCompleteResult struct {
	Completed bool  `json:"completed"`
	FileID    int64 `json:"fileID"`
}

// Complete 通知上传完毕（V2）。Completed 为 false 时需间隔 1 秒重复调用轮询。
func (s *UploadService) Complete(ctx context.Context, preuploadID string) (*UploadCompleteResult, error) {
	var out UploadCompleteResult
	err := s.client.post(ctx, "/upload/v2/file/upload_complete", map[string]string{"preuploadID": preuploadID}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Domains 获取单步上传的上传域名列表。
func (s *UploadService) Domains(ctx context.Context) ([]string, error) {
	var out []string
	if err := s.client.get(ctx, "/upload/v2/file/domain", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SingleCreateResult 是单步上传的返回。
type SingleCreateResult struct {
	FileID    int64 `json:"fileID"`
	Completed bool  `json:"completed"`
}

// SingleCreate 单步上传：一次请求完成小文件上传，单文件上限 1GB。
// server 取 Domains 返回的域名之一；req.Etag 为整个文件的 MD5。
func (s *UploadService) SingleCreate(ctx context.Context, server string, req *UploadCreateRequest, file io.Reader) (*SingleCreateResult, error) {
	fields := map[string]string{
		"parentFileID": fmt.Sprintf("%d", req.ParentFileID),
		"filename":     req.Filename,
		"etag":         req.Etag,
		"size":         fmt.Sprintf("%d", req.Size),
	}
	if req.Duplicate != 0 {
		fields["duplicate"] = fmt.Sprintf("%d", req.Duplicate)
	}
	if req.ContainDir {
		fields["containDir"] = "true"
	}
	var out SingleCreateResult
	err := s.client.postMultipart(ctx, server+"/upload/v2/file/single/create", fields, "file", req.Filename, file, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Sha1ReuseResult 是 sha1 秒传的返回。
type Sha1ReuseResult struct {
	FileID int64 `json:"fileID"`
	Reuse  bool  `json:"reuse"`
}

// Sha1Reuse 通过文件 sha1 尝试秒传。Reuse 为 false 时需改走正常上传流程。
// duplicate：0 默认，1 保留两者，2 覆盖原文件。
func (s *UploadService) Sha1Reuse(ctx context.Context, parentFileID int64, filename, sha1 string, size int64, duplicate int) (*Sha1ReuseResult, error) {
	body := map[string]any{
		"parentFileID": parentFileID,
		"filename":     filename,
		"sha1":         sha1,
		"size":         size,
	}
	if duplicate != 0 {
		body["duplicate"] = duplicate
	}
	var out Sha1ReuseResult
	if err := s.client.post(ctx, "/upload/v2/file/sha1_reuse", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- V1 旧版上传流程 ----

// UploadCreateResultV1 是创建文件（V1 旧版）的返回。
type UploadCreateResultV1 struct {
	FileID      int64  `json:"fileID"`
	PreuploadID string `json:"preuploadID"`
	Reuse       bool   `json:"reuse"`
	SliceSize   int64  `json:"sliceSize"`
}

// CreateV1 创建文件（V1 旧版预上传）。推荐使用 Create（V2）。
func (s *UploadService) CreateV1(ctx context.Context, req *UploadCreateRequest) (*UploadCreateResultV1, error) {
	var out UploadCreateResultV1
	if err := s.client.post(ctx, "/upload/v1/file/create", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUploadURLV1 获取分片的预签名上传地址（V1），sliceNo 从 1 开始。
func (s *UploadService) GetUploadURLV1(ctx context.Context, preuploadID string, sliceNo int64) (string, error) {
	var out struct {
		PresignedURL string `json:"presignedURL"`
	}
	err := s.client.post(ctx, "/upload/v1/file/get_upload_url", map[string]any{
		"preuploadID": preuploadID,
		"sliceNo":     sliceNo,
	}, &out)
	if err != nil {
		return "", err
	}
	return out.PresignedURL, nil
}

// PutSliceV1 向预签名地址 PUT 上传分片二进制（V1）。
// 注意：此请求不携带 Authorization/Platform 头（官方要求）。
func (s *UploadService) PutSliceV1(ctx context.Context, presignedURL string, slice io.Reader, size int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, presignedURL, slice)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("pan123: put slice failed with http %d: %s", resp.StatusCode, body)
	}
	return nil
}

// UploadedPart 是已上传分片信息（V1）。
type UploadedPart struct {
	// PartNumber 分片编号（服务端可能返回字符串，用 json.Number 兼容）。
	PartNumber json.Number `json:"partNumber"`
	Size       int64       `json:"size"`
	Etag       string      `json:"etag"`
}

// ListUploadPartsV1 列举已上传分片（V1，非必需），用于本地 MD5 比对。
// 文件小于 sliceSize 时返回空。
func (s *UploadService) ListUploadPartsV1(ctx context.Context, preuploadID string) ([]UploadedPart, error) {
	var out struct {
		Parts []UploadedPart `json:"parts"`
	}
	err := s.client.post(ctx, "/upload/v1/file/list_upload_parts", map[string]string{"preuploadID": preuploadID}, &out)
	if err != nil {
		return nil, err
	}
	return out.Parts, nil
}

// UploadCompleteResultV1 是上传完毕（V1）的返回。
type UploadCompleteResultV1 struct {
	// Async 为 true 时需调用 AsyncResultV1 轮询最终结果。
	Async     bool  `json:"async"`
	Completed bool  `json:"completed"`
	FileID    int64 `json:"fileID"`
}

// CompleteV1 通知上传完毕（V1）。
func (s *UploadService) CompleteV1(ctx context.Context, preuploadID string) (*UploadCompleteResultV1, error) {
	var out UploadCompleteResultV1
	err := s.client.post(ctx, "/upload/v1/file/upload_complete", map[string]string{"preuploadID": preuploadID}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AsyncResultV1 异步轮询获取上传结果（V1）。Completed 为 false 时至少间隔 1 秒再调用。
func (s *UploadService) AsyncResultV1(ctx context.Context, preuploadID string) (*UploadCompleteResult, error) {
	var out UploadCompleteResult
	err := s.client.post(ctx, "/upload/v1/file/upload_async_result", map[string]string{"preuploadID": preuploadID}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ---- 高级封装 ----

// uploadConcurrency 分片并发上传数。
const uploadConcurrency = 3

// UploadFromPath 上传本地文件到指定目录，返回文件 ID。
// 自动完成：MD5 计算、秒传检测、按服务端分片大小切分并发上传、轮询完成。
func (s *UploadService) UploadFromPath(ctx context.Context, parentFileID int64, path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return s.UploadFile(ctx, parentFileID, filepath.Base(path), f, stat.Size())
}

// UploadFile 上传文件到指定目录，返回文件 ID。
// r 需要支持随机读（*os.File、*bytes.Reader 等均满足）；size 为文件总大小。
// 自动完成：MD5 计算、秒传检测、分片并发上传、轮询完成。
func (s *UploadService) UploadFile(ctx context.Context, parentFileID int64, filename string, r io.ReaderAt, size int64) (int64, error) {
	// 计算全文件 MD5
	h := md5.New()
	if _, err := io.Copy(h, io.NewSectionReader(r, 0, size)); err != nil {
		return 0, fmt.Errorf("pan123: compute md5: %w", err)
	}
	etag := hex.EncodeToString(h.Sum(nil))

	created, err := s.Create(ctx, &UploadCreateRequest{
		ParentFileID: parentFileID,
		Filename:     filename,
		Etag:         etag,
		Size:         size,
	})
	if err != nil {
		return 0, err
	}
	if created.Reuse {
		return created.FileID, nil
	}
	if len(created.Servers) == 0 {
		return 0, fmt.Errorf("pan123: create upload returned no servers")
	}
	server := created.Servers[0]
	sliceSize := created.SliceSize
	if sliceSize <= 0 {
		return 0, fmt.Errorf("pan123: create upload returned invalid sliceSize %d", sliceSize)
	}

	// 并发上传分片
	numSlices := (size + sliceSize - 1) / sliceSize
	if numSlices == 0 {
		numSlices = 1
	}
	sem := make(chan struct{}, uploadConcurrency)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	uploadCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for no := int64(1); no <= numSlices; no++ {
		select {
		case sem <- struct{}{}:
		case <-uploadCtx.Done():
			no = numSlices // 结束派发
			continue
		}
		wg.Add(1)
		go func(sliceNo int64) {
			defer wg.Done()
			defer func() { <-sem }()
			offset := (sliceNo - 1) * sliceSize
			length := sliceSize
			if offset+length > size {
				length = size - offset
			}
			sh := md5.New()
			if _, err := io.Copy(sh, io.NewSectionReader(r, offset, length)); err != nil {
				recordErr(&mu, &firstErr, fmt.Errorf("pan123: slice %d md5: %w", sliceNo, err), cancel)
				return
			}
			sliceMD5 := hex.EncodeToString(sh.Sum(nil))
			part := io.NewSectionReader(r, offset, length)
			if err := s.UploadSlice(uploadCtx, server, created.PreuploadID, sliceNo, sliceMD5, part); err != nil {
				recordErr(&mu, &firstErr, fmt.Errorf("pan123: upload slice %d: %w", sliceNo, err), cancel)
			}
		}(no)
	}
	wg.Wait()
	if firstErr != nil {
		return 0, firstErr
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	// 上传完毕并轮询结果（间隔 1 秒）
	for {
		res, err := s.Complete(ctx, created.PreuploadID)
		if err != nil {
			return 0, err
		}
		if res.Completed && res.FileID != 0 {
			return res.FileID, nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func recordErr(mu *sync.Mutex, dst *error, err error, cancel context.CancelFunc) {
	mu.Lock()
	if *dst == nil {
		*dst = err
	}
	mu.Unlock()
	cancel()
}

// postMultipart 以 multipart/form-data 向 fullURL 发起 POST（携带鉴权头），并解析统一响应。
func (c *Client) postMultipart(ctx context.Context, fullURL string, fields map[string]string, fileField, filename string, file io.Reader, out any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		var werr error
		defer func() { pw.CloseWithError(werr) }()
		for k, v := range fields {
			if werr = mw.WriteField(k, v); werr != nil {
				return
			}
		}
		part, err := mw.CreateFormFile(fileField, filename)
		if err != nil {
			werr = err
			return
		}
		if _, werr = io.Copy(part, file); werr != nil {
			return
		}
		werr = mw.Close()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Platform", platformHeader)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
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
