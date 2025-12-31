package controller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/holygeek00/lite-sdwan/pkg/config"
	"github.com/holygeek00/lite-sdwan/pkg/logging"
	"github.com/holygeek00/lite-sdwan/pkg/models"
)

// Default cleaner interval
const defaultCleanerInterval = 60 * time.Second

// Server Controller HTTP 服务器
type Server struct {
	cfg     *config.ControllerConfig
	db      *TopologyDB
	solver  *RouteSolver
	router  *gin.Engine
	cleaner *StaleDataCleaner
	logger  logging.Logger
}

// NewServer 创建新的 Controller 服务器
func NewServer(cfg *config.ControllerConfig) *Server {
	gin.SetMode(gin.ReleaseMode)

	// 创建 logger
	logger := logging.NewJSONLoggerFromString(cfg.Logging.Level, nil)

	s := &Server{
		cfg:    cfg,
		db:     NewTopologyDB(),
		solver: NewRouteSolver(cfg.Algorithm.PenaltyFactor, cfg.Algorithm.Hysteresis),
		router: gin.New(),
		logger: logger,
	}

	// 创建并启动陈旧数据清理器
	s.cleaner = NewStaleDataCleaner(
		s.db,
		cfg.Topology.StaleThreshold,
		defaultCleanerInterval,
		logger,
	)
	s.cleaner.Start()

	s.setupRoutes()
	return s
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	s.router.Use(gin.Recovery())
	s.router.Use(s.loggingMiddleware())

	// API v1
	v1 := s.router.Group("/api/v1")
	{
		v1.POST("/telemetry", s.handleTelemetry)
		v1.GET("/routes", s.handleGetRoutes)
		v1.GET("/topology", s.handleTopology)
	}

	// 健康检查
	s.router.GET("/health", s.handleHealth)
}

// loggingMiddleware 返回结构化日志中间件
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		duration := time.Since(start)
		s.logger.Info("HTTP request",
			logging.F("method", c.Request.Method),
			logging.F("path", path),
			logging.F("status", c.Writer.Status()),
			logging.F("duration_ms", float64(duration.Microseconds())/1000.0),
			logging.F("client_ip", c.ClientIP()),
		)
	}
}

// handleTelemetry 处理遥测数据上报
func (s *Server) handleTelemetry(c *gin.Context) {
	var req models.TelemetryRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Detail: fmt.Sprintf("Invalid JSON: %v", err),
		})
		return
	}

	if err := req.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Detail: err.Error(),
		})
		return
	}

	// 存储数据
	s.db.Store(&req)

	s.logger.Info("Received telemetry",
		logging.F("agent_id", req.AgentID),
		logging.F("metric_count", len(req.Metrics)),
	)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleGetRoutes 处理路由查询
func (s *Server) handleGetRoutes(c *gin.Context) {
	agentID := c.Query("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{
			Detail: "agent_id query parameter is required",
		})
		return
	}

	if !s.db.Exists(agentID) {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Detail: "Agent not found. Has it sent telemetry?",
		})
		return
	}

	routes := s.solver.ComputeRoutes(s.db, agentID)
	if routes == nil {
		routes = []models.RouteConfig{}
	}

	s.logger.Info("Computed routes",
		logging.F("agent_id", agentID),
		logging.F("route_count", len(routes)),
	)

	c.JSON(http.StatusOK, models.RouteResponse{Routes: routes})
}

// handleHealth 处理健康检查
func (s *Server) handleHealth(c *gin.Context) {
	resp := models.NewDetailedHealthResponse()

	// TopologyDB 状态
	dbHealth := models.NewComponentHealth(models.HealthStatusHealthy)
	dbHealth.Details["node_count"] = s.db.Count()
	if lastUpdate := s.db.GetLastUpdateTime(); lastUpdate != nil {
		dbHealth.Details["last_update"] = lastUpdate.Format(time.RFC3339)
	} else {
		dbHealth.Details["last_update"] = nil
	}
	resp.AddComponent("topology_db", dbHealth)

	// Cleaner 状态
	cleanerHealth := models.NewComponentHealth(models.HealthStatusHealthy)
	cleanerHealth.Details["cleanup_count"] = s.cleaner.GetCleanupCount()
	resp.AddComponent("cleaner", cleanerHealth)

	// 根据整体状态返回 HTTP 状态码
	if resp.IsHealthy() {
		c.JSON(http.StatusOK, resp)
	} else {
		c.JSON(http.StatusServiceUnavailable, resp)
	}
}

// TopologyNode 拓扑节点信息
type TopologyNode struct {
	AgentID     string            `json:"agent_id"`
	LastSeen    string            `json:"last_seen"`
	Peers       map[string]Metric `json:"peers"`
}

// Metric 指标信息
type Metric struct {
	RTT  float64 `json:"rtt_ms"`
	Loss float64 `json:"loss_rate"`
}

// TopologyResponse 拓扑响应
type TopologyResponse struct {
	NodeCount int            `json:"node_count"`
	Nodes     []TopologyNode `json:"nodes"`
}

// handleTopology 处理拓扑查询
func (s *Server) handleTopology(c *gin.Context) {
	allData := s.db.GetAll()
	
	nodes := make([]TopologyNode, 0, len(allData))
	for agentID, data := range allData {
		peers := make(map[string]Metric)
		for targetIP, metric := range data.Metrics {
			rtt := 0.0
			if metric.RTT != nil {
				rtt = *metric.RTT
			}
			peers[targetIP] = Metric{
				RTT:  rtt,
				Loss: metric.Loss,
			}
		}
		
		nodes = append(nodes, TopologyNode{
			AgentID:  agentID,
			LastSeen: data.Timestamp.Format(time.RFC3339),
			Peers:    peers,
		})
	}
	
	c.JSON(http.StatusOK, TopologyResponse{
		NodeCount: len(nodes),
		Nodes:     nodes,
	})
}

// Run 启动服务器
func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.ListenAddress, s.cfg.Server.Port)
	s.logger.Info("Controller starting",
		logging.F("address", addr),
	)
	return s.router.Run(addr)
}

// GetDB 获取拓扑数据库（用于测试）
func (s *Server) GetDB() *TopologyDB {
	return s.db
}

// GetSolver 获取路径计算引擎（用于测试）
func (s *Server) GetSolver() *RouteSolver {
	return s.solver
}

// Shutdown 关闭服务器，停止清理器
func (s *Server) Shutdown() {
	if s.cleaner != nil {
		s.cleaner.Stop()
	}
}

// GetCleaner 获取清理器（用于测试）
func (s *Server) GetCleaner() *StaleDataCleaner {
	return s.cleaner
}
