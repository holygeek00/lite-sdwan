package controller

import (
	"container/heap"
	"math"
	"sync"

	"github.com/holygeek00/lite-sdwan/pkg/models"
)

// RouteSolver 路径计算引擎
type RouteSolver struct {
	penaltyFactor float64
	hysteresis    float64
	mu            sync.RWMutex
	previousCosts map[string]float64 // "source->target" -> cost
}

// NewRouteSolver 创建新的路径计算引擎
func NewRouteSolver(penaltyFactor, hysteresis float64) *RouteSolver {
	return &RouteSolver{
		penaltyFactor: penaltyFactor,
		hysteresis:    hysteresis,
		previousCosts: make(map[string]float64),
	}
}

// Graph 表示网络拓扑图
type Graph struct {
	nodes map[string]bool
	edges map[string]map[string]float64 // source -> target -> cost
}

// NewGraph 创建新的图
func NewGraph() *Graph {
	return &Graph{
		nodes: make(map[string]bool),
		edges: make(map[string]map[string]float64),
	}
}

// AddNode 添加节点
func (g *Graph) AddNode(id string) {
	g.nodes[id] = true
	if g.edges[id] == nil {
		g.edges[id] = make(map[string]float64)
	}
}

// AddEdge 添加边
func (g *Graph) AddEdge(from, to string, cost float64) {
	g.AddNode(from)
	g.AddNode(to)
	g.edges[from][to] = cost
}

// CalculateCost 计算链路成本
// Cost = RTT_ms + (Loss_rate × PenaltyFactor)
func (s *RouteSolver) CalculateCost(rtt *float64, lossRate float64) float64 {
	if rtt == nil {
		return math.Inf(1) // 链路不可达
	}
	return *rtt + (lossRate * s.penaltyFactor)
}

// BuildGraph 从拓扑数据库构建图
func (s *RouteSolver) BuildGraph(db *TopologyDB) *Graph {
	g := NewGraph()
	allData := db.GetAll()

	// 添加所有节点
	for agentID := range allData {
		g.AddNode(agentID)
	}

	// 添加边
	for source, data := range allData {
		for target, metrics := range data.Metrics {
			cost := s.CalculateCost(metrics.RTT, metrics.Loss)
			g.AddEdge(source, target, cost)
		}
	}

	return g
}

// priorityQueue 用于 Dijkstra 算法的优先队列
type priorityQueue []*pqItem

type pqItem struct {
	node     string
	priority float64
	index    int
}

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].priority < pq[j].priority
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*pqItem)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

// DijkstraResult Dijkstra 算法结果
type DijkstraResult struct {
	Distances map[string]float64
	Previous  map[string]string
}

// Dijkstra 执行 Dijkstra 最短路径算法
func (g *Graph) Dijkstra(source string) *DijkstraResult {
	dist := make(map[string]float64)
	prev := make(map[string]string)

	// 初始化距离
	for node := range g.nodes {
		dist[node] = math.Inf(1)
	}
	dist[source] = 0

	// 优先队列
	pq := make(priorityQueue, 0)
	heap.Init(&pq)
	heap.Push(&pq, &pqItem{node: source, priority: 0})

	visited := make(map[string]bool)

	for pq.Len() > 0 {
		item := heap.Pop(&pq).(*pqItem)
		u := item.node

		if visited[u] {
			continue
		}
		visited[u] = true

		// 遍历邻居
		for v, cost := range g.edges[u] {
			if visited[v] {
				continue
			}
			alt := dist[u] + cost
			if alt < dist[v] {
				dist[v] = alt
				prev[v] = u
				heap.Push(&pq, &pqItem{node: v, priority: alt})
			}
		}
	}

	return &DijkstraResult{
		Distances: dist,
		Previous:  prev,
	}
}

// GetPath 从 Dijkstra 结果中获取路径
func (r *DijkstraResult) GetPath(target string) []string {
	if _, ok := r.Previous[target]; !ok && r.Distances[target] == math.Inf(1) {
		return nil // 不可达
	}

	path := []string{target}
	current := target
	for {
		prev, ok := r.Previous[current]
		if !ok {
			break
		}
		path = append([]string{prev}, path...)
		current = prev
	}
	return path
}

// ComputeRoutes 为指定 Agent 计算路由
func (s *RouteSolver) ComputeRoutes(db *TopologyDB, sourceAgent string) []models.RouteConfig {
	g := s.BuildGraph(db)

	// 检查源节点是否存在
	if !g.nodes[sourceAgent] {
		return nil
	}

	result := g.Dijkstra(sourceAgent)
	routes := make([]models.RouteConfig, 0)

	s.mu.Lock()
	defer s.mu.Unlock()

	for target := range g.nodes {
		if target == sourceAgent {
			continue
		}

		path := result.GetPath(target)
		if len(path) < 2 {
			continue // 不可达或就是自己
		}

		newCost := result.Distances[target]
		if math.IsInf(newCost, 1) {
			continue // 不可达
		}

		// 应用迟滞逻辑
		costKey := sourceAgent + "->" + target
		oldCost, exists := s.previousCosts[costKey]

		var nextHop string
		var reason string

		if len(path) == 2 {
			// 直连
			nextHop = "direct"
			reason = "default"
		} else {
			// 需要中继
			nextHop = path[1]
			reason = "optimized_path"
		}

		// 检查是否需要更新路由
		shouldUpdate := false
		if !exists {
			shouldUpdate = true
		} else if newCost < oldCost*(1-s.hysteresis) {
			// 新成本比旧成本低 15% 以上
			shouldUpdate = true
		}

		if shouldUpdate {
			s.previousCosts[costKey] = newCost
			routes = append(routes, models.RouteConfig{
				DstCIDR: target + "/32",
				NextHop: nextHop,
				Reason:  reason,
			})
		}
	}

	return routes
}

// HasLoop 检查路径是否有环
func HasLoop(path []string) bool {
	seen := make(map[string]bool)
	for _, node := range path {
		if seen[node] {
			return true
		}
		seen[node] = true
	}
	return false
}
