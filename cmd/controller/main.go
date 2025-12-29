// SD-WAN Controller 主程序
package main

import (
	"flag"
	"log"
	"os"

	"github.com/holygeek00/lite-sdwan/internal/controller"
	"github.com/holygeek00/lite-sdwan/pkg/config"
)

func main() {
	configPath := flag.String("config", "config/controller_config.yaml", "Path to config file")
	flag.Parse()

	// 设置日志格式
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 加载配置
	cfg, err := config.LoadControllerConfig(*configPath)
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		os.Exit(1)
	}

	log.Printf("Starting SD-WAN Controller...")
	log.Printf("Config: listen=%s:%d, penalty_factor=%.0f, hysteresis=%.2f",
		cfg.Server.ListenAddress, cfg.Server.Port,
		cfg.Algorithm.PenaltyFactor, cfg.Algorithm.Hysteresis)

	// 创建并启动服务器
	server := controller.NewServer(cfg)
	if err := server.Run(); err != nil {
		log.Printf("Server error: %v", err)
		os.Exit(1)
	}
}
