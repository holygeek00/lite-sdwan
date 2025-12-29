// Package controller 实现 SD-WAN Controller 功能
package controller

import (
	"sync"
	"time"

	"github.com/example/lite-sdwan/pkg/models"
)

// TopologyDB 拓扑数据库，存储所有 Agent 的遥测数据
type TopologyDB struct {
	mu   sync.RWMutex
	data map[string]*models.AgentData // agent_id -> data
}

// NewTopologyDB 创建新的拓扑数据库
func NewTopologyDB() *TopologyDB {
	return &TopologyDB{
		data: make(map[string]*models.AgentData),
	}
}

// Store 存储 Agent 的遥测数据
func (db *TopologyDB) Store(req *models.TelemetryRequest) {
	db.mu.Lock()
	defer db.mu.Unlock()

	metrics := make(map[string]*models.MetricData)
	for _, m := range req.Metrics {
		metrics[m.TargetIP] = &models.MetricData{
			RTT:  m.RTTMs,
			Loss: m.LossRate,
		}
	}

	db.data[req.AgentID] = &models.AgentData{
		Timestamp: time.Unix(req.Timestamp, 0),
		Metrics:   metrics,
	}
}

// Get 获取指定 Agent 的数据
func (db *TopologyDB) Get(agentID string) (*models.AgentData, bool) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	data, ok := db.data[agentID]
	return data, ok
}

// GetAll 获取所有 Agent 的数据
func (db *TopologyDB) GetAll() map[string]*models.AgentData {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// 返回副本
	result := make(map[string]*models.AgentData)
	for k, v := range db.data {
		result[k] = v
	}
	return result
}

// Count 返回 Agent 数量
func (db *TopologyDB) Count() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.data)
}

// Exists 检查 Agent 是否存在
func (db *TopologyDB) Exists(agentID string) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	_, ok := db.data[agentID]
	return ok
}

// GetAllAgentIDs 获取所有 Agent ID
func (db *TopologyDB) GetAllAgentIDs() []string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	ids := make([]string, 0, len(db.data))
	for id := range db.data {
		ids = append(ids, id)
	}
	return ids
}

// CleanStale 清理过期数据
func (db *TopologyDB) CleanStale(threshold time.Duration) int {
	db.mu.Lock()
	defer db.mu.Unlock()

	now := time.Now()
	count := 0
	for id, data := range db.data {
		if now.Sub(data.Timestamp) > threshold {
			delete(db.data, id)
			count++
		}
	}
	return count
}
