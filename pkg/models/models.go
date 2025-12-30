// Package models 定义 SD-WAN 系统的核心数据模型
package models

import (
	"encoding/json"
	"time"
)

// Metric 表示单个目标节点的探测指标
type Metric struct {
	TargetIP string   `json:"target_ip" yaml:"target_ip"`
	RTTMs    *float64 `json:"rtt_ms" yaml:"rtt_ms"`       // nil 表示超时
	LossRate float64  `json:"loss_rate" yaml:"loss_rate"` // 0.0 - 1.0
}

// TelemetryRequest 表示 Agent 上报的遥测数据
type TelemetryRequest struct {
	AgentID   string   `json:"agent_id" yaml:"agent_id"`
	Timestamp int64    `json:"timestamp" yaml:"timestamp"`
	Metrics   []Metric `json:"metrics" yaml:"metrics"`
}

// RouteConfig 表示单条路由配置
type RouteConfig struct {
	DstCIDR string `json:"dst_cidr" yaml:"dst_cidr"`
	NextHop string `json:"next_hop" yaml:"next_hop"` // IP 地址或 "direct"
	Reason  string `json:"reason" yaml:"reason"`     // "optimized_path" 或 "default"
}

// RouteResponse 表示路由查询响应
type RouteResponse struct {
	Routes []RouteConfig `json:"routes"`
}

// HealthResponse 表示健康检查响应
type HealthResponse struct {
	Status     string `json:"status"`
	AgentCount int    `json:"agent_count"`
}

// ErrorResponse 表示错误响应
type ErrorResponse struct {
	Detail string `json:"detail"`
}

// AgentData 表示存储在拓扑数据库中的 Agent 数据
type AgentData struct {
	Timestamp time.Time
	Metrics   map[string]*MetricData // target_ip -> metrics
}

// MetricData 表示存储的指标数据
type MetricData struct {
	RTT  *float64
	Loss float64
}

// ToJSON 将 TelemetryRequest 序列化为 JSON
func (t *TelemetryRequest) ToJSON() ([]byte, error) {
	return json.Marshal(t)
}

// FromJSON 从 JSON 反序列化 TelemetryRequest
func (t *TelemetryRequest) FromJSON(data []byte) error {
	return json.Unmarshal(data, t)
}

// Validate 验证 TelemetryRequest 的有效性
func (t *TelemetryRequest) Validate() error {
	if t.AgentID == "" {
		return ErrEmptyAgentID
	}
	if t.Timestamp <= 0 {
		return ErrInvalidTimestamp
	}
	if len(t.Metrics) == 0 {
		return ErrEmptyMetrics
	}
	for _, m := range t.Metrics {
		if err := m.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate 验证 Metric 的有效性
func (m *Metric) Validate() error {
	if m.TargetIP == "" {
		return ErrEmptyTargetIP
	}
	if m.RTTMs != nil && *m.RTTMs < 0 {
		return ErrNegativeRTT
	}
	if m.LossRate < 0 || m.LossRate > 1 {
		return ErrInvalidLossRate
	}
	return nil
}

// HealthStatus 健康状态常量
const (
	HealthStatusHealthy   = "healthy"
	HealthStatusDegraded  = "degraded"
	HealthStatusUnhealthy = "unhealthy"
)

// DetailedHealthResponse 详细健康响应
type DetailedHealthResponse struct {
	Status     string                     `json:"status"`
	Version    string                     `json:"version,omitempty"`
	Uptime     string                     `json:"uptime,omitempty"`
	Components map[string]ComponentHealth `json:"components"`
	Timestamp  string                     `json:"timestamp"`
}

// ComponentHealth 组件健康状态
type ComponentHealth struct {
	Status    string                 `json:"status"`
	Details   map[string]interface{} `json:"details,omitempty"`
	LastCheck string                 `json:"last_check"`
}

// IsHealthy 检查整体健康状态
func (d *DetailedHealthResponse) IsHealthy() bool {
	for _, comp := range d.Components {
		if comp.Status == HealthStatusUnhealthy {
			return false
		}
	}
	return true
}

// NewComponentHealth 创建健康的组件状态
func NewComponentHealth(status string) ComponentHealth {
	return ComponentHealth{
		Status:    status,
		Details:   make(map[string]interface{}),
		LastCheck: time.Now().Format(time.RFC3339),
	}
}

// NewDetailedHealthResponse 创建详细健康响应
func NewDetailedHealthResponse() *DetailedHealthResponse {
	return &DetailedHealthResponse{
		Status:     HealthStatusHealthy,
		Components: make(map[string]ComponentHealth),
		Timestamp:  time.Now().Format(time.RFC3339),
	}
}

// AddComponent 添加组件健康状态
func (d *DetailedHealthResponse) AddComponent(name string, health ComponentHealth) {
	d.Components[name] = health
	// 更新整体状态
	if health.Status == HealthStatusUnhealthy {
		d.Status = HealthStatusUnhealthy
	} else if health.Status == HealthStatusDegraded && d.Status == HealthStatusHealthy {
		d.Status = HealthStatusDegraded
	}
}
