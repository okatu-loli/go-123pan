# go-123pan

<div align="center">

**Unofficial Go SDK for the 123Pan (123 Cloud Disk) Open Platform**

[![Go Reference](https://pkg.go.dev/badge/github.com/okatu-loli/go-123pan.svg)](https://pkg.go.dev/github.com/okatu-loli/go-123pan)
[![Go Report Card](https://goreportcard.com/badge/github.com/okatu-loli/go-123pan)](https://goreportcard.com/report/github.com/okatu-loli/go-123pan)
[![CI](https://github.com/okatu-loli/go-123pan/actions/workflows/ci.yml/badge.svg)](https://github.com/okatu-loli/go-123pan/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/okatu-loli/go-123pan)](https://github.com/okatu-loli/go-123pan/releases)
[![License](https://img.shields.io/github/license/okatu-loli/go-123pan)](LICENSE)

[简体中文](README.md) | [English](README_EN.md)

</div>

A Go SDK for the [123Pan Open Platform](https://www.123pan.com/developer), covering the complete API surface: file management, upload (V1/V2/instant), sharing, offline download, direct links, image hosting, video transcoding, and third-party OAuth.

## ✨ Features

- **Batteries included** — `pan123.New(clientID, clientSecret)` and you're ready to call every API
- **Automatic token management** — fetch, cache, and refresh before expiry; concurrency-safe (avoids the 3-tokens-per-client_id kick-out limit)
- **One-line uploads** — `UploadFile` handles MD5, instant-upload detection, concurrent slice upload, and completion polling
- **Built-in rate limiting & retries** — client-side QPS limiting per the official table, exponential backoff on 429
- **Rich errors** — business errors surface as `*APIError` (code, message, x-traceID), compatible with `errors.As`
- **Full API coverage** — including the legacy V1 upload flow and legacy file listing
- **Minimal dependencies** — standard library plus `golang.org/x/time` only

## 📦 Installation

```bash
go get github.com/okatu-loli/go-123pan
```

Requires Go 1.22+.

## 🚀 Quick Start

Get your `clientID` and `clientSecret` from the [123Pan Open Platform](https://www.123pan.com/developer).

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

	// Fetch user info
	info, err := client.User.Info(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Hello,", info.Nickname)

	// Upload a file in one line (instant upload / slicing / polling handled for you)
	fileID, err := client.Upload.UploadFromPath(ctx, 0, "/path/to/file.zip")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Uploaded, fileID =", fileID)

	// List everything in the root directory (auto pagination)
	files, _ := client.File.ListAll(ctx, 0)
	for _, f := range files {
		fmt.Println(f.Filename)
	}
}
```

More runnable examples live in [examples/](examples/):

| Example | Description |
|---|---|
| [quickstart](examples/quickstart/) | User info + file listing |
| [upload](examples/upload/) | Upload a local file |
| [download](examples/download/) | Download a file |
| [share](examples/share/) | Create a share link |
| [offline](examples/offline/) | Offline download + progress polling |
| [imagebed](examples/imagebed/) | Image-bed upload + public URL |

## 📚 API Coverage

| Service | Scope | Key methods |
|---|---|---|
| `client.File` | File management | `Mkdir` `Rename` `BatchRename` `Trash` `Recover` `RecoverTo` `Copy` `AsyncCopy` `CopyProcess` `Move` `Detail` `Infos` `List` `ListAll` `ListV1` `SafeboxID` `DownloadInfo` `DownloadTo` |
| `client.Upload` | Uploads | `UploadFile` `UploadFromPath` (high-level); V2: `Create` `UploadSlice` `Complete` `Domains` `SingleCreate` `Sha1Reuse`; V1: `CreateV1` `GetUploadURLV1` `PutSliceV1` `ListUploadPartsV1` `CompleteV1` `AsyncResultV1` |
| `client.Share` | Sharing | `Create` `List` `Update` `CreatePaid` `ListPaid` `UpdatePaid` |
| `client.Offline` | Offline download | `Download` `Process` |
| `client.User` | User | `Info` |
| `client.Link` | Direct links | `Enable` `Disable` `URL` `RefreshCache` `TrafficLog` `OfflineLogs` `SwitchIPBlacklist` `UpdateIPBlacklist` `IPBlacklist` |
| `client.Oss` | Image bed | `UploadFile` `UploadFromPath` (high-level) `Mkdir` `CreateFile` `GetUploadURL` `UploadComplete` `UploadAsyncResult` `CopyFromDisk` `CopyProcess` `CopyFailList` `Move` `Delete` `Detail` `List` `OfflineDownload` `OfflineProcess` |
| `client.Transcode` | Video transcoding | `FolderInfo` `CloudVideoFiles` `SpaceFiles` `UploadFromCloudDisk` `Resolutions` `Transcode` `Records` `Results` `List` `Delete` `DownloadOriginal` `DownloadM3U8` `DownloadTS` `DownloadAll` |
| `client.OAuth` | Third-party OAuth | `AuthURL` `TokenByCode` `RefreshToken` |

## ⚙️ Options

```go
client := pan123.New(id, secret,
	pan123.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
	pan123.WithMaxRetries(5),      // retries on 429 (default 3, 0 disables)
	pan123.WithoutRateLimit(),     // disable built-in client-side rate limiting
	pan123.WithBaseURL("..."),     // override API host (testing/proxy)
)

// Third-party mount apps: use an OAuth access token directly
client := pan123.NewWithToken(accessToken)
```

## 🧯 Error Handling

```go
_, err := client.File.DownloadInfo(ctx, fileID)
var apiErr *pan123.APIError
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.Code, apiErr.Message, apiErr.TraceID)
}
if pan123.IsRateLimited(err) { /* too many requests */ }
if pan123.IsTokenExpired(err) { /* token expired */ }
```

## 🤝 Contributing

Issues and pull requests are welcome!

Please follow [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, …) — after merging to `main`, CI automatically publishes a semantic version (`fix` → patch, `feat` → minor, `BREAKING CHANGE` → major).

### Contributors

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

## 📄 License

[MIT](LICENSE)

> This is a community project, not affiliated with 123Pan. Refer to the [official docs](https://123yunpan.yuque.com/org-wiki-123yunpan-muaork/cr6ced) as the source of truth.
