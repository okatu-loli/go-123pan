// imagebed 演示：上传本地图片到图床并获取外链。
//
// 用法：
//
//	export PAN123_CLIENT_ID=... PAN123_CLIENT_SECRET=...
//	go run . /path/to/image.png
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	pan123 "github.com/okatu-loli/go-123pan"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("用法: go run . <本地图片路径>")
	}
	client := pan123.New(os.Getenv("PAN123_CLIENT_ID"), os.Getenv("PAN123_CLIENT_SECRET"))
	ctx := context.Background()

	fileID, err := client.Oss.UploadFromPath(ctx, "", os.Args[1])
	if err != nil {
		log.Fatalf("图床上传失败: %v", err)
	}
	fmt.Printf("上传成功, fileID=%s\n", fileID)

	detail, err := client.Oss.Detail(ctx, fileID)
	if err != nil {
		log.Fatalf("获取图片详情失败: %v", err)
	}
	fmt.Printf("图片外链: %s\n", detail.DownloadURL)
}
