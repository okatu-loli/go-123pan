package pan123

import (
	"context"
	"net/url"
	"strconv"
)

// LinkService 提供直链相关接口：直链空间启停、直链获取、缓存刷新、
// 流量/离线日志查询、IP 黑名单配置。多数接口需要开通开发者权益。
type LinkService struct {
	client *Client
}

// Enable 对文件夹启用直链空间，返回文件夹名称。
func (s *LinkService) Enable(ctx context.Context, folderID int64) (string, error) {
	var out struct {
		Filename string `json:"filename"`
	}
	if err := s.client.post(ctx, "/api/v1/direct-link/enable", map[string]int64{"fileID": folderID}, &out); err != nil {
		return "", err
	}
	return out.Filename, nil
}

// Disable 对文件夹禁用直链空间，返回文件夹名称。
func (s *LinkService) Disable(ctx context.Context, folderID int64) (string, error) {
	var out struct {
		Filename string `json:"filename"`
	}
	if err := s.client.post(ctx, "/api/v1/direct-link/disable", map[string]int64{"fileID": folderID}, &out); err != nil {
		return "", err
	}
	return out.Filename, nil
}

// URL 获取文件的直链链接。文件必须位于已启用直链空间的目录下。
func (s *LinkService) URL(ctx context.Context, fileID int64) (string, error) {
	q := url.Values{}
	q.Set("fileID", strconv.FormatInt(fileID, 10))
	var out struct {
		URL string `json:"url"`
	}
	if err := s.client.get(ctx, "/api/v1/direct-link/url", q, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// RefreshCache 全量刷新直链 CDN 缓存。
func (s *LinkService) RefreshCache(ctx context.Context) error {
	return s.client.post(ctx, "/api/v1/direct-link/cache/refresh", struct{}{}, nil)
}

// TrafficLogEntry 是直链流量日志条目。
type TrafficLogEntry struct {
	UniqueID      string `json:"uniqueID"`
	FileName      string `json:"fileName"`
	FileSize      int64  `json:"fileSize"`
	FilePath      string `json:"filePath"`
	DirectLinkURL string `json:"directLinkURL"`
	// FileSource 文件来源：1 全部文件，2 图床。
	FileSource int `json:"fileSource"`
	// TotalTraffic 消耗流量（字节）。
	TotalTraffic int64 `json:"totalTraffic"`
}

// TrafficLogResult 是直链流量日志的返回。
type TrafficLogResult struct {
	Total int64             `json:"total"`
	List  []TrafficLogEntry `json:"list"`
}

// TrafficLog 查询直链流量日志（仅近 3 天）。
// startTime/endTime 格式："2025-01-01 00:00:00"；pageNum 从 1 开始。
func (s *LinkService) TrafficLog(ctx context.Context, pageNum, pageSize int, startTime, endTime string) (*TrafficLogResult, error) {
	q := url.Values{}
	q.Set("pageNum", strconv.Itoa(pageNum))
	q.Set("pageSize", strconv.Itoa(pageSize))
	q.Set("startTime", startTime)
	q.Set("endTime", endTime)
	var out TrafficLogResult
	if err := s.client.get(ctx, "/api/v1/direct-link/log", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// OfflineLogEntry 是直链离线日志文件条目（按小时打包的 .gz 日志）。
type OfflineLogEntry struct {
	// ID 文档标 string，实际可能返回数字，用 Number 兼容。
	ID           any    `json:"id"`
	FileName     string `json:"fileName"`
	FileSize     int64  `json:"fileSize"`
	LogTimeRange string `json:"logTimeRange"`
	DownloadURL  string `json:"downloadURL"`
}

// OfflineLogResult 是直链离线日志的返回。
type OfflineLogResult struct {
	Total int64             `json:"total"`
	List  []OfflineLogEntry `json:"list"`
}

// OfflineLogs 查询直链离线日志（仅近 30 天）。
// startHour/endHour 格式精确到小时："2025010115"；pageNum 从 1 开始。
func (s *LinkService) OfflineLogs(ctx context.Context, pageNum, pageSize int, startHour, endHour string) (*OfflineLogResult, error) {
	q := url.Values{}
	q.Set("startHour", startHour)
	q.Set("endHour", endHour)
	q.Set("pageNum", strconv.Itoa(pageNum))
	q.Set("pageSize", strconv.Itoa(pageSize))
	var out OfflineLogResult
	if err := s.client.get(ctx, "/api/v1/direct-link/offline/logs", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// IPBlacklistStatus IP 黑名单状态：1 启用，2 禁用。
type IPBlacklistStatus int

const (
	IPBlacklistEnabled  IPBlacklistStatus = 1
	IPBlacklistDisabled IPBlacklistStatus = 2
)

// SwitchIPBlacklist 开启或关闭 IP 黑名单，返回操作是否完成。
func (s *LinkService) SwitchIPBlacklist(ctx context.Context, status IPBlacklistStatus) (bool, error) {
	var out struct {
		Done bool `json:"Done"`
	}
	err := s.client.post(ctx, "/api/v1/developer/config/forbide-ip/switch", map[string]int{"Status": int(status)}, &out)
	if err != nil {
		return false, err
	}
	return out.Done, nil
}

// UpdateIPBlacklist 全量覆盖更新 IP 黑名单列表，最多 2000 个 IPv4 地址。
func (s *LinkService) UpdateIPBlacklist(ctx context.Context, ips []string) error {
	return s.client.post(ctx, "/api/v1/developer/config/forbide-ip/update", map[string][]string{"IpList": ips}, nil)
}

// IPBlacklist 获取 IP 黑名单列表及开关状态。
func (s *LinkService) IPBlacklist(ctx context.Context) (ips []string, status IPBlacklistStatus, err error) {
	var out struct {
		IPList []string `json:"ipList"`
		Status int      `json:"status"`
	}
	if err := s.client.get(ctx, "/api/v1/developer/config/forbide-ip/list", nil, &out); err != nil {
		return nil, 0, err
	}
	return out.IPList, IPBlacklistStatus(out.Status), nil
}
