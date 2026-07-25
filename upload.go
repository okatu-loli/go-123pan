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
//
// V2 分片上传流程：Create（预上传，返回 Reuse 为 true 即秒传结束）→
// UploadSlice（按 SliceSize 逐片上传到上传域名）→ Complete（轮询直至 Completed）。
// 单步上传流程（≤1GB 小文件）：Domains 获取上传域名 → SingleCreate 一次上传。
// V1 旧版流程：CreateV1 → GetUploadURLV1（每片换预签名 URL）→ PutSliceV1 →
// ListUploadPartsV1（可选比对）→ CompleteV1（Async 为 true 时转 AsyncResultV1 轮询）。
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
	// ContainDir 文件名是否携带路径，为 true 时自动创建不存在的中间目录。
	ContainDir bool `json:"containDir,omitempty"`
}

// UploadCreateResult 是创建文件（V2）的返回。
type UploadCreateResult struct {
	// FileID 秒传成功时返回的文件 ID。
	FileID int64 `json:"fileID"`
	// PreuploadID 预上传 ID（Reuse 为 true 时不存在），后续分片上传与完成接口凭此标识。
	PreuploadID string `json:"preuploadID"`
	// Reuse 为 true 表示秒传成功，上传结束。
	Reuse bool `json:"reuse"`
	// SliceSize 分片大小（字节），必须按此大小切分文件。
	SliceSize int64 `json:"sliceSize"`
	// Servers 上传域名（V2），后续上传分片必须使用其中之一。
	Servers []string `json:"servers"`
}

