// SD-WAN Agent 主程序
package main

import (
	"flag"
	"log"
	"os"

	"github.com/holygeek00/lite-sdwan/internal/agent"
	"github.com/holygeek00/lite-sdwan/pkg/config"
)

func main() {
	configPath := flag.String("config", "config/agent_config.yaml", "Path to config file")
	flag.Parse()

	// 设置日志格式
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// 加载配置
	cfg, err := config.LoadAgentConfig(*configPath)
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		os.Exit(1)
	}

	log.Printf("Starting SD-WAN Agent...")
	log.Printf("Config: agent_id=%s, controller=%s, peers=%v",
		cfg.AgentID, cfg.Controller.URL, cfg.Network.PeerIPs)

	// 创建并运行 Agent
	a, err := agent.NewAgent(cfg)
	if err != nil {
		log.Printf("Failed to create agent: %v", err)
		os.Exit(1)
	}

	a.Run()
}
