# go-123pan

<div align="center">

**123 云盘开放平台 Go SDK（非官方）**

[![Go Reference](https://pkg.go.dev/badge/github.com/okatu-loli/go-123pan.svg)](https://pkg.go.dev/github.com/okatu-loli/go-123pan)
[![CI](https://github.com/okatu-loli/go-123pan/actions/workflows/ci.yml/badge.svg)](https://github.com/okatu-loli/go-123pan/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/okatu-loli/go-123pan)](https://github.com/okatu-loli/go-123pan/releases)
[![License](https://img.shields.io/github/license/okatu-loli/go-123pan)](LICENSE)

[简体中文](README.md) | [English](README_EN.md)

</div>

基于 [123 云盘开放平台](https://www.123pan.com/developer) 官方文档实现的 Go SDK，覆盖全部开放接口：文件管理、上传（V1/V2/秒传）、分享、离线下载、直链、图床、视频转码与第三方 OAuth 授权。

## ✨ 特性

- **开箱即用** —— `pan123.New(clientID, clientSecret)` 即可调用全部接口
- **Token 自动管理** —— 自动获取、缓存、过期前刷新，并发安全（避免触发同 client_id 最多 3 个 token 的踢下线机制）
- **一行代码上传** —— `UploadFile` 自动完成 MD5 计算、秒传检测、分片并发上传与轮询
- **内置限流与重试** —— 按官方 QPS 表做客户端限流，429 自动指数退避重试
- **完整错误信息** —— 业务错误返回 `*APIError`（含 code、message、x-traceID），支持 `errors.As`
- **全接口覆盖** —— 含 V1 旧版上传、旧版文件列表在内的全部文档接口
- **零重量级依赖** —— 仅依赖标准库与 `golang.org/x/time`

## 📦 安装

```bash
go get github.com/okatu-loli/go-123pan
```

要求 Go 1.22+。

## 🚀 快速开始

前往 [123 云盘开放平台](https://www.123pan.com/developer) 申请 `clientID` 与 `clientSecret`。

```go
package main

import (
	"context"
	"fmt"
	"log"

	pan123 "github.com/okatu-loli/go-123pan"
)

func main() {
	client := pan123.New("your-client-id", "your-client-secret")
	ctx := context.Background()

	// 获取用户信息
	info, err := client.User.Info(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("你好,", info.Nickname)

	// 一行代码上传文件（自动秒传/分片/轮询）
	fileID, err := client.Upload.UploadFromPath(ctx, 0, "/path/to/file.zip")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("上传成功, fileID =", fileID)

	// 列出根目录全部文件（自动翻页）
	files, _ := client.File.ListAll(ctx, 0)
	for _, f := range files {
		fmt.Println(f.Filename)
	}
}
```

更多可运行示例见 [examples/](examples/)：

| 示例 | 说明 |
|---|---|
| [quickstart](examples/quickstart/) | 用户信息 + 文件列表 |
| [upload](examples/upload/) | 上传本地文件 |
| [download](examples/download/) | 下载文件到本地 |
| [share](examples/share/) | 创建分享链接 |
| [offline](examples/offline/) | 离线下载 + 进度轮询 |
| [imagebed](examples/imagebed/) | 图床上传 + 获取外链 |

## 🖥️ 命令行工具

SDK 附带 `pan123` CLI，覆盖常用操作：

```bash
go install github.com/okatu-loli/go-123pan/cmd/pan123@latest

# 保存凭证（也可用环境变量 PAN123_CLIENT_ID / PAN123_CLIENT_SECRET）
pan123 login -client-id <ID> -client-secret <SECRET>

pan123 whoami                    # 用户信息与空间用量
pan123 ls                        # 列出根目录（可跟目录ID）
pan123 mkdir -parent 0 新目录     # 创建目录
pan123 upload -parent 0 a.zip    # 上传（自动秒传/分片）
pan123 download -o a.zip 123456  # 下载
pan123 rm 123456 123457          # 删除至回收站
pan123 mv -to 789 123456         # 移动
pan123 rename 123456 新名称       # 重命名
pan123 share -pwd 1234 123456    # 创建分享链接
pan123 offline https://…/f.mp4   # 离线下载并等待完成
pan123 link 123456               # 获取直链
```

access_token 会缓存在系统配置目录（macOS 为 `~/Library/Application Support/pan123/`，Linux 为 `~/.config/pan123/`）并跨进程复用，避免触发官方同 client_id 最多 3 个 token 的限制。

## 📚 API 覆盖

完整的「方法 ↔ 官方接口 ↔ 参数说明」对照表见 **[docs/API.md](docs/API.md)**，逐方法的类型签名与注释见 [pkg.go.dev](https://pkg.go.dev/github.com/okatu-loli/go-123pan)。

| 服务 | 说明 | 主要方法 |
|---|---|---|
| `client.File` | 文件管理 | `Mkdir` `Rename` `BatchRename` `Trash` `Recover` `RecoverTo` `Copy` `AsyncCopy` `CopyProcess` `Move` `Detail` `Infos` `List` `ListAll` `ListV1` `SafeboxID` `DownloadInfo` `DownloadTo` |
| `client.Upload` | 文件上传 | `UploadFile` `UploadFromPath`（高级封装）；V2：`Create` `UploadSlice` `Complete` `Domains` `SingleCreate` `Sha1Reuse`；V1：`CreateV1` `GetUploadURLV1` `PutSliceV1` `ListUploadPartsV1` `CompleteV1` `AsyncResultV1` |
| `client.Share` | 分享管理 | `Create` `List` `Update` `CreatePaid` `ListPaid` `UpdatePaid` |
| `client.Offline` | 离线下载 | `Download` `Process` |
| `client.User` | 用户信息 | `Info` |
| `client.Link` | 直链 | `Enable` `Disable` `URL` `RefreshCache` `TrafficLog` `OfflineLogs` `SwitchIPBlacklist` `UpdateIPBlacklist` `IPBlacklist` |
| `client.Oss` | 图床 | `UploadFile` `UploadFromPath`（高级封装）`Mkdir` `CreateFile` `GetUploadURL` `UploadComplete` `UploadAsyncResult` `CopyFromDisk` `CopyProcess` `CopyFailList` `Move` `Delete` `Detail` `List` `OfflineDownload` `OfflineProcess` |
| `client.Transcode` | 视频转码 | `FolderInfo` `CloudVideoFiles` `SpaceFiles` `UploadFromCloudDisk` `Resolutions` `Transcode` `Records` `Results` `List` `Delete` `DownloadOriginal` `DownloadM3U8` `DownloadTS` `DownloadAll` |
| `client.OAuth` | 三方授权 | `AuthURL` `TokenByCode` `RefreshToken` |

## ⚙️ 配置选项

```go
client := pan123.New(id, secret,
	pan123.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}), // 自定义 HTTP 客户端
	pan123.WithMaxRetries(5),      // 429 限流重试次数（默认 3，0 关闭）
	pan123.WithoutRateLimit(),     // 关闭内置客户端限流
	pan123.WithBaseURL("..."),     // 覆盖接口域名（测试/代理）
)

// 三方挂载应用：使用 OAuth 授权得到的 token
client := pan123.NewWithToken(accessToken)
```

## 🧯 错误处理

```go
_, err := client.File.DownloadInfo(ctx, fileID)
var apiErr *pan123.APIError
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.Code, apiErr.Message, apiErr.TraceID)
}
if pan123.IsRateLimited(err) { /* 请求过于频繁 */ }
if pan123.IsTokenExpired(err) { /* token 失效 */ }
```

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

提交信息请遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/)（`feat:`、`fix:`、`docs:` 等）——合并到 `main` 后 CI 会依据提交类型自动发布语义化版本（`fix` → patch、`feat` → minor、`BREAKING CHANGE` → major）。

### 贡献者

<!-- readme: contributors -start -->
<table>
	<tbody>
		<tr>
            <td align="center">
                <a href="https://github.com/okatu-loli">
                    <img src="https://avatars.githubusercontent.com/u/53247097?v=4" width="100;" alt="okatu-loli"/>
                    <br />
                    <sub><b>千石</b></sub>
                </a>
            </td>
		</tr>
	<tbody>
</table>
<!-- readme: contributors -end -->

## 📄 许可证

[MIT](LICENSE)

> 本项目为社区实现，与 123 云盘官方无隶属关系。接口以[官方文档](https://123yunpan.yuque.com/org-wiki-123yunpan-muaork/cr6ced)为准。