// Create 创建文件（V2 预上传），是 V2 分片上传流程的第一步。
//
// 接口: POST /upload/v2/file/create（主域名）
//
// 参数:
//   - req: 预上传参数，见 UploadCreateRequest；Etag 为整个文件的 MD5，
//     Size 单位字节，单文件上限 10GB。
//
// 返回 Reuse 为 true 时秒传成功、上传结束（FileID 即结果）；
// 否则按 SliceSize 切分文件，用返回的 Servers 之一调用 UploadSlice，
// 全部分片传完后调用 Complete。
func (s *UploadService) Create(ctx context.Context, req *UploadCreateRequest) (*UploadCreateResult, error) {
	var out UploadCreateResult
	if err := s.client.post(ctx, "/upload/v2/file/create", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UploadSlice 上传单个分片（V2），multipart/form-data 格式。
//
// 接口: POST {server}/upload/v2/file/slice（上传域名，非主域名）
//
// 参数:
//   - server: 上传域名，取 Create 返回的 Servers 之一。
//   - preuploadID: Create 返回的预上传 ID。
//   - sliceNo: 分片编号，从 1 开始。
//   - sliceMD5: 该分片内容的 MD5。
//   - slice: 分片内容，长度须等于 Create 返回的 SliceSize（末片可小于）。
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
	// Completed 为 true 表示服务端合并校验完成，上传成功。
	Completed bool `json:"completed"`
	// FileID 上传成功后的文件 ID（Completed 为 true 时有效）。
	FileID int64 `json:"fileID"`
}

// Complete 通知上传完毕（V2），是 V2 分片上传流程的最后一步。
//
// 接口: POST /upload/v2/file/upload_complete（主域名）
//
// 参数:
//   - preuploadID: Create 返回的预上传 ID。
//
// 返回 Completed 为 false 时表示服务端仍在合并校验，需间隔 1 秒重复调用轮询，
// 直至 Completed 为 true 并取得 FileID。
func (s *UploadService) Complete(ctx context.Context, preuploadID string) (*UploadCompleteResult, error) {
	var out UploadCompleteResult
	err := s.client.post(ctx, "/upload/v2/file/upload_complete", map[string]string{"preuploadID": preuploadID}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// Domains 获取单步上传的上传域名列表，是单步上传流程的第一步。
//
// 接口: GET /upload/v2/file/domain
//
// 返回的域名任选其一作为 SingleCreate 的 server 参数。
func (s *UploadService) Domains(ctx context.Context) ([]string, error) {
	var out []string
	if err := s.client.get(ctx, "/upload/v2/file/domain", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SingleCreateResult 是单步上传的返回。
type SingleCreateResult struct {
	// FileID 上传成功后的文件 ID。
	FileID int64 `json:"fileID"`
	// Completed 为 true 表示上传完成。
	Completed bool `json:"completed"`
}

// SingleCreate 单步上传：一次 multipart 请求完成小文件上传。
//
// 接口: POST {server}/upload/v2/file/single/create（上传域名，非主域名）
//
// 参数:
//   - server: 上传域名，取 Domains 返回的域名之一。
//   - req: 上传参数，见 UploadCreateRequest；Etag 为整个文件的 MD5。
//   - file: 文件内容，单文件上限 1GB；更大文件请走 Create 分片上传（上限 10GB）。
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
	// FileID 秒传成功后的文件 ID（Reuse 为 true 时有效）。
	FileID int64 `json:"fileID"`
	// Reuse 为 true 表示秒传成功。
	Reuse bool `json:"reuse"`
}

// Sha1Reuse 通过文件 sha1 尝试秒传。
//
// 接口: POST /upload/v2/file/sha1_reuse
//
// 参数:
//   - parentFileID: 父目录 ID，根目录为 0。
//   - filename: 文件名，小于 255 字符且不能包含 "\/:*?|>< 。
//   - sha1: 整个文件的 SHA1（注意：此接口用 SHA1，其余上传接口均用 MD5）。
//   - size: 文件大小（字节）。
//   - duplicate: 重名处理策略：0 默认，1 保留两者（自动加后缀），2 覆盖原文件。
//
// 返回 Reuse 为 false 时表示服务端无此文件，需改走正常上传流程（Create 或 SingleCreate）。
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
	// FileID 秒传成功时返回的文件 ID。
	FileID int64 `json:"fileID"`
	// PreuploadID 预上传 ID（Reuse 为 true 时不存在）。
	PreuploadID string `json:"preuploadID"`
	// Reuse 为 true 表示秒传成功，上传结束。
	Reuse bool `json:"reuse"`
	// SliceSize 分片大小（字节），必须按此大小切分文件。
	SliceSize int64 `json:"sliceSize"`
}

// CreateV1 创建文件（V1 旧版预上传）。推荐使用 Create（V2）。
//
// 接口: POST /upload/v1/file/create
//
// 参数:
//   - req: 预上传参数，见 UploadCreateRequest；Etag 为整个文件的 MD5。
//
// 返回 Reuse 为 true 时秒传成功；否则按 SliceSize 切分，逐片经
// GetUploadURLV1 换取预签名地址并 PutSliceV1 上传，最后调用 CompleteV1。
func (s *UploadService) CreateV1(ctx context.Context, req *UploadCreateRequest) (*UploadCreateResultV1, error) {
	var out UploadCreateResultV1
	if err := s.client.post(ctx, "/upload/v1/file/create", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUploadURLV1 获取分片的预签名上传地址（V1）。每个分片都需单独换取。
//
// 接口: POST /upload/v1/file/get_upload_url
//
// 参数:
//   - preuploadID: CreateV1 返回的预上传 ID。
//   - sliceNo: 分片编号，从 1 开始。
//
// 返回的预签名地址交给 PutSliceV1 上传分片内容。
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
//
// 接口: PUT {presignedURL}（预签名地址，非开放平台域名）
//
// 参数:
//   - presignedURL: GetUploadURLV1 返回的预签名上传地址。
//   - slice: 分片内容。
//   - size: 分片字节数，用于设置 Content-Length。
//
// 注意：此请求不携带 Authorization/Platform 头（官方要求），
// Content-Type 为 application/octet-stream。
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
	// Size 分片大小（字节）。
	Size int64 `json:"size"`
	// Etag 分片 MD5，可与本地计算值比对校验。
	Etag string `json:"etag"`
}

// ListUploadPartsV1 列举已上传分片（V1，非必需步骤），用于本地 MD5 比对校验。
//
// 接口: POST /upload/v1/file/list_upload_parts
//
// 参数:
//   - preuploadID: CreateV1 返回的预上传 ID。
//
// 注意：文件小于 sliceSize（即未真正分片）时返回空列表。
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
	Async bool `json:"async"`
	// Completed 为 true 表示上传完成。
	Completed bool `json:"completed"`
	// FileID 上传成功后的文件 ID（Completed 为 true 时有效）。
	FileID int64 `json:"fileID"`
}

// CompleteV1 通知上传完毕（V1）。
//
// 接口: POST /upload/v1/file/upload_complete
//
// 参数:
//   - preuploadID: CreateV1 返回的预上传 ID。
//
// 返回 Async 为 true 时表示服务端异步合并，需转 AsyncResultV1 轮询最终结果
// （间隔至少 1 秒）；Completed 为 true 时直接取 FileID。
func (s *UploadService) CompleteV1(ctx context.Context, preuploadID string) (*UploadCompleteResultV1, error) {
	var out UploadCompleteResultV1
	err := s.client.post(ctx, "/upload/v1/file/upload_complete", map[string]string{"preuploadID": preuploadID}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AsyncResultV1 异步轮询获取上传结果（V1）。
//
// 接口: POST /upload/v1/file/upload_async_result
//
// 参数:
//   - preuploadID: CreateV1 返回的预上传 ID。
//
// 返回 Completed 为 false 时至少间隔 1 秒再调用，直至 Completed 为 true 并取得 FileID。
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
//
// 接口: 组合调用 V2 分片上传流程（Create → UploadSlice → Complete），见 UploadFile
//
// 参数:
//   - parentFileID: 目标目录 ID，根目录为 0。
//   - path: 本地文件路径，文件名取自路径的 basename；单文件上限 10GB。
//
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
//
// 接口: 组合调用 V2 分片上传流程 POST /upload/v2/file/create →
// POST {server}/upload/v2/file/slice → POST /upload/v2/file/upload_complete
//
// 参数:
//   - parentFileID: 目标目录 ID，根目录为 0。
//   - filename: 文件名，小于 255 字符且不能包含 "\/:*?|>< 。
//   - r: 文件内容，需要支持随机读（*os.File、*bytes.Reader 等均满足）。
//   - size: 文件总大小（字节），单文件上限 10GB。
//
// 自动完成：MD5 计算、秒传检测（Reuse 命中即直接返回）、分片并发上传
// （并发数 3）、Complete 轮询（间隔 1 秒）直至服务端合并完成。
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
