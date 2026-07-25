// pan123 是 123 云盘开放平台的命令行工具。
//
// 首次使用请先保存凭证（在 https://www.123pan.com/developer 申请）：
//
//	pan123 login -client-id <ID> -client-secret <SECRET>
//
// 之后即可使用各子命令，运行 pan123 -h 查看完整帮助。
// access_token 会缓存在配置目录并跨进程复用，避免触发官方
// 同 client_id 最多 3 个 token 的限制。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"time"

	pan123 "github.com/okatu-loli/go-123pan"
)

// version 由 -ldflags "-X main.version=..." 注入；
// 未注入时回退到 go install 记录的模块版本。
var version = "dev"

func versionString() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return version
}

const usageText = `pan123 - 123 云盘命令行工具

用法: pan123 <命令> [参数]

凭证:
  login       保存 clientID/clientSecret     pan123 login -client-id ID -client-secret SECRET
  logout      清除本地凭证与 token 缓存

账号:
  whoami      显示当前用户信息与空间用量

文件:
  ls          列出目录文件                    pan123 ls [目录ID]（默认根目录 0）
  mkdir       创建目录                        pan123 mkdir -parent 0 <目录名>
  upload      上传文件（自动秒传/分片）       pan123 upload -parent 0 <本地文件>
  download    下载文件                        pan123 download -o 保存路径 <文件ID>
  rm          删除文件至回收站                pan123 rm <文件ID> [文件ID...]
  mv          移动文件                        pan123 mv -to <目标目录ID> <文件ID> [文件ID...]
  rename      重命名文件                      pan123 rename <文件ID> <新名称>

分享与传输:
  share       创建分享链接                    pan123 share [-pwd 提取码] [-expire 7] <文件ID> [文件ID...]
  offline     创建离线下载并等待完成          pan123 offline [-dir 目录ID] <URL>
  link        获取文件直链                    pan123 link <文件ID>

其他:
  version     显示版本

环境变量 PAN123_CLIENT_ID / PAN123_CLIENT_SECRET 可代替 login 保存的凭证。`

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usageText)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "login":
		err = cmdLogin(args)
	case "logout":
		err = cmdLogout()
	case "whoami":
		err = cmdWhoami(ctx)
	case "ls":
		err = cmdLs(ctx, args)
	case "mkdir":
		err = cmdMkdir(ctx, args)
	case "upload":
		err = cmdUpload(ctx, args)
	case "download":
		err = cmdDownload(ctx, args)
	case "rm":
		err = cmdRm(ctx, args)
	case "mv":
		err = cmdMv(ctx, args)
	case "rename":
		err = cmdRename(ctx, args)
	case "share":
		err = cmdShare(ctx, args)
	case "offline":
		err = cmdOffline(ctx, args)
	case "link":
		err = cmdLink(ctx, args)
	case "version", "-v", "--version":
		fmt.Println("pan123", versionString())
	case "help", "-h", "--help":
		fmt.Println(usageText)
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q，运行 pan123 -h 查看帮助\n", cmd)
		os.Exit(2)
	}
	if err != nil {
		var apiErr *pan123.APIError
		if errors.As(err, &apiErr) {
			fmt.Fprintf(os.Stderr, "接口错误 (code=%d): %s\n", apiErr.Code, apiErr.Message)
		} else {
			fmt.Fprintln(os.Stderr, "错误:", err)
		}
		os.Exit(1)
	}
}

func cmdLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	id := fs.String("client-id", "", "开放平台 clientID")
	secret := fs.String("client-secret", "", "开放平台 clientSecret")
	_ = fs.Parse(args)
	if *id == "" || *secret == "" {
		return errors.New("必须同时提供 -client-id 与 -client-secret")
	}
	if err := saveConfig(&config{ClientID: *id, ClientSecret: *secret}); err != nil {
		return err
	}
	// 立即验证凭证有效性
	c := pan123.New(*id, *secret)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tok, err := c.RefreshToken(ctx)
	if err != nil {
		return fmt.Errorf("凭证验证失败: %w", err)
	}
	if err := saveToken(tok.AccessToken, tok.ExpiredAt); err != nil {
		return err
	}
	fmt.Println("凭证已保存并验证成功，token 有效期至", tok.ExpiredAt.Format("2006-01-02 15:04"))
	return nil
}

func cmdLogout() error {
	if err := removeConfig(); err != nil {
		return err
	}
	fmt.Println("已清除本地凭证与 token 缓存")
	return nil
}

func cmdWhoami(ctx context.Context) error {
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)
	info, err := c.User.Info(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("昵称:     %s\n", info.Nickname)
	fmt.Printf("UID:      %d\n", info.UID)
	fmt.Printf("会员:     %v\n", info.Vip)
	fmt.Printf("已用空间: %s\n", humanSize(info.SpaceUsed))
	fmt.Printf("总空间:   %s (永久) + %s (临时)\n", humanSize(info.SpacePermanent), humanSize(info.SpaceTemp))
	fmt.Printf("直链流量: %s\n", humanSize(info.DirectTraffic))
	return nil
}

func cmdLs(ctx context.Context, args []string) error {
	parent := int64(0)
	if len(args) > 0 {
		var err error
		if parent, err = strconv.ParseInt(args[0], 10, 64); err != nil {
			return fmt.Errorf("无效的目录ID %q", args[0])
		}
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)
	files, err := c.File.ListAll(ctx, parent)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("(空目录)")
		return nil
	}
	fmt.Printf("%-14s %-4s %10s  %s\n", "文件ID", "类型", "大小", "名称")
	for _, f := range files {
		kind := "文件"
		if f.Type == 1 {
			kind = "目录"
		}
		fmt.Printf("%-14d %-4s %10s  %s\n", f.FileID, kind, humanSize(f.Size), f.Filename)
	}
	return nil
}

