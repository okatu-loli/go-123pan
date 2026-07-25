// share 演示：创建分享链接并拼接可访问的分享地址。
//
// 用法：
//
//	export PAN123_CLIENT_ID=... PAN123_CLIENT_SECRET=...
//	go run . <fileID> [提取码]
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	pan123 "github.com/okatu-loli/go-123pan"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("用法: go run . <fileID> [提取码]")
	}
	fileID, err := strconv.ParseInt(os.Args[1], 10, 64)
	if err != nil {
		log.Fatalf("无效的 fileID: %v", err)
	}
	pwd := ""
	if len(os.Args) > 2 {
		pwd = os.Args[2]
	}

	client := pan123.New(os.Getenv("PAN123_CLIENT_ID"), os.Getenv("PAN123_CLIENT_SECRET"))
	ctx := context.Background()

	share, err := client.Share.Create(ctx, &pan123.ShareCreateRequest{
		ShareName:   "SDK 分享示例",
		ShareExpire: 7, // 7 天有效
		FileIDs:     []int64{fileID},
		SharePwd:    pwd,
	})
	if err != nil {
		log.Fatalf("创建分享失败: %v", err)
	}

	info, err := client.User.Info(ctx)
	if err != nil {
		log.Fatalf("获取用户信息失败: %v", err)
	}
	fmt.Printf("分享创建成功: %s\n", pan123.ShareURL(info.UID, share.ShareKey))
	if pwd != "" {
		fmt.Printf("提取码: %s\n", pwd)
	}
}
