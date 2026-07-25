package pan123

import (
	"context"
	"encoding/json"
)

// UserService 提供用户信息接口。
type UserService struct {
	client *Client
}

// VipInfo 是会员信息。
type VipInfo struct {
	// VipLevel 会员等级：1 VIP；2 SVIP；3 长期VIP。
	VipLevel int `json:"vipLevel"`
	// VipLabel 会员等级名称，如"月卡"。
	VipLabel string `json:"vipLabel"`
	// StartTime 会员开始时间。
	StartTime string `json:"startTime"`
	// EndTime 会员结束时间。
	EndTime string `json:"endTime"`
}

// DeveloperInfo 是开发者权益信息（直链等接口多数需要开通该权益）。
type DeveloperInfo struct {
	// StartTime 权益开始时间。
	StartTime string `json:"startTime"`
	// EndTime 权益结束时间。
	EndTime string `json:"endTime"`
}

// UserInfo 是用户信息。
type UserInfo struct {
	// UID 用户账号 ID，也用于拼接分享链接（见 ShareURL）。
	UID int64 `json:"uid"`
	// Nickname 昵称。
	Nickname string `json:"nickname"`
	// HeadImage 头像地址。
	HeadImage string `json:"headImage"`
	// Passport 手机号码。
	Passport string `json:"passport"`
	// Mail 邮箱。
	Mail string `json:"mail"`
	// SpaceUsed 已用空间（字节）。
	SpaceUsed int64 `json:"spaceUsed"`
	// SpacePermanent 永久空间（字节）。
	SpacePermanent int64 `json:"spacePermanent"`
	// SpaceTemp 临时空间（字节）。
	SpaceTemp int64 `json:"spaceTemp"`
	// SpaceTempExpr 临时空间到期日（文档标 string，实际可能返回数字，用 RawMessage 兼容）。
	SpaceTempExpr json.RawMessage `json:"spaceTempExpr"`
	// Vip 是否为会员。
	Vip bool `json:"vip"`
	// DirectTraffic 剩余直链流量（字节）。
	DirectTraffic int64 `json:"directTraffic"`
	// IsHideUID 直链链接是否隐藏 UID。
	IsHideUID bool `json:"isHideUID"`
	// HTTPSCount https 数量。
	HTTPSCount int `json:"httpsCount"`
	// VipInfoList 会员信息，非会员为 nil。
	VipInfoList []VipInfo `json:"vipInfo"`
	// DeveloperInfo 开发者权益信息。
	DeveloperInfo *DeveloperInfo `json:"developerInfo"`
}

// Info 获取当前用户信息，包括账号、空间用量、会员及开发者权益等。
//
// 接口: GET /api/v1/user/info (QPS 10)
//
// 注意事项：返回的 UID 用于拼接分享页面链接
// https://{uid}.share.123pan.cn/123pan/{shareKey}（见 ShareURL）；
// 该接口有 QPS 10 限制，建议缓存结果，避免高频调用。
func (s *UserService) Info(ctx context.Context) (*UserInfo, error) {
	var out UserInfo
	if err := s.client.get(ctx, "/api/v1/user/info", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
