package main

import (
	"context"
	"log"

	"github.com/joho/godotenv"
	"top1000/internal/server"
)

func main() {
	// 加载 .env 文件（非必需，失败时使用系统环境变量）
	_ = godotenv.Load()

	log.Println("[main] 环境变量已加载")

	srv := server.New()
	if err := srv.Start(context.Background()); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
