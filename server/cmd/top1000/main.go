package main

import (
	"log"

	"github.com/joho/godotenv"
	"top1000/internal/server"
)

// @title Top1000 API
// @version 1.0
// @description Top1000 站点数据查询 API
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:7066
// @BasePath /
// @schemes http https

func main() {
	// 加载 .env 文件（非必需，失败时使用系统环境变量）
	_ = godotenv.Load()

	log.Println("[main] 环境变量已加载")

	// 使用兼容性启动函数（自动处理 context 和优雅关闭）
	server.StartCompat()
}
