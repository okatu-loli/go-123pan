// upload 演示：一行代码上传本地文件（自动 MD5、秒传检测、分片并发上传、轮询完成）。
//
// 用法：
//
//	export PAN123_CLIENT_ID=... PAN123_CLIENT_SECRET=...
//	go run . /path/to/local/file
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
		log.Fatal("用法: go run . <本地文件路径>")
	}
	client := pan123.New(os.Getenv("PAN123_CLIENT_ID"), os.Getenv("PAN123_CLIENT_SECRET"))

	fileID, err := client.Upload.UploadFromPath(context.Background(), 0, os.Args[1])
	if err != nil {
		log.Fatalf("上传失败: %v", err)
	}
	fmt.Printf("上传成功, fileID=%d\n", fileID)
}
