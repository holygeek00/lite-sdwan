package controller

import (
	"testing"
	"time"

	"github.com/example/lite-sdwan/pkg/models"
)

func TestTopologyDBStore(t *testing.T) {
	db := NewTopologyDB()

	req := &models.TelemetryRequest{
		AgentID:   "10.254.0.1",
		Timestamp: time.Now().Unix(),
		Metrics: []models.Metric{
			{TargetIP: "10.254.0.2", RTTMs: ptrFloat64(10.5), LossRate: 0.0},
		},
	}

	db.Store(req)

	// 验证存储
	data, ok := db.Get("10.254.0.1")
	if !ok {
		t.Fatal("Agent not found after store")
	}

	if len(data.Metrics) != 1 {
		t.Errorf("Expected 1 metric, got %d", len(data.Metrics))
	}

	metric, ok := data.Metrics["10.254.0.2"]
	if !ok {
		t.Fatal("Metric for 10.254.0.2 not found")
	}

	if *metric.RTT != 10.5 {
		t.Errorf("RTT = %v, want 10.5", *metric.RTT)
	}
}

func TestTopologyDBCount(t *testing.T) {
	db := NewTopologyDB()

	if db.Count() != 0 {
		t.Errorf("Initial count should be 0")
	}

	db.Store(&models.TelemetryRequest{
		AgentID:   "agent1",
		Timestamp: time.Now().Unix(),
		Metrics:   []models.Metric{{TargetIP: "10.0.0.1", LossRate: 0}},
	})

	if db.Count() != 1 {
		t.Errorf("Count should be 1 after store")
	}

	db.Store(&models.TelemetryRequest{
		AgentID:   "agent2",
		Timestamp: time.Now().Unix(),
		Metrics:   []models.Metric{{TargetIP: "10.0.0.1", LossRate: 0}},
	})

	if db.Count() != 2 {
		t.Errorf("Count should be 2 after second store")
	}
}

func TestTopologyDBExists(t *testing.T) {
	db := NewTopologyDB()

	if db.Exists("agent1") {
		t.Error("Agent should not exist initially")
	}

	db.Store(&models.TelemetryRequest{
		AgentID:   "agent1",
		Timestamp: time.Now().Unix(),
		Metrics:   []models.Metric{{TargetIP: "10.0.0.1", LossRate: 0}},
	})

	if !db.Exists("agent1") {
		t.Error("Agent should exist after store")
	}
}

func TestTopologyDBGetAll(t *testing.T) {
	db := NewTopologyDB()

	db.Store(&models.TelemetryRequest{
		AgentID:   "agent1",
		Timestamp: time.Now().Unix(),
		Metrics:   []models.Metric{{TargetIP: "10.0.0.1", LossRate: 0}},
	})
	db.Store(&models.TelemetryRequest{
		AgentID:   "agent2",
		Timestamp: time.Now().Unix(),
		Metrics:   []models.Metric{{TargetIP: "10.0.0.1", LossRate: 0}},
	})

	all := db.GetAll()
	if len(all) != 2 {
		t.Errorf("GetAll should return 2 agents, got %d", len(all))
	}
}

func TestTopologyDBCleanStale(t *testing.T) {
	db := NewTopologyDB()

	// 添加一个旧数据
	db.Store(&models.TelemetryRequest{
		AgentID:   "old_agent",
		Timestamp: time.Now().Add(-2 * time.Hour).Unix(),
		Metrics:   []models.Metric{{TargetIP: "10.0.0.1", LossRate: 0}},
	})

	// 添加一个新数据
	db.Store(&models.TelemetryRequest{
		AgentID:   "new_agent",
		Timestamp: time.Now().Unix(),
		Metrics:   []models.Metric{{TargetIP: "10.0.0.1", LossRate: 0}},
	})

	if db.Count() != 2 {
		t.Errorf("Should have 2 agents before cleanup")
	}

	// 清理 1 小时前的数据
	cleaned := db.CleanStale(1 * time.Hour)

	if cleaned != 1 {
		t.Errorf("Should have cleaned 1 agent, cleaned %d", cleaned)
	}

	if db.Count() != 1 {
		t.Errorf("Should have 1 agent after cleanup")
	}

	if !db.Exists("new_agent") {
		t.Error("new_agent should still exist")
	}

	if db.Exists("old_agent") {
		t.Error("old_agent should be cleaned")
	}
}

func TestTopologyDBGetAllAgentIDs(t *testing.T) {
	db := NewTopologyDB()

	db.Store(&models.TelemetryRequest{
		AgentID:   "agent1",
		Timestamp: time.Now().Unix(),
		Metrics:   []models.Metric{{TargetIP: "10.0.0.1", LossRate: 0}},
	})
	db.Store(&models.TelemetryRequest{
		AgentID:   "agent2",
		Timestamp: time.Now().Unix(),
		Metrics:   []models.Metric{{TargetIP: "10.0.0.1", LossRate: 0}},
	})

	ids := db.GetAllAgentIDs()
	if len(ids) != 2 {
		t.Errorf("Should have 2 agent IDs, got %d", len(ids))
	}

	// 检查是否包含两个 ID
	hasAgent1 := false
	hasAgent2 := false
	for _, id := range ids {
		if id == "agent1" {
			hasAgent1 = true
		}
		if id == "agent2" {
			hasAgent2 = true
		}
	}

	if !hasAgent1 || !hasAgent2 {
		t.Errorf("Missing agent IDs: %v", ids)
	}
}