func cmdMkdir(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mkdir", flag.ExitOnError)
	parent := fs.Int64("parent", 0, "父目录ID（默认根目录）")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		return errors.New("用法: pan123 mkdir -parent 0 <目录名>")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)
	dirID, err := c.File.Mkdir(ctx, *parent, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Printf("目录已创建, dirID=%d\n", dirID)
	return nil
}

func cmdUpload(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("upload", flag.ExitOnError)
	parent := fs.Int64("parent", 0, "目标目录ID（默认根目录）")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		return errors.New("用法: pan123 upload -parent 0 <本地文件>")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)
	start := time.Now()
	fileID, err := c.Upload.UploadFromPath(ctx, *parent, fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Printf("上传成功, fileID=%d, 耗时 %s\n", fileID, time.Since(start).Round(time.Millisecond))
	return nil
}

func cmdDownload(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("download", flag.ExitOnError)
	out := fs.String("o", "", "保存路径（默认使用云端文件名）")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		return errors.New("用法: pan123 download -o 保存路径 <文件ID>")
	}
	fileID, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil {
		return fmt.Errorf("无效的文件ID %q", fs.Arg(0))
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)

	dest := *out
	if dest == "" {
		detail, err := c.File.Detail(ctx, fileID)
		if err != nil {
			return err
		}
		dest = detail.Filename
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := c.File.DownloadTo(ctx, fileID, f)
	if err != nil {
		return err
	}
	fmt.Printf("下载完成: %s (%s)\n", dest, humanSize(n))
	return nil
}

func cmdRm(ctx context.Context, args []string) error {
	ids, err := parseIDs(args)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errors.New("用法: pan123 rm <文件ID> [文件ID...]")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)
	if err := c.File.Trash(ctx, ids); err != nil {
		return err
	}
	fmt.Printf("%d 个文件已移入回收站\n", len(ids))
	return nil
}

func cmdMv(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("mv", flag.ExitOnError)
	to := fs.Int64("to", -1, "目标目录ID（根目录为 0）")
	_ = fs.Parse(args)
	ids, err := parseIDs(fs.Args())
	if err != nil {
		return err
	}
	if *to < 0 || len(ids) == 0 {
		return errors.New("用法: pan123 mv -to <目标目录ID> <文件ID> [文件ID...]")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)
	if err := c.File.Move(ctx, ids, *to); err != nil {
		return err
	}
	fmt.Printf("%d 个文件已移动到目录 %d\n", len(ids), *to)
	return nil
}

func cmdRename(ctx context.Context, args []string) error {
	if len(args) < 2 {
		return errors.New("用法: pan123 rename <文件ID> <新名称>")
	}
	fileID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的文件ID %q", args[0])
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)
	if err := c.File.Rename(ctx, fileID, args[1]); err != nil {
		return err
	}
	fmt.Println("重命名成功")
	return nil
}

func cmdShare(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("share", flag.ExitOnError)
	pwd := fs.String("pwd", "", "提取码（可选）")
	expire := fs.Int("expire", 7, "有效期天数: 1/7/30/0(永久)")
	name := fs.String("name", "pan123 分享", "分享名称")
	_ = fs.Parse(args)
	ids, err := parseIDs(fs.Args())
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errors.New("用法: pan123 share [-pwd 提取码] [-expire 7] <文件ID> [文件ID...]")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)
	share, err := c.Share.Create(ctx, &pan123.ShareCreateRequest{
		ShareName:   *name,
		ShareExpire: *expire,
		FileIDs:     ids,
		SharePwd:    *pwd,
	})
	if err != nil {
		return err
	}
	info, err := c.User.Info(ctx)
	if err != nil {
		return err
	}
	fmt.Println("分享链接:", pan123.ShareURL(info.UID, share.ShareKey))
	if *pwd != "" {
		fmt.Println("提取码:", *pwd)
	}
	return nil
}

func cmdOffline(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("offline", flag.ExitOnError)
	dir := fs.Int64("dir", 0, "下载到的目录ID（默认官方\"来自:离线下载\"目录）")
	_ = fs.Parse(args)
	if fs.NArg() < 1 {
		return errors.New("用法: pan123 offline [-dir 目录ID] <URL>")
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)
	taskID, err := c.Offline.Download(ctx, &pan123.OfflineDownloadRequest{URL: fs.Arg(0), DirID: *dir})
	if err != nil {
		return err
	}
	fmt.Printf("任务已创建 (taskID=%d)，等待完成...\n", taskID)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
		p, err := c.Offline.Process(ctx, taskID)
		if err != nil {
			return err
		}
		switch p.Status {
		case pan123.OfflineSuccess:
			fmt.Println("离线下载完成")
			return nil
		case pan123.OfflineFailed:
			return errors.New("离线下载失败")
		default:
			fmt.Printf("\r进度: %.1f%%", p.Process)
		}
	}
}

func cmdLink(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("用法: pan123 link <文件ID>")
	}
	fileID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的文件ID %q", args[0])
	}
	c, err := newClient()
	if err != nil {
		return err
	}
	defer persistToken(c)
	u, err := c.Link.URL(ctx, fileID)
	if err != nil {
		return err
	}
	fmt.Println(u)
	return nil
}

func parseIDs(args []string) ([]int64, error) {
	ids := make([]int64, 0, len(args))
	for _, a := range args {
		id, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的文件ID %q", a)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
