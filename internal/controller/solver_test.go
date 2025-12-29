package controller

import (
	"math"
	"testing"

	"github.com/holygeek00/lite-sdwan/pkg/models"
)

func TestCalculateCost(t *testing.T) {
	solver := NewRouteSolver(100, 0.15)

	tests := []struct {
		name     string
		rtt      *float64
		lossRate float64
		want     float64
	}{
		{
			name:     "normal link",
			rtt:      ptrFloat64(50.0),
			lossRate: 0.0,
			want:     50.0,
		},
		{
			name:     "link with loss",
			rtt:      ptrFloat64(50.0),
			lossRate: 0.1,
			want:     60.0, // 50 + 0.1 * 100
		},
		{
			name:     "link down",
			rtt:      nil,
			lossRate: 1.0,
			want:     math.Inf(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := solver.CalculateCost(tt.rtt, tt.lossRate)
			if math.IsInf(tt.want, 1) {
				if !math.IsInf(got, 1) {
					t.Errorf("CalculateCost() = %v, want Inf", got)
				}
			} else if got != tt.want {
				t.Errorf("CalculateCost() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDijkstra(t *testing.T) {
	g := NewGraph()

	// 构建测试图
	// A -> B (cost 10)
	// A -> C (cost 100)
	// B -> C (cost 10)
	g.AddEdge("A", "B", 10)
	g.AddEdge("A", "C", 100)
	g.AddEdge("B", "C", 10)

	result := g.Dijkstra("A")

	// 检查距离
	if result.Distances["A"] != 0 {
		t.Errorf("Distance to A = %v, want 0", result.Distances["A"])
	}
	if result.Distances["B"] != 10 {
		t.Errorf("Distance to B = %v, want 10", result.Distances["B"])
	}
	if result.Distances["C"] != 20 {
		t.Errorf("Distance to C = %v, want 20 (via B)", result.Distances["C"])
	}

	// 检查路径
	pathToC := result.GetPath("C")
	if len(pathToC) != 3 || pathToC[0] != "A" || pathToC[1] != "B" || pathToC[2] != "C" {
		t.Errorf("Path to C = %v, want [A B C]", pathToC)
	}
}

func TestDijkstraNoPath(t *testing.T) {
	g := NewGraph()
	g.AddNode("A")
	g.AddNode("B")
	// 没有边连接 A 和 B

	result := g.Dijkstra("A")

	if !math.IsInf(result.Distances["B"], 1) {
		t.Errorf("Distance to B should be Inf, got %v", result.Distances["B"])
	}

	path := result.GetPath("B")
	if path != nil {
		t.Errorf("Path to B should be nil, got %v", path)
	}
}

func TestHasLoop(t *testing.T) {
	tests := []struct {
		name string
		path []string
		want bool
	}{
		{
			name: "no loop",
			path: []string{"A", "B", "C"},
			want: false,
		},
		{
			name: "has loop",
			path: []string{"A", "B", "C", "A"},
			want: true,
		},
		{
			name: "empty path",
			path: []string{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasLoop(tt.path); got != tt.want {
				t.Errorf("HasLoop() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeRoutes(t *testing.T) {
	db := NewTopologyDB()
	solver := NewRouteSolver(100, 0.15)

	// 添加测试数据
	// A -> B: RTT 10ms, loss 0%
	// A -> C: RTT 100ms, loss 0%
	// B -> C: RTT 10ms, loss 0%
	db.Store(&models.TelemetryRequest{
		AgentID:   "A",
		Timestamp: 1000,
		Metrics: []models.Metric{
			{TargetIP: "B", RTTMs: ptrFloat64(10), LossRate: 0},
			{TargetIP: "C", RTTMs: ptrFloat64(100), LossRate: 0},
		},
	})
	db.Store(&models.TelemetryRequest{
		AgentID:   "B",
		Timestamp: 1000,
		Metrics: []models.Metric{
			{TargetIP: "A", RTTMs: ptrFloat64(10), LossRate: 0},
			{TargetIP: "C", RTTMs: ptrFloat64(10), LossRate: 0},
		},
	})
	db.Store(&models.TelemetryRequest{
		AgentID:   "C",
		Timestamp: 1000,
		Metrics: []models.Metric{
			{TargetIP: "A", RTTMs: ptrFloat64(100), LossRate: 0},
			{TargetIP: "B", RTTMs: ptrFloat64(10), LossRate: 0},
		},
	})

	// 从 A 计算路由
	routes := solver.ComputeRoutes(db, "A")

	// 应该有到 B 和 C 的路由
	if len(routes) < 1 {
		t.Errorf("Expected at least 1 route, got %d", len(routes))
	}

	// 检查到 C 的路由应该通过 B（因为 A->B->C = 20ms < A->C = 100ms）
	for _, r := range routes {
		if r.DstCIDR == "C/32" {
			if r.NextHop != "B" {
				t.Errorf("Route to C should go via B, got %s", r.NextHop)
			}
		}
	}
}

func TestHysteresis(t *testing.T) {
	solver := NewRouteSolver(100, 0.15)

	// 设置初始成本
	solver.previousCosts["A->B"] = 100

	// 新成本 90（只降低 10%），不应该触发更新
	// 因为需要降低 15% 以上
	newCost := 90.0
	oldCost := solver.previousCosts["A->B"]

	shouldUpdate := newCost < oldCost*(1-solver.hysteresis)
	if shouldUpdate {
		t.Errorf("Should not update: new cost %v is not 15%% lower than old cost %v", newCost, oldCost)
	}

	// 新成本 80（降低 20%），应该触发更新
	newCost = 80.0
	shouldUpdate = newCost < oldCost*(1-solver.hysteresis)
	if !shouldUpdate {
		t.Errorf("Should update: new cost %v is 20%% lower than old cost %v", newCost, oldCost)
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}
