package pan123

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ShareService 提供分享管理相关接口：免费/付费分享链接的创建、列表与修改。
type ShareService struct {
	client *Client
}

// ShareURL 按官方规则拼接分享页面链接：https://{uid}.share.123pan.cn/123pan/{shareKey}。
// uid 可通过 client.User.Info 获取。
func ShareURL(uid int64, shareKey string) string {
	return fmt.Sprintf("https://%d.share.123pan.cn/123pan/%s", uid, shareKey)
}

// TrafficSwitch 分享提取流量包开关：1 全部关闭；2 打开游客免登录提取；3 打开超流量用户提取；4 全部开启。
type TrafficSwitch int

// ShareCreateRequest 是创建分享链接的参数。
type ShareCreateRequest struct {
	// ShareName 分享链接名称。
	ShareName string `json:"shareName"`
	// ShareExpire 有效期天数，枚举：1、7、30、0（0 为永久）。
	ShareExpire int `json:"shareExpire"`
	// FileIDs 分享的文件 ID 列表，最多 100 个。
	FileIDs []int64 `json:"-"`
	// SharePwd 提取码（可选）。
	SharePwd string `json:"sharePwd,omitempty"`
	// TrafficSwitch 分享提取流量包开关（可选）。
	TrafficSwitch TrafficSwitch `json:"trafficSwitch,omitempty"`
	// TrafficLimitSwitch 流量限制开关：1 关闭限制；2 打开限制（可选）。
	TrafficLimitSwitch int `json:"trafficLimitSwitch,omitempty"`
	// TrafficLimit 限制流量，单位字节（可选）。
	TrafficLimit int64 `json:"trafficLimit,omitempty"`
}

// ShareCreateResult 是创建分享链接的返回。
type ShareCreateResult struct {
	ShareID  int64  `json:"shareID"`
	ShareKey string `json:"shareKey"`
}

func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ",")
}

