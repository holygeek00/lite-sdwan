package controller

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/holygeek00/lite-sdwan/pkg/config"
	"github.com/holygeek00/lite-sdwan/pkg/models"
)

// Server Controller HTTP 服务器
type Server struct {
	cfg    *config.ControllerConfig
	db     *TopologyDB
	solver *RouteSolver
	router *gin.Engine
}

// NewServer 创建新的 Controller 服务器
func NewServer(cfg *config.ControllerConfig) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		cfg:    cfg,
		db:     NewTopologyDB(),
		solver: NewRouteSolver(cfg.Algorithm.PenaltyFactor, cfg.Algorithm.Hysteresis),
		router: gin.New(),
	}

	s.setupRoutes()
	return s
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	s.router.Use(gin.Recovery())
	s.router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		return fmt.Sprintf("[%s] %s %s %d %s\n",
			param.TimeStamp.Format(time.RFC3339),
			param.Method,
			param.Path,
			param.StatusCode,
			param.Latency,
		)
	}))

	// API v1
	v1 := s.router.Group("/api/v1")
	{
		v1.POST("/telemetry", s.handleTelemetry)
		v1.GET("/routes", s.handleGetRoutes)
	}

	// 健康检查
	s.router.GET("/health", s.handleHealth)
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

	log.Printf("Received telemetry from agent %s with %d metrics", req.AgentID, len(req.Metrics))

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

	log.Printf("Computed %d routes for agent %s", len(routes), agentID)

	c.JSON(http.StatusOK, models.RouteResponse{Routes: routes})
}

// handleHealth 处理健康检查
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, models.HealthResponse{
		Status:     "healthy",
		AgentCount: s.db.Count(),
	})
}

// Run 启动服务器
func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.ListenAddress, s.cfg.Server.Port)
	log.Printf("Controller starting on %s", addr)
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
