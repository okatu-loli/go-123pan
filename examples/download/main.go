// download 演示：获取下载直链并保存文件到本地。
//
// 用法：
//
//	export PAN123_CLIENT_ID=... PAN123_CLIENT_SECRET=...
//	go run . <fileID> <保存路径>
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
	if len(os.Args) < 3 {
		log.Fatal("用法: go run . <fileID> <保存路径>")
	}
	fileID, err := strconv.ParseInt(os.Args[1], 10, 64)
	if err != nil {
		log.Fatalf("无效的 fileID: %v", err)
	}
	client := pan123.New(os.Getenv("PAN123_CLIENT_ID"), os.Getenv("PAN123_CLIENT_SECRET"))

	out, err := os.Create(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	n, err := client.File.DownloadTo(context.Background(), fileID, out)
	if err != nil {
		log.Fatalf("下载失败: %v", err)
	}
	fmt.Printf("下载完成, 写入 %d bytes -> %s\n", n, os.Args[2])
}
