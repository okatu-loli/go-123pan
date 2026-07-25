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

// Enable 对文件夹启用直链空间，返回该文件夹的名称。
//
// 接口: POST /api/v1/direct-link/enable
//
// 参数说明：
//   - folderID：要启用直链空间的文件夹（目录）ID。
//
// 注意事项：需要开通开发者权益；启用后该目录下的文件才能通过 URL 获取直链。
func (s *LinkService) Enable(ctx context.Context, folderID int64) (string, error) {
	var out struct {
		Filename string `json:"filename"`
	}
	if err := s.client.post(ctx, "/api/v1/direct-link/enable", map[string]int64{"fileID": folderID}, &out); err != nil {
		return "", err
	}
	return out.Filename, nil
}

// Disable 对文件夹禁用直链空间，返回该文件夹的名称。
//
// 接口: POST /api/v1/direct-link/disable
//
// 参数说明：
//   - folderID：要禁用直链空间的文件夹（目录）ID。
//
// 注意事项：需要开通开发者权益；禁用后该目录下已生成的直链将失效。
func (s *LinkService) Disable(ctx context.Context, folderID int64) (string, error) {
	var out struct {
		Filename string `json:"filename"`
	}
	if err := s.client.post(ctx, "/api/v1/direct-link/disable", map[string]int64{"fileID": folderID}, &out); err != nil {
		return "", err
	}
	return out.Filename, nil
}

// URL 获取文件的直链链接。
//
// 接口: GET /api/v1/direct-link/url
//
// 参数说明：
//   - fileID：文件 ID。前提是该文件所在目录已通过 Enable 启用直链空间，
//     否则接口会返回错误。
//
// 注意事项：需要开通开发者权益；直链访问会消耗直链流量
// （剩余流量见 UserInfo.DirectTraffic）。
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
//
// 接口: POST /api/v1/direct-link/cache/refresh
//
// 注意事项：需要开通开发者权益；为全量刷新，不支持按文件粒度刷新，
// 文件内容更新后调用可使 CDN 尽快回源取新内容。
func (s *LinkService) RefreshCache(ctx context.Context) error {
	return s.client.post(ctx, "/api/v1/direct-link/cache/refresh", struct{}{}, nil)
}

// TrafficLogEntry 是直链流量日志条目。
type TrafficLogEntry struct {
	// UniqueID 记录唯一 ID。
	UniqueID string `json:"uniqueID"`
	// FileName 文件名称。
	FileName string `json:"fileName"`
	// FileSize 文件大小（字节）。
	FileSize int64 `json:"fileSize"`
	// FilePath 文件路径。
	FilePath string `json:"filePath"`
	// DirectLinkURL 直链链接。
	DirectLinkURL string `json:"directLinkURL"`
	// FileSource 文件来源：1 全部文件，2 图床。
	FileSource int `json:"fileSource"`
	// TotalTraffic 消耗流量（字节）。
	TotalTraffic int64 `json:"totalTraffic"`
}

// TrafficLogResult 是直链流量日志的返回。
type TrafficLogResult struct {
	// Total 记录总数。
	Total int64 `json:"total"`
	// List 当前页的日志条目。
	List []TrafficLogEntry `json:"list"`
}

// TrafficLog 分页查询直链流量消耗明细日志。
//
// 接口: GET /api/v1/direct-link/log
//
// 参数说明：
//   - pageNum：页码，从 1 开始。
//   - pageSize：每页数量。
//   - startTime/endTime：查询时间范围（闭区间），
//     格式为 "2025-01-01 00:00:00"。
//
// 注意事项：需要开通开发者权益；流量日志仅可查询最近 3 天的数据，
// 更早的明细请通过 OfflineLogs 下载离线日志文件。
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
	// ID 日志文件 ID（文档标 string，实际可能返回数字，用 any 兼容）。
	ID any `json:"id"`
	// FileName 日志文件名称。
	FileName string `json:"fileName"`
	// FileSize 日志文件大小（字节）。
	FileSize int64 `json:"fileSize"`
	// LogTimeRange 该日志文件覆盖的时间范围。
	LogTimeRange string `json:"logTimeRange"`
	// DownloadURL 日志文件（.gz）下载地址。
	DownloadURL string `json:"downloadURL"`
}

// OfflineLogResult 是直链离线日志的返回。
type OfflineLogResult struct {
	// Total 日志文件总数。
	Total int64 `json:"total"`
	// List 当前页的日志文件条目。
	List []OfflineLogEntry `json:"list"`
}

// OfflineLogs 分页查询直链离线日志文件列表（按小时打包的 .gz 文件）。
//
// 接口: GET /api/v1/direct-link/offline/logs
//
// 参数说明：
//   - pageNum：页码，从 1 开始。
//   - pageSize：每页数量。
//   - startHour/endHour：查询时间范围，精确到小时，格式为 "2025010115"
//     （即 yyyyMMddHH）。
//
// 注意事项：需要开通开发者权益；离线日志仅可查询最近 30 天的数据，
// 日志内容需通过条目中的 DownloadURL 自行下载解压。
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

// IPBlacklistStatus IP 黑名单开关状态。
// 枚举值：1 启用；2 禁用。
type IPBlacklistStatus int

const (
	// IPBlacklistEnabled 黑名单启用。
	IPBlacklistEnabled IPBlacklistStatus = 1
	// IPBlacklistDisabled 黑名单禁用。
	IPBlacklistDisabled IPBlacklistStatus = 2
)

// SwitchIPBlacklist 开启或关闭直链 IP 黑名单，返回操作是否完成。
//
// 接口: POST /api/v1/developer/config/forbide-ip/switch
//
// 参数说明：
//   - status：目标开关状态，IPBlacklistEnabled（1 启用）或
//     IPBlacklistDisabled（2 禁用）。
//
// 注意事项：需要开通开发者权益；黑名单条目通过 UpdateIPBlacklist 维护，
// 仅当状态为启用时黑名单才会生效。
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

// UpdateIPBlacklist 全量覆盖更新直链 IP 黑名单列表。
//
// 接口: POST /api/v1/developer/config/forbide-ip/update
//
// 参数说明：
//   - ips：黑名单 IP 列表，仅支持 IPv4 地址，最多 2000 个。
//
// 注意事项：需要开通开发者权益；本接口为全量覆盖更新，
// 传入的列表会替换现有全部黑名单条目（追加/删除需先经 IPBlacklist
// 取回现有列表合并后再提交）。
func (s *LinkService) UpdateIPBlacklist(ctx context.Context, ips []string) error {
	return s.client.post(ctx, "/api/v1/developer/config/forbide-ip/update", map[string][]string{"IpList": ips}, nil)
}

// IPBlacklist 获取直链 IP 黑名单列表及当前开关状态。
//
// 接口: GET /api/v1/developer/config/forbide-ip/list
//
// 返回说明：
//   - ips：当前黑名单中的 IPv4 地址列表（上限 2000 个）。
//   - status：黑名单开关状态，1 启用；2 禁用。
//
// 注意事项：需要开通开发者权益。
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
