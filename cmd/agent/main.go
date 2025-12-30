// SD-WAN Agent 主程序
package main

import (
	"flag"
	"os"

	"github.com/holygeek00/lite-sdwan/internal/agent"
	"github.com/holygeek00/lite-sdwan/pkg/config"
	"github.com/holygeek00/lite-sdwan/pkg/logging"
)

func main() {
	configPath := flag.String("config", "config/agent_config.yaml", "Path to config file")
	flag.Parse()

	// 加载配置
	cfg, err := config.LoadAgentConfig(*configPath)
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

	logger.Info("Starting SD-WAN Agent",
		logging.F("agent_id", cfg.AgentID),
		logging.F("controller_url", cfg.Controller.URL),
		logging.F("peer_count", len(cfg.Network.PeerIPs)),
		logging.F("log_level", cfg.Logging.Level),
	)

	// 创建并运行 Agent
	a, err := agent.NewAgentWithLogger(cfg, logger)
	if err != nil {
		logger.Error("Failed to create agent",
			logging.F("error", err.Error()),
		)
		os.Exit(1)
	}

	a.Run()
}
