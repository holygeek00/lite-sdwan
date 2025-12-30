// Package config 提供配置文件解析和验证功能
package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidationError 配置验证错误
type ValidationError struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

// Error 实现 error 接口
func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s (got: '%s')", e.Field, e.Message, e.Value)
}

// ValidationResult 验证结果
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// ValidateIPAddress 验证 IP 地址格式
// 返回 true 如果字符串是有效的 IPv4 地址（四个 0-255 的八位组，用点分隔）
func ValidateIPAddress(ip string) bool {
	if ip == "" {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	// 确保是 IPv4 地址
	return parsed.To4() != nil
}

// ValidateURL 验证 URL 格式
// 返回 true 如果字符串是有效的 HTTP 或 HTTPS URL
func ValidateURL(urlStr string) bool {
	if urlStr == "" {
		return false
	}
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	// 必须是 http 或 https 协议
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	// 必须有主机名
	if parsed.Host == "" {
		return false
	}
	return true
}

// ValidatePort 验证端口范围
// 返回 true 如果端口在有效范围 [1, 65535] 内
func ValidatePort(port int) bool {
	return port >= 1 && port <= 65535
}

// ValidateSubnet 验证子网格式
// 返回 true 如果字符串是有效的 CIDR 格式子网（如 10.0.0.0/24）
func ValidateSubnet(subnet string) bool {
	if subnet == "" {
		return false
	}
	_, _, err := net.ParseCIDR(subnet)
	return err == nil
}

// ValidateListenAddress 验证监听地址格式
// 支持 IP 地址或 0.0.0.0 格式
func ValidateListenAddress(addr string) bool {
	if addr == "" {
		return false
	}
	// 0.0.0.0 是有效的监听地址
	if addr == "0.0.0.0" {
		return true
	}
	// 检查是否是有效的 IP 地址
	return ValidateIPAddress(addr)
}

// ValidateHostPort 验证 host:port 格式
func ValidateHostPort(addr string) bool {
	if addr == "" {
		return false
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		return false
	}
	// 验证端口
	var port int
	_, err = fmt.Sscanf(portStr, "%d", &port)
	if err != nil {
		return false
	}
	return ValidatePort(port)
}

// ValidateAgentConfig 验证 Agent 配置
// 返回所有验证错误的列表
func ValidateAgentConfig(cfg *AgentConfig) []ValidationError {
	var errors []ValidationError

	// 验证 agent_id
	if cfg.AgentID == "" {
		errors = append(errors, ValidationError{
			Field:   "agent_id",
			Value:   "",
			Message: "agent_id is required",
		})
	}

	// 验证 controller.url
	if cfg.Controller.URL == "" {
		errors = append(errors, ValidationError{
			Field:   "controller.url",
			Value:   "",
			Message: "controller.url is required",
		})
	} else if !ValidateURL(cfg.Controller.URL) {
		errors = append(errors, ValidationError{
			Field:   "controller.url",
			Value:   cfg.Controller.URL,
			Message: "controller.url must be a valid HTTP or HTTPS URL (e.g., http://controller:8000)",
		})
	}

	// 验证 network.peer_ips
	if len(cfg.Network.PeerIPs) == 0 {
		errors = append(errors, ValidationError{
			Field:   "network.peer_ips",
			Value:   "[]",
			Message: "network.peer_ips cannot be empty, at least one peer IP is required",
		})
	} else {
		for i, ip := range cfg.Network.PeerIPs {
			if !ValidateIPAddress(ip) {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("network.peer_ips[%d]", i),
					Value:   ip,
					Message: "must be a valid IPv4 address (e.g., 10.254.0.1)",
				})
			}
		}
	}

	// 验证 network.subnet
	if cfg.Network.Subnet != "" && !ValidateSubnet(cfg.Network.Subnet) {
		errors = append(errors, ValidationError{
			Field:   "network.subnet",
			Value:   cfg.Network.Subnet,
			Message: "must be a valid CIDR subnet (e.g., 10.254.0.0/24)",
		})
	}

	return errors
}

// ValidateControllerConfig 验证 Controller 配置
// 返回所有验证错误的列表
func ValidateControllerConfig(cfg *ControllerConfig) []ValidationError {
	var errors []ValidationError

	// 验证 server.listen_address
	if cfg.Server.ListenAddress != "" && !ValidateListenAddress(cfg.Server.ListenAddress) {
		errors = append(errors, ValidationError{
			Field:   "server.listen_address",
			Value:   cfg.Server.ListenAddress,
			Message: "must be a valid IPv4 address (e.g., 0.0.0.0 or 192.168.1.1)",
		})
	}

	// 验证 server.port
	if cfg.Server.Port != 0 && !ValidatePort(cfg.Server.Port) {
		errors = append(errors, ValidationError{
			Field:   "server.port",
			Value:   fmt.Sprintf("%d", cfg.Server.Port),
			Message: "must be in range [1, 65535]",
		})
	}

	// 验证 algorithm.penalty_factor
	if cfg.Algorithm.PenaltyFactor < 0 {
		errors = append(errors, ValidationError{
			Field:   "algorithm.penalty_factor",
			Value:   fmt.Sprintf("%f", cfg.Algorithm.PenaltyFactor),
			Message: "must be non-negative",
		})
	}

	// 验证 algorithm.hysteresis
	if cfg.Algorithm.Hysteresis < 0 || cfg.Algorithm.Hysteresis > 1 {
		errors = append(errors, ValidationError{
			Field:   "algorithm.hysteresis",
			Value:   fmt.Sprintf("%f", cfg.Algorithm.Hysteresis),
			Message: "must be in range [0, 1]",
		})
	}

	// 验证 logging.level
	validLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}
	if cfg.Logging.Level != "" && !validLevels[strings.ToUpper(cfg.Logging.Level)] {
		errors = append(errors, ValidationError{
			Field:   "logging.level",
			Value:   cfg.Logging.Level,
			Message: "must be one of: DEBUG, INFO, WARN, ERROR",
		})
	}

	return errors
}

// FormatValidationErrors 格式化验证错误为可读字符串
func FormatValidationErrors(errors []ValidationError) string {
	if len(errors) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("configuration validation failed:\n")
	for _, err := range errors {
		sb.WriteString(fmt.Sprintf("  - %s: %s (got: '%s')\n", err.Field, err.Message, err.Value))
	}
	return sb.String()
}
