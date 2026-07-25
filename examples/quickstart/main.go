// quickstart 演示：创建客户端、获取用户信息、列出根目录文件。
//
// 运行前设置环境变量：
//
//	export PAN123_CLIENT_ID=你的clientID
//	export PAN123_CLIENT_SECRET=你的clientSecret
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	pan123 "github.com/okatu-loli/go-123pan"
)

func main() {
	client := pan123.New(os.Getenv("PAN123_CLIENT_ID"), os.Getenv("PAN123_CLIENT_SECRET"))
	ctx := context.Background()

	info, err := client.User.Info(ctx)
	if err != nil {
		log.Fatalf("获取用户信息失败: %v", err)
	}
	fmt.Printf("你好, %s (uid=%d)\n", info.Nickname, info.UID)
	fmt.Printf("已用空间: %.2f GB / 永久空间: %.2f GB\n",
		float64(info.SpaceUsed)/1e9, float64(info.SpacePermanent)/1e9)

	files, err := client.File.ListAll(ctx, 0)
	if err != nil {
		log.Fatalf("获取文件列表失败: %v", err)
	}
	fmt.Printf("根目录共 %d 个文件/目录:\n", len(files))
	for _, f := range files {
		kind := "文件"
		if f.Type == 1 {
			kind = "目录"
		}
		fmt.Printf("  [%s] %s (id=%d, %d bytes)\n", kind, f.Filename, f.FileID, f.Size)
	}
}
