# go-123pan API 参考

本文档按服务分组列出 SDK 全部方法与官方接口的对应关系、参数说明和使用注意事项。完整的类型签名与字段说明见 [pkg.go.dev](https://pkg.go.dev/github.com/okatu-loli/go-123pan)，官方接口文档见 [123 云盘开放平台](https://123yunpan.yuque.com/org-wiki-123yunpan-muaork/cr6ced)。

## 目录

- [客户端与鉴权](#客户端与鉴权)
- [File — 文件管理](#file--文件管理)
- [Upload — 文件上传](#upload--文件上传)
- [Share — 分享管理](#share--分享管理)
- [Offline — 离线下载](#offline--离线下载)
- [User — 用户信息](#user--用户信息)
- [Link — 直链](#link--直链)
- [Oss — 图床](#oss--图床)
- [Transcode — 视频转码](#transcode--视频转码)
- [OAuth — 第三方授权](#oauth--第三方授权)
- [错误处理](#错误处理)
- [限流与重试](#限流与重试)

## 客户端与鉴权

| 方法 | 功能 | 说明 |
|---|---|---|
| `New(clientID, clientSecret, opts...)` | 创建客户端 | 开发者模式。SDK 自动获取/缓存/刷新 access_token（有效期 30 天，同 client_id 同时最多 3 个 token 在线） |
| `NewWithToken(accessToken, opts...)` | 创建客户端 | 三方挂载应用 OAuth 场景，直接使用外部 token，不自动刷新 |
| `(*Client).RefreshToken(ctx)` | 强制换取新 token | 接口: `POST /api/v1/access_token` (QPS 10)。一般无需手动调用 |
| `(*Client).SetToken(token, expiredAt)` | 手动设置 token | 配合 `TokenInfo` 可跨进程持久化复用 token |
| `(*Client).Token()` / `TokenInfo()` | 读取当前 token | `TokenInfo` 额外返回过期时间 |

**Options**：`WithHTTPClient`（自定义 HTTP 客户端）、`WithBaseURL`（覆盖域名）、`WithUserAgent`、`WithMaxRetries(n)`（429 重试次数，默认 3）、`WithoutRateLimit()`（关闭内置限流）。

所有 API 方法第一个参数均为 `context.Context`，支持超时与取消。

## File — 文件管理

| 方法 | 接口 | 功能 | 关键参数与限制 |
|---|---|---|---|
| `Mkdir(ctx, parentID, name)` | `POST /upload/v1/file/mkdir` (QPS 20) | 创建目录 | parentID 根目录为 0；同级不能重名 |
| `Rename(ctx, fileID, newName)` | `PUT /api/v1/file/name` | 重命名单个文件 | 文件名 <255 字符，不含 `"\/:*?\|><` |
| `BatchRename(ctx, renames)` | `POST /api/v1/file/rename` | 批量重命名 | map[文件ID]新名，一次最多 30 个 |
| `Trash(ctx, fileIDs)` | `POST /api/v1/file/trash` | 删除至回收站 | 一次最多 100 个 |
| `Recover(ctx, fileIDs)` | `POST /api/v1/file/recover` | 回收站恢复至原位置 | 一次最多 100 个；返回父目录已不存在的异常 ID |
| `RecoverTo(ctx, fileIDs, parentFileID)` | `POST /api/v1/file/recover/by_path` | 恢复到指定目录 | 一次最多 100 个 |
| `Copy(ctx, fileID, targetDirID)` | `POST /api/v1/file/copy` | 复制单个文件（同步） | 立即返回新文件 ID |
| `AsyncCopy(ctx, fileIDs, targetDirID)` | `POST /api/v1/file/async/copy` | 批量复制（异步） | 单级最多 3000 个；返回任务 ID |
| `CopyProcess(ctx, taskID)` | `GET /api/v1/file/async/copy/process` | 批量复制进度 | 状态: 0 待处理 1 进行中 2 完成 3 失败 |
| `Move(ctx, fileIDs, toParentFileID)` | `POST /api/v1/file/move` (QPS 20) | 批量移动 | 单级最多 100 个；根目录为 0 |
| `Detail(ctx, fileID)` | `GET /api/v1/file/detail` | 单文件详情 | 查文件夹时 size 为累计大小 |
| `Infos(ctx, fileIDs)` | `POST /api/v1/file/infos` | 批量文件详情 | — |
| `List(ctx, req)` | `GET /api/v2/file/list` (QPS 15) | 文件列表（推荐） | lastFileId 游标翻页，-1 为最后一页；limit ≤100；**结果含回收站文件**，按 Trashed 过滤；searchData 非空时全局搜索 |
| `ListAll(ctx, parentFileID)` | 同上 | 自动翻页取全部 | 已过滤回收站文件 |
| `ListV1(ctx, req)` | `GET /api/v1/file/list` (QPS 10) | 文件列表（旧版） | page 页码翻页；支持 orderBy/orderDirection 排序与 trashed 过滤 |
| `SafeboxID(ctx, password)` | `GET /api/v1/safebox/id` | 解锁保险箱 | 返回保险箱目录 ID，可作 parentFileID |
| `DownloadInfo(ctx, fileID)` | `GET /api/v1/file/download_info` | 获取下载直链 | 临时 CDN 直链；免费账号每日 1GB（超限 code 5113）；文件不存在 code 5066 |
| `DownloadTo(ctx, fileID, w)` | 同上 | 下载写入 io.Writer | 返回写入字节数 |

## Upload — 文件上传

**高级封装（推荐）**：

| 方法 | 功能 | 说明 |
|---|---|---|
| `UploadFromPath(ctx, parentFileID, path)` | 上传本地文件 | 自动 MD5 → 秒传检测 → 分片 3 并发上传 → 轮询完成，返回文件 ID |
| `UploadFile(ctx, parentFileID, filename, r, size)` | 上传 io.ReaderAt | 同上；r 需支持随机读（*os.File、*bytes.Reader 等） |

**V2 分片上传（单文件 ≤10GB）**：

| 方法 | 接口 | 功能 |
|---|---|---|
| `Create(ctx, req)` | `POST /upload/v2/file/create` | 预上传；reuse=true 即秒传成功；否则按返回 sliceSize 切分、用返回 servers 上传 |
| `UploadSlice(ctx, server, preuploadID, sliceNo, sliceMD5, r)` | `POST {server}/upload/v2/file/slice` | 上传分片（multipart）；sliceNo 从 1 开始 |
| `Complete(ctx, preuploadID)` | `POST /upload/v2/file/upload_complete` | 上传完毕；completed=false 时间隔 1 秒轮询本接口 |

**单步上传（单文件 ≤1GB）**：

| 方法 | 接口 | 功能 |
|---|---|---|
| `Domains(ctx)` | `GET /upload/v2/file/domain` | 获取上传域名列表 |
| `SingleCreate(ctx, server, req, file)` | `POST {server}/upload/v2/file/single/create` | 一次请求完成小文件上传（multipart） |

**秒传**：

| 方法 | 接口 | 功能 |
|---|---|---|
| `Sha1Reuse(ctx, parentFileID, filename, sha1, size, duplicate)` | `POST /upload/v2/file/sha1_reuse` | 按 SHA1 尝试秒传（其余接口均用 MD5）；reuse=false 需走正常流程 |

**V1 旧版流程**（官方已推荐 V2，SDK 保留完整实现）：

| 方法 | 接口 | 功能 |
|---|---|---|
| `CreateV1(ctx, req)` | `POST /upload/v1/file/create` | 预上传 |
| `GetUploadURLV1(ctx, preuploadID, sliceNo)` | `POST /upload/v1/file/get_upload_url` | 逐片换取预签名 URL |
| `PutSliceV1(ctx, presignedURL, r, size)` | `PUT {presignedURL}` | 上传分片二进制；**不携带鉴权头**（官方要求） |
| `ListUploadPartsV1(ctx, preuploadID)` | `POST /upload/v1/file/list_upload_parts` | 列举已传分片做 MD5 比对（文件小于分片大小时返回空） |
| `CompleteV1(ctx, preuploadID)` | `POST /upload/v1/file/upload_complete` | 上传完毕；async=true 转轮询 |
| `AsyncResultV1(ctx, preuploadID)` | `POST /upload/v1/file/upload_async_result` | 轮询结果，间隔 ≥1 秒 |

通用参数说明：`parentFileID` 根目录为 0；`duplicate` 重名策略：1 保留两者（自动加后缀）、2 覆盖；`containDir=true` 时 filename 可携带路径（如 `/a/b/c.mp4`）自动建目录。

## Share — 分享管理

| 方法 | 接口 | 功能 | 关键参数与限制 |
|---|---|---|---|
| `Create(ctx, req)` | `POST /api/v1/share/create` | 创建分享链接 | 有效期枚举 1/7/30/0（0 永久）；文件最多 100 个；可设提取码/流量开关 |
| `List(ctx, limit, lastShareID)` | `GET /api/v1/share/list` | 分享列表 | lastShareId 游标翻页，-1 结束；limit ≤100 |
| `Update(ctx, req)` | `PUT /api/v1/share/list/info` | 修改分享 | **仅能改流量三项设置**；ID 最多 100 个 |
| `CreatePaid(ctx, req)` | `POST /api/v1/share/content-payment/create` | 创建付费分享 | 金额整数 1-1000 元；名称 <35 字符 |
| `ListPaid(ctx, limit, lastShareID)` | `GET /api/v1/share/payment/list` | 付费分享列表 | 同 List |
| `UpdatePaid(ctx, req)` | `PUT /api/v1/share/list/payment/info` | 修改付费分享 | 同 Update |
| `ShareURL(uid, shareKey)` | —（本地拼接） | 生成分享页链接 | `https://{uid}.share.123pan.cn/123pan/{shareKey}`，uid 取自 `User.Info` |

## Offline — 离线下载

| 方法 | 接口 | 功能 | 说明 |
|---|---|---|---|
| `Download(ctx, req)` | `POST /api/v1/offline/download` | 创建离线下载任务 | 仅支持 http/https；不支持下载到根目录（默认"来自:离线下载"目录）；可设回调地址 |
| `Process(ctx, taskID)` | `GET /api/v1/offline/download/process` | 查询进度 | 状态: 0 进行中 1 失败 2 成功 3 重试中（3 仍未终结）；失败时进度归零 |

## User — 用户信息

| 方法 | 接口 | 功能 | 说明 |
|---|---|---|---|
| `Info(ctx)` | `GET /api/v1/user/info` (QPS 10) | 获取用户信息 | 含 UID（拼分享链接用）、昵称、空间用量、VIP、直链流量、开发者权益等；非会员 VipInfoList 为 nil |

## Link — 直链

多数接口需要开通**开发者权益**。前提：文件所在目录已启用直链空间。

| 方法 | 接口 | 功能 | 说明 |
|---|---|---|---|
| `Enable(ctx, folderID)` | `POST /api/v1/direct-link/enable` | 启用直链空间 | 作用于**文件夹** |
| `Disable(ctx, folderID)` | `POST /api/v1/direct-link/disable` | 禁用直链空间 | 同上 |
| `URL(ctx, fileID)` | `GET /api/v1/direct-link/url` | 获取文件直链 | 文件须在已启用直链的目录下 |
| `RefreshCache(ctx)` | `POST /api/v1/direct-link/cache/refresh` | 刷新直链 CDN 缓存 | 全量刷新，无粒度参数 |
| `TrafficLog(ctx, pageNum, pageSize, start, end)` | `GET /api/v1/direct-link/log` | 直链流量日志 | **仅近 3 天**；时间格式 `2025-01-01 00:00:00` |
| `OfflineLogs(ctx, pageNum, pageSize, startHour, endHour)` | `GET /api/v1/direct-link/offline/logs` | 直链离线日志 | **仅近 30 天**；按小时打包 .gz；时间格式 `2025010115` |
| `SwitchIPBlacklist(ctx, status)` | `POST /api/v1/developer/config/forbide-ip/switch` | 开关 IP 黑名单 | 1 启用 2 禁用 |
| `UpdateIPBlacklist(ctx, ips)` | `POST /api/v1/developer/config/forbide-ip/update` | 更新黑名单 | **全量覆盖**；最多 2000 个 IPv4 |
| `IPBlacklist(ctx)` | `GET /api/v1/developer/config/forbide-ip/list` | 查询黑名单 | 返回列表与开关状态 |

## Oss — 图床

**注意**：图床的文件/目录 ID 均为**字符串**（云盘是 int64），根目录用空字符串 `""`。支持格式 png/gif/jpeg/tiff/webp/jpg/tif/svg/bmp，单图上限 100M；图床目录下相同 etag+size 的图片会被覆盖。

| 方法 | 接口 | 功能 | 说明 |
|---|---|---|---|
| `UploadFromPath(ctx, parentFileID, path)` | —（组合流程） | 一行上传图片 | 自动 MD5/秒传/分片/轮询 |
| `UploadFile(ctx, parentFileID, filename, r, size)` | —（组合流程） | 上传 io.ReaderAt | 同上 |
| `Mkdir(ctx, parentID, name)` | `POST /upload/v1/oss/file/mkdir` | 创建图床目录 | — |
| `CreateFile(ctx, ...)` | `POST /upload/v1/oss/file/create` | 预上传 | reuse=true 秒传 |
| `GetUploadURL(ctx, preuploadID, sliceNo)` | `POST /upload/v1/oss/file/get_upload_url` | 换取分片预签名 URL | 配合 `Upload.PutSliceV1` PUT 上传 |
| `UploadComplete(ctx, preuploadID)` | `POST /upload/v1/oss/file/upload_complete` | 上传完毕 | async=true 转轮询 |
| `UploadAsyncResult(ctx, preuploadID)` | `POST /upload/v1/oss/file/upload_async_result` | 轮询上传结果 | 间隔 ≥1 秒 |
| `CopyFromDisk(ctx, fileIDs, toParentFileID)` | `POST /api/v1/oss/source/copy` | 云盘图片复制到图床 | 并发任务≤3、单任务≤1000 张、fileIDs≤100 |
| `CopyProcess(ctx, taskID)` | `GET /api/v1/oss/source/copy/process` | 复制任务状态 | 1 进行中 2 结束 3 失败 4 等待 |
| `CopyFailList(ctx, taskID, page, limit)` | `GET /api/v1/oss/source/copy/fail` | 复制失败列表 | page 页码翻页，limit ≤100 |
| `Move(ctx, fileIDs, toParentFileID)` | `POST /api/v1/oss/file/move` | 移动图片 | 单级最多 100；目标不能为根目录 |
| `Delete(ctx, fileIDs)` | `POST /api/v1/oss/file/delete` | 删除图片 | 一次最多 100 |
| `Detail(ctx, fileID)` | `GET /api/v1/oss/file/detail` | 图片详情 | 含 DownloadURL 外链、UserSelfURL 自定义域名链接 |
| `List(ctx, req)` | `POST /api/v1/oss/file/list` | 图片列表 | lastFileId **字符串**游标，"-1" 结束；可按创建时间戳筛选 |
| `OfflineDownload(ctx, req)` | `POST /api/v1/oss/offline/download` | 离线迁移（URL 拉图） | 仅 http/https |
| `OfflineProcess(ctx, taskID)` | `GET /api/v1/oss/offline/download/process` | 迁移进度 | 状态同云盘离线下载 |

## Transcode — 视频转码

典型流程：`FolderInfo` 获取转码空间目录 → 上传视频（本地用 `Upload.UploadFile` 传到该目录，或 `UploadFromCloudDisk` 从云盘导入）→ `Resolutions` 查询可转码分辨率（轮询）→ `Transcode` 发起转码 → `Records`/`Results` 查询进度与产物 → 下载。

| 方法 | 接口 | 功能 | 说明 |
|---|---|---|---|
| `FolderInfo(ctx)` | `POST /api/v1/transcode/folder/info` (QPS 20) | 转码空间目录 ID | 本地上传视频时作 parentFileID |
| `CloudVideoFiles(ctx, req)` | `GET /api/v2/file/list` (category=2) | 云盘中的视频文件 | 用于挑选导入转码空间的视频 |
| `SpaceFiles(ctx, req)` | `GET /api/v2/file/list` (businessType=2) | 转码空间文件列表 | — |
| `UploadFromCloudDisk(ctx, fileIDs)` | `POST /api/v1/transcode/upload/from_cloud_disk` (QPS 1) | 云盘视频导入转码空间 | 一次最多 100 个 |
| `Resolutions(ctx, fileID)` | `POST /api/v1/transcode/video/resolutions` (QPS 1) | 可转码分辨率 | IsGetResolution=true 时未就绪，**10 秒**间隔轮询 |
| `Transcode(ctx, req)` | `POST /api/v1/transcode/video` (QPS 3) | 发起转码 | 分辨率 **P 大写**（如 "2160P,1080P"）；已转码的分辨率勿重复提交 |
| `Records(ctx, fileID)` | `POST /api/v1/transcode/video/record` (QPS 20) | 转码记录 | status 255 成功时 Link 为 m3u8 地址 |
| `Results(ctx, fileID)` | `POST /api/v1/transcode/video/result` (QPS 20) | 转码结果与产物 | 产物含 m3u8/ts 文件；FileSize 为带单位字符串 |
| `List(ctx, fileID)` | `GET /api/v1/video/transcode/list` | 转码列表 | **仅限三方挂载应用授权 token** |
| `Delete(ctx, fileID, mode)` | `POST /api/v1/transcode/delete` (QPS 10) | 删除转码视频 | mode: 1 仅原文件、2 原文件+转码产物（不可逆） |
| `DownloadOriginal(ctx, fileID)` | `POST /api/v1/transcode/file/download` (QPS 10) | 原文件下载地址 | 转码空间满（isFull）时无地址 |
| `DownloadM3U8(ctx, fileID, resolution)` | `POST /api/v1/transcode/m3u8_ts/download` (QPS 20) | m3u8 下载地址 | 分辨率 P 大写 |
| `DownloadTS(ctx, fileID, resolution, tsName)` | 同上 | 单个 ts 下载地址 | tsName 不含 .ts 后缀（"000.ts" 传 "000"） |
| `DownloadAll(ctx, fileID, zipName)` | `POST /api/v1/transcode/file/download/all` (QPS 1) | 全部产物打包下载 | isDownloading=true 时 10 秒轮询；返回 URL 含 token 注意脱敏 |

转码状态码：1 准备转码；2 转码中；3-254 失败；255 成功。

## OAuth — 第三方授权

面向三方挂载应用（需官方资质审核，暂不支持个人开发者）。授权域名为 `yun.123pan.com`。

| 方法 | 接口 | 功能 | 说明 |
|---|---|---|---|
| `AuthURL(clientID, redirectURI, state)` | —（本地拼接） | 生成用户授权页地址 | scope 固定 `user:base,file:all:read,file:all:write`；用户授权后携带 code 回跳 |
| `TokenByCode(ctx, clientID, clientSecret, code, redirectURI)` | `POST /api/v1/oauth2/access_token` | code 换 token | redirectURI 须与注册的回调一致；限流 100 次/分钟 |
| `RefreshToken(ctx, clientID, clientSecret, refreshToken)` | 同上 | 刷新 token | refresh_token **单次有效**（90 天），刷新后必须持久化新值；旧 access_token 立即失效 |

拿到 token 后用 `pan123.NewWithToken(accessToken)` 创建客户端；`expires_in` 为秒（通常 7200）。

## 错误处理

响应 `code != 0` 时返回 `*pan123.APIError{Code, Message, TraceID}`：

```go
var apiErr *pan123.APIError
if errors.As(err, &apiErr) {
	// apiErr.TraceID 用于联系官方技术支持定位问题
}
pan123.IsTokenExpired(err) // code == 401
pan123.IsRateLimited(err)  // code == 429
```

常见业务错误码：`401` token 无效；`429` 请求过频；`5066` 文件不存在；`5113` 下载流量超限。

## 限流与重试

- SDK 内置按官方 QPS 表的**客户端限流**（见 `ratelimit.go` 的 `apiQPS`），未列出的接口不限流；`WithoutRateLimit()` 可关闭
- 服务端返回 429 时自动指数退避重试（500ms 起，上限 8s），默认 3 次，`WithMaxRetries` 可调、0 关闭
- 官方对开发者与三方授权场景的限流表不完全一致，SDK 按开发者接入文档配置
