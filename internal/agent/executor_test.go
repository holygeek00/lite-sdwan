package agent

import (
	"strings"
	"testing"

	"github.com/holygeek00/lite-sdwan/pkg/models"
)

func TestValidateIP(t *testing.T) {
	executor, err := NewExecutor("wg0", "10.254.0.0/24")
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"10.254.0.1", true},
		{"10.254.0.255", true},
		{"10.254.1.1", false},
		{"192.168.1.1", false},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := executor.ValidateIP(tt.ip); got != tt.want {
				t.Errorf("ValidateIP(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestGenerateAddCommand(t *testing.T) {
	executor, _ := NewExecutor("wg0", "10.254.0.0/24")

	cmd := executor.GenerateAddCommand("10.254.0.2", "10.254.0.1")

	expected := []string{"ip", "route", "replace", "10.254.0.2/32", "via", "10.254.0.1", "dev", "wg0"}

	if len(cmd) != len(expected) {
		t.Errorf("Command length mismatch: got %d, want %d", len(cmd), len(expected))
	}

	for i, part := range expected {
		if cmd[i] != part {
			t.Errorf("Command part %d: got %s, want %s", i, cmd[i], part)
		}
	}
}

func TestGenerateDelCommand(t *testing.T) {
	executor, _ := NewExecutor("wg0", "10.254.0.0/24")

	cmd := executor.GenerateDelCommand("10.254.0.2")

	expected := []string{"ip", "route", "del", "10.254.0.2/32", "dev", "wg0"}

	if len(cmd) != len(expected) {
		t.Errorf("Command length mismatch: got %d, want %d", len(cmd), len(expected))
	}

	for i, part := range expected {
		if cmd[i] != part {
			t.Errorf("Command part %d: got %s, want %s", i, cmd[i], part)
		}
	}
}

func TestCalculateDiff(t *testing.T) {
	current := []CurrentRoute{
		{Destination: "10.254.0.2/32", NextHop: "10.254.0.1"},
		{Destination: "10.254.0.3/32", NextHop: "10.254.0.1"},
	}

	desired := []models.RouteConfig{
		{DstCIDR: "10.254.0.2/32", NextHop: "10.254.0.1", Reason: "optimized_path"}, // 不变
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.4", Reason: "optimized_path"}, // 修改
		{DstCIDR: "10.254.0.5/32", NextHop: "10.254.0.1", Reason: "optimized_path"}, // 新增
	}

	toAdd, toRemove := CalculateDiff(current, desired)

	// 应该有 2 个需要添加/修改（10.254.0.3 和 10.254.0.5）
	if len(toAdd) != 2 {
		t.Errorf("Expected 2 routes to add, got %d", len(toAdd))
	}

	// 没有需要删除的（因为 desired 中没有 direct）
	if len(toRemove) != 0 {
		t.Errorf("Expected 0 routes to remove, got %d", len(toRemove))
	}
}

func TestCalculateDiffWithDirect(t *testing.T) {
	current := []CurrentRoute{
		{Destination: "10.254.0.2/32", NextHop: "10.254.0.1"},
	}

	desired := []models.RouteConfig{
		{DstCIDR: "10.254.0.2/32", NextHop: "direct", Reason: "default"}, // 恢复直连
	}

	toAdd, toRemove := CalculateDiff(current, desired)

	// 没有需要添加的
	if len(toAdd) != 0 {
		t.Errorf("Expected 0 routes to add, got %d", len(toAdd))
	}

	// 应该有 1 个需要删除
	if len(toRemove) != 1 {
		t.Errorf("Expected 1 route to remove, got %d", len(toRemove))
	}
}

func TestNewExecutorInvalidSubnet(t *testing.T) {
	_, err := NewExecutor("wg0", "invalid")
	if err == nil {
		t.Error("Expected error for invalid subnet")
	}
	if !strings.Contains(err.Error(), "invalid subnet") {
		t.Errorf("Error should mention invalid subnet: %v", err)
	}
}
