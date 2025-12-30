// SD-WAN Controller 主程序
package main

import (
	"flag"
	"os"

	"github.com/holygeek00/lite-sdwan/internal/controller"
	"github.com/holygeek00/lite-sdwan/pkg/config"
	"github.com/holygeek00/lite-sdwan/pkg/logging"
)

func main() {
	configPath := flag.String("config", "config/controller_config.yaml", "Path to config file")
	flag.Parse()

	// 加载配置
	cfg, err := config.LoadControllerConfig(*configPath)
	if err != nil {
		// 配置加载失败时使用默认 logger
		logger := logging.NewJSONLogger(logging.ERROR, os.Stderr)
		logger.Error("Failed to load config",
			logging.F("error", err.Error()),
			logging.F("config_path", *configPath),
		)
		os.Exit(1)
	}

	// 从配置创建 Logger
	logger := logging.NewJSONLoggerFromString(cfg.Logging.Level, os.Stdout)

	logger.Info("Starting SD-WAN Controller",
		logging.F("listen_address", cfg.Server.ListenAddress),
		logging.F("port", cfg.Server.Port),
		logging.F("penalty_factor", cfg.Algorithm.PenaltyFactor),
		logging.F("hysteresis", cfg.Algorithm.Hysteresis),
		logging.F("log_level", cfg.Logging.Level),
	)

	// 创建并启动服务器
	server := controller.NewServer(cfg)
	if err := server.Run(); err != nil {
		logger.Error("Server error",
			logging.F("error", err.Error()),
		)
		os.Exit(1)
	}
}
