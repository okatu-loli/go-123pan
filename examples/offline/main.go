// offline 演示：创建离线下载任务并轮询进度直到完成。
//
// 用法：
//
//	export PAN123_CLIENT_ID=... PAN123_CLIENT_SECRET=...
//	go run . https://example.com/file.mp4
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	pan123 "github.com/okatu-loli/go-123pan"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("用法: go run . <资源URL>")
	}
	client := pan123.New(os.Getenv("PAN123_CLIENT_ID"), os.Getenv("PAN123_CLIENT_SECRET"))
	ctx := context.Background()

	taskID, err := client.Offline.Download(ctx, &pan123.OfflineDownloadRequest{URL: os.Args[1]})
	if err != nil {
		log.Fatalf("创建离线下载任务失败: %v", err)
	}
	fmt.Printf("任务已创建, taskID=%d, 开始轮询进度...\n", taskID)

	for {
		time.Sleep(3 * time.Second)
		p, err := client.Offline.Process(ctx, taskID)
		if err != nil {
			log.Fatalf("查询进度失败: %v", err)
		}
		switch p.Status {
		case pan123.OfflineSuccess:
			fmt.Println("离线下载完成 ✔")
			return
		case pan123.OfflineFailed:
			log.Fatal("离线下载失败 ✘")
		default:
			fmt.Printf("进度: %.1f%% (状态=%d)\n", p.Process, p.Status)
		}
	}
}
