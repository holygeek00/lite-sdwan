// Package config 提供配置文件解析功能
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentConfig Agent 配置
type AgentConfig struct {
	AgentID    string           `yaml:"agent_id"`
	Controller ControllerClient `yaml:"controller"`
	Probe      ProbeConfig      `yaml:"probe"`
	Sync       SyncConfig       `yaml:"sync"`
	Network    NetworkConfig    `yaml:"network"`
	Logging    LoggingConfig    `yaml:"logging"`
}

// ControllerClient Controller 客户端配置
type ControllerClient struct {
	URL     string        `yaml:"url"`
	Timeout time.Duration `yaml:"timeout"`
}

// ProbeConfig 探测配置
type ProbeConfig struct {
	Interval   time.Duration `yaml:"interval"`
	Timeout    time.Duration `yaml:"timeout"`
	WindowSize int           `yaml:"window_size"`
}

// SyncConfig 同步配置
type SyncConfig struct {
	Interval      time.Duration `yaml:"interval"`
	RetryAttempts int           `yaml:"retry_attempts"`
	RetryBackoff  []int         `yaml:"retry_backoff"` // 秒
}

// NetworkConfig 网络配置
type NetworkConfig struct {
	WGInterface string   `yaml:"wg_interface"`
	Subnet      string   `yaml:"subnet"`
	PeerIPs     []string `yaml:"peer_ips"`
}

// ControllerConfig Controller 配置
type ControllerConfig struct {
	Server    ServerConfig    `yaml:"server"`
	Algorithm AlgorithmConfig `yaml:"algorithm"`
	Topology  TopologyConfig  `yaml:"topology"`
	Logging   LoggingConfig   `yaml:"logging"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	ListenAddress string `yaml:"listen_address"`
	Port          int    `yaml:"port"`
}

// AlgorithmConfig 算法配置
type AlgorithmConfig struct {
	PenaltyFactor float64 `yaml:"penalty_factor"`
	Hysteresis    float64 `yaml:"hysteresis"`
}

// TopologyConfig 拓扑配置
type TopologyConfig struct {
	StaleThreshold time.Duration `yaml:"stale_threshold"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

// LoadAgentConfig 从文件加载 Agent 配置
func LoadAgentConfig(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- config file path is trusted input
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 设置默认值
	if cfg.Probe.Interval == 0 {
		cfg.Probe.Interval = 5 * time.Second
	}
	if cfg.Probe.Timeout == 0 {
		cfg.Probe.Timeout = 2 * time.Second
	}
	if cfg.Probe.WindowSize == 0 {
		cfg.Probe.WindowSize = 10
	}
	if cfg.Sync.Interval == 0 {
		cfg.Sync.Interval = 10 * time.Second
	}
	if cfg.Sync.RetryAttempts == 0 {
		cfg.Sync.RetryAttempts = 3
	}
	if len(cfg.Sync.RetryBackoff) == 0 {
		cfg.Sync.RetryBackoff = []int{1, 2, 4}
	}
	if cfg.Network.WGInterface == "" {
		cfg.Network.WGInterface = "wg0"
	}
	if cfg.Network.Subnet == "" {
		cfg.Network.Subnet = "10.254.0.0/24"
	}
	if cfg.Controller.Timeout == 0 {
		cfg.Controller.Timeout = 5 * time.Second
	}
	if cfg.Network.PeerIPs == nil {
		cfg.Network.PeerIPs = []string{}
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "INFO"
	}

	// 执行配置验证
	validationErrors := ValidateAgentConfig(&cfg)
	if len(validationErrors) > 0 {
		return nil, fmt.Errorf("%s", FormatValidationErrors(validationErrors))
	}

	return &cfg, nil
}

// LoadControllerConfig 从文件加载 Controller 配置
func LoadControllerConfig(path string) (*ControllerConfig, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- config file path is trusted input
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg ControllerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 设置默认值
	if cfg.Server.ListenAddress == "" {
		cfg.Server.ListenAddress = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8000
	}
	if cfg.Algorithm.PenaltyFactor == 0 {
		cfg.Algorithm.PenaltyFactor = 100
	}
	if cfg.Algorithm.Hysteresis == 0 {
		cfg.Algorithm.Hysteresis = 0.15
	}
	if cfg.Topology.StaleThreshold == 0 {
		cfg.Topology.StaleThreshold = 60 * time.Second
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "INFO"
	}

	// 执行配置验证
	validationErrors := ValidateControllerConfig(&cfg)
	if len(validationErrors) > 0 {
		return nil, fmt.Errorf("%s", FormatValidationErrors(validationErrors))
	}

	return &cfg, nil
}