// Create 创建分享链接。
func (s *ShareService) Create(ctx context.Context, req *ShareCreateRequest) (*ShareCreateResult, error) {
	body := map[string]any{
		"shareName":   req.ShareName,
		"shareExpire": req.ShareExpire,
		"fileIDList":  joinIDs(req.FileIDs),
	}
	if req.SharePwd != "" {
		body["sharePwd"] = req.SharePwd
	}
	if req.TrafficSwitch != 0 {
		body["trafficSwitch"] = int(req.TrafficSwitch)
	}
	if req.TrafficLimitSwitch != 0 {
		body["trafficLimitSwitch"] = req.TrafficLimitSwitch
	}
	if req.TrafficLimit != 0 {
		body["trafficLimit"] = req.TrafficLimit
	}
	var out ShareCreateResult
	if err := s.client.post(ctx, "/api/v1/share/create", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ShareInfo 是分享链接信息。
type ShareInfo struct {
	ShareID   int64  `json:"shareId"`
	ShareKey  string `json:"shareKey"`
	ShareName string `json:"shareName"`
	// Expiration 过期时间。
	Expiration string `json:"expiration"`
	// Expired 是否失效：0 未失效；1 失效。
	Expired            int    `json:"expired"`
	SharePwd           string `json:"sharePwd"`
	TrafficSwitch      int    `json:"trafficSwitch"`
	TrafficLimitSwitch int    `json:"trafficLimitSwitch"`
	TrafficLimit       int64  `json:"trafficLimit"`
	// BytesCharge 分享已使用流量（字节）。
	BytesCharge   int64 `json:"bytesCharge"`
	PreviewCount  int64 `json:"previewCount"`
	DownloadCount int64 `json:"downloadCount"`
	SaveCount     int64 `json:"saveCount"`
	// 以下字段仅付费分享返回。
	PayAmount float64 `json:"payAmount"`
	// Amount 分享收益。
	Amount   float64 `json:"amount"`
	OrderCnt int64   `json:"orderCnt"`
}

// ShareListResult 是分享链接列表的返回。
type ShareListResult struct {
	// LastShareID 为 -1 表示最后一页；否则作为下一页的翻页游标。
	LastShareID int64       `json:"lastShareId"`
	ShareList   []ShareInfo `json:"shareList"`
}

func shareListQuery(limit int, lastShareID int64) url.Values {
	if limit <= 0 {
		limit = 100
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	if lastShareID > 0 {
		q.Set("lastShareId", strconv.FormatInt(lastShareID, 10))
	}
	return q
}

// List 获取分享链接列表（lastShareId 游标翻页，返回 -1 表示结束）。
func (s *ShareService) List(ctx context.Context, limit int, lastShareID int64) (*ShareListResult, error) {
	var out ShareListResult
	if err := s.client.get(ctx, "/api/v1/share/list", shareListQuery(limit, lastShareID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ShareUpdateRequest 是修改分享链接的参数（仅支持修改流量相关设置）。
type ShareUpdateRequest struct {
	// ShareIDs 分享链接 ID 列表，最多 100 个。
	ShareIDs []int64 `json:"shareIdList"`
	// TrafficSwitch 分享提取流量包开关（可选）。
	TrafficSwitch TrafficSwitch `json:"trafficSwitch,omitempty"`
	// TrafficLimitSwitch 流量限制开关：1 关闭限制；2 打开限制（可选）。
	TrafficLimitSwitch int `json:"trafficLimitSwitch,omitempty"`
	// TrafficLimit 限制流量，单位字节（可选）。
	TrafficLimit int64 `json:"trafficLimit,omitempty"`
}

// Update 修改分享链接的流量设置。
func (s *ShareService) Update(ctx context.Context, req *ShareUpdateRequest) error {
	return s.client.put(ctx, "/api/v1/share/list/info", req, nil)
}

// PaidShareCreateRequest 是创建付费分享链接的参数。
type PaidShareCreateRequest struct {
	// ShareName 分享链接名称，小于 35 字符且不含特殊字符。
	ShareName string
	// FileIDs 分享的文件 ID 列表，最多 100 个。
	FileIDs []int64
	// PayAmount 付费金额（整数元），1-1000。
	PayAmount int
	// IsReward 是否开启打赏：0 否，1 是。
	IsReward int
	// ResourceDesc 资源描述（可选）。
	ResourceDesc string
	// TrafficSwitch 分享提取流量包开关（可选）。
	TrafficSwitch TrafficSwitch
	// TrafficLimitSwitch 流量限制开关（可选）。
	TrafficLimitSwitch int
	// TrafficLimit 限制流量，单位字节（可选）。
	TrafficLimit int64
}

// CreatePaid 创建付费分享链接。
func (s *ShareService) CreatePaid(ctx context.Context, req *PaidShareCreateRequest) (*ShareCreateResult, error) {
	body := map[string]any{
		"shareName":  req.ShareName,
		"fileIDList": joinIDs(req.FileIDs),
		"payAmount":  req.PayAmount,
	}
	if req.IsReward != 0 {
		body["isReward"] = req.IsReward
	}
	if req.ResourceDesc != "" {
		body["resourceDesc"] = req.ResourceDesc
	}
	if req.TrafficSwitch != 0 {
		body["trafficSwitch"] = int(req.TrafficSwitch)
	}
	if req.TrafficLimitSwitch != 0 {
		body["trafficLimitSwitch"] = req.TrafficLimitSwitch
	}
	if req.TrafficLimit != 0 {
		body["trafficLimit"] = req.TrafficLimit
	}
	var out ShareCreateResult
	if err := s.client.post(ctx, "/api/v1/share/content-payment/create", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPaid 获取付费分享链接列表（lastShareId 游标翻页，返回 -1 表示结束）。
func (s *ShareService) ListPaid(ctx context.Context, limit int, lastShareID int64) (*ShareListResult, error) {
	var out ShareListResult
	if err := s.client.get(ctx, "/api/v1/share/payment/list", shareListQuery(limit, lastShareID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdatePaid 修改付费分享链接的流量设置。
func (s *ShareService) UpdatePaid(ctx context.Context, req *ShareUpdateRequest) error {
	return s.client.put(ctx, "/api/v1/share/list/payment/info", req, nil)
}
