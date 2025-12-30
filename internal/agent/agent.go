package agent

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/holygeek00/lite-sdwan/pkg/config"
	"github.com/holygeek00/lite-sdwan/pkg/logging"
	"github.com/holygeek00/lite-sdwan/pkg/models"
)

// Agent SD-WAN Agent 主程序
type Agent struct {
	cfg      *config.AgentConfig
	prober   *Prober
	executor *Executor
	client   *RetryClient
	logger   logging.Logger

	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
	inflight  int64 // 正在进行的请求数
	acceptNew int32 // 是否接受新的探测结果 (1=接受, 0=不接受)
}

// NewAgent 创建新的 Agent
func NewAgent(cfg *config.AgentConfig) (*Agent, error) {
	return NewAgentWithLogger(cfg, nil)
}

// NewAgentWithLogger 创建新的 Agent，使用指定的 Logger
func NewAgentWithLogger(cfg *config.AgentConfig, logger logging.Logger) (*Agent, error) {
	if logger == nil {
		logger = logging.NewJSONLoggerFromString(cfg.Logging.Level, nil)
	}

	executor, err := NewExecutorWithLogger(cfg.Network.WGInterface, cfg.Network.Subnet, logger)
	if err != nil {
		return nil, err
	}

	prober := NewProberWithLogger(
		cfg.Network.PeerIPs,
		cfg.Probe.Interval,
		cfg.Probe.Timeout,
		cfg.Probe.WindowSize,
		logger,
	)

	client := NewRetryClientWithLogger(
		cfg.Controller.URL,
		cfg.Controller.Timeout,
		cfg.Sync.RetryAttempts,
		cfg.Sync.RetryBackoff,
		logger,
	)

	return &Agent{
		cfg:       cfg,
		prober:    prober,
		executor:  executor,
		client:    client,
		logger:    logger,
		stopCh:    make(chan struct{}),
		acceptNew: 1, // 默认接受新的探测结果
	}, nil
}

// Start 启动 Agent
func (a *Agent) Start() {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return
	}
	a.running = true
	a.mu.Unlock()

	a.logger.Info("Agent starting", logging.F("agent_id", a.cfg.AgentID))

	// 启动探测器
	a.prober.Start()

	// 启动遥测上报协程
	a.wg.Add(1)
	go a.telemetryLoop()

	// 启动路由同步协程
	a.wg.Add(1)
	go a.syncLoop()

	a.logger.Info("Agent started", logging.F("agent_id", a.cfg.AgentID))
}

// telemetryLoop 遥测上报循环
func (a *Agent) telemetryLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.cfg.Sync.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.sendTelemetry()
		case <-a.stopCh:
			return
		}
	}
}

// sendTelemetry 发送遥测数据
func (a *Agent) sendTelemetry() {
	metrics := a.prober.GetMetrics()
	if len(metrics) == 0 {
		a.logger.Debug("No metrics to send")
		return
	}

	req := &models.TelemetryRequest{
		AgentID:   a.cfg.AgentID,
		Timestamp: time.Now().Unix(),
		Metrics:   metrics,
	}

	err := a.client.SendTelemetryWithRetry(req)
	if err != nil {
		a.logger.Error("Failed to send telemetry",
			logging.F("error", err.Error()),
			logging.F("agent_id", a.cfg.AgentID),
		)

		if a.client.ShouldEnterFallback() {
			a.enterFallback()
		}
	}
}

// syncLoop 路由同步循环
func (a *Agent) syncLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(a.cfg.Sync.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.syncRoutes()
		case <-a.stopCh:
			return
		}
	}
}

// syncRoutes 同步路由
func (a *Agent) syncRoutes() {
	if a.client.IsInFallback() {
		// 在 fallback 模式下，尝试恢复连接
		if err := a.client.client.CheckHealth(); err == nil {
			a.logger.Info("Controller recovered, exiting fallback mode")
			a.client.ResetFailureCount()
		}
		return
	}

	routes, err := a.client.GetRoutesWithRetry(a.cfg.AgentID)
	if err != nil {
		a.logger.Error("Failed to get routes",
			logging.F("error", err.Error()),
			logging.F("agent_id", a.cfg.AgentID),
		)

		if a.client.ShouldEnterFallback() {
			a.enterFallback()
		}
		return
	}

	if len(routes.Routes) > 0 {
		a.logger.Info("Received routes from controller",
			logging.F("route_count", len(routes.Routes)),
			logging.F("agent_id", a.cfg.AgentID),
		)
		if syncErr := a.executor.SyncRoutes(routes.Routes); syncErr != nil {
			a.logger.Error("Failed to sync routes",
				logging.F("error", syncErr.Error()),
			)
		}
	}
}

// enterFallback 进入 fallback 模式
func (a *Agent) enterFallback() {
	a.client.EnterFallback()
	a.logger.Warn("Entering fallback mode, flushing routes")

	if flushErr := a.executor.FlushRoutes(); flushErr != nil {
		a.logger.Error("Failed to flush routes",
			logging.F("error", flushErr.Error()),
		)
	}
}

// Stop 停止 Agent
func (a *Agent) Stop() {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	a.running = false
	a.mu.Unlock()

	a.logger.Info("Agent stopping", logging.F("agent_id", a.cfg.AgentID))

	// 停止探测器
	a.prober.Stop()

	// 停止协程
	close(a.stopCh)
	a.wg.Wait()

	a.logger.Info("Agent stopped", logging.F("agent_id", a.cfg.AgentID))
}

// Shutdown 优雅关闭 Agent
// 按顺序执行：停止接受新请求 -> 等待进行中的请求完成 -> 清理路由 -> 停止服务
func (a *Agent) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return nil
	}
	a.running = false
	a.mu.Unlock()

	a.logger.Info("Agent initiating graceful shutdown", logging.F("agent_id", a.cfg.AgentID))

	// 1. 停止接受新的探测结果
	atomic.StoreInt32(&a.acceptNew, 0)
	a.logger.Info("Stopped accepting new probe results")

	// 2. 停止探测器
	a.prober.Stop()

	// 3. 停止协程
	close(a.stopCh)
	a.wg.Wait()

	// 4. 等待进行中的请求完成
	if err := a.waitForInflight(ctx); err != nil {
		a.logger.Warn("Timeout waiting for in-flight requests",
			logging.F("error", err.Error()),
		)
	}

	// 5. 清理路由
	if err := a.cleanupRoutes(); err != nil {
		a.logger.Warn("Route cleanup encountered errors",
			logging.F("error", err.Error()),
		)
		// 继续执行其他清理任务，不返回错误
	}

	a.logger.Info("Agent shutdown complete", logging.F("agent_id", a.cfg.AgentID))
	return nil
}

// cleanupRoutes 清理由 Agent 添加的所有路由
func (a *Agent) cleanupRoutes() error {
	a.logger.Info("Cleaning up managed routes")

	cleaned, errors := a.executor.CleanupManagedRoutes()

	if len(errors) > 0 {
		for _, err := range errors {
			a.logger.Error("Route cleanup error", logging.F("error", err.Error()))
		}
		a.logger.Info("Route cleanup completed with errors",
			logging.F("error_count", len(errors)),
			logging.F("cleaned_count", cleaned),
		)
		return errors[0] // 返回第一个错误
	}

	a.logger.Info("Successfully cleaned up managed routes",
		logging.F("cleaned_count", cleaned),
	)
	return nil
}

// waitForInflight 等待进行中的请求完成
func (a *Agent) waitForInflight(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			remaining := atomic.LoadInt64(&a.inflight)
			if remaining > 0 {
				a.logger.Warn("Shutdown timeout with in-flight requests remaining",
					logging.F("remaining", remaining),
				)
			}
			return ctx.Err()
		case <-ticker.C:
			if atomic.LoadInt64(&a.inflight) == 0 {
				a.logger.Info("All in-flight requests completed")
				return nil
			}
		}
	}
}

// IsAcceptingNew 检查是否接受新的探测结果
func (a *Agent) IsAcceptingNew() bool {
	return atomic.LoadInt32(&a.acceptNew) == 1
}

// IncrementInflight 增加进行中的请求计数
func (a *Agent) IncrementInflight() {
	atomic.AddInt64(&a.inflight, 1)
}

// DecrementInflight 减少进行中的请求计数
func (a *Agent) DecrementInflight() {
	atomic.AddInt64(&a.inflight, -1)
}

// DefaultShutdownTimeout 默认关闭超时时间
const DefaultShutdownTimeout = 30 * time.Second

// Run 运行 Agent（阻塞直到收到信号）
func (a *Agent) Run() {
	a.RunWithTimeout(DefaultShutdownTimeout)
}

// RunWithTimeout 运行 Agent，使用指定的关闭超时时间
func (a *Agent) RunWithTimeout(shutdownTimeout time.Duration) {
	a.Start()

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	a.logger.Info("Received signal, initiating graceful shutdown",
		logging.F("signal", sig.String()),
	)

	// 创建带超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// 执行优雅关闭
	if err := a.Shutdown(ctx); err != nil {
		a.logger.Warn("Graceful shutdown completed with error",
			logging.F("error", err.Error()),
		)
	} else {
		a.logger.Info("Graceful shutdown completed successfully")
	}
}

// GetHealthStatus 获取 Agent 健康状态
func (a *Agent) GetHealthStatus() *models.DetailedHealthResponse {
	resp := models.NewDetailedHealthResponse()

	// Prober 状态
	proberHealth := models.NewComponentHealth(models.HealthStatusHealthy)
	if a.prober != nil {
		proberHealth.Details["running"] = a.prober.IsRunning()
		proberHealth.Details["success_rate"] = a.prober.GetSuccessRate()
		if lastProbe := a.prober.GetLastProbeTime(); lastProbe != nil {
			proberHealth.Details["last_probe_time"] = lastProbe.Format(time.RFC3339)
		} else {
			proberHealth.Details["last_probe_time"] = nil
		}

		// 如果探测器未运行，标记为不健康
		if !a.prober.IsRunning() {
			proberHealth.Status = models.HealthStatusUnhealthy
		}
	} else {
		proberHealth.Status = models.HealthStatusUnhealthy
		proberHealth.Details["error"] = "prober not initialized"
	}
	resp.AddComponent("prober", proberHealth)

	// Controller 连接状态
	controllerHealth := models.NewComponentHealth(models.HealthStatusHealthy)
	if a.client != nil {
		inFallback := a.client.IsInFallback()
		controllerHealth.Details["in_fallback"] = inFallback
		controllerHealth.Details["controller_url"] = a.cfg.Controller.URL

		// 如果在 fallback 模式，标记为降级
		if inFallback {
			controllerHealth.Status = models.HealthStatusDegraded
		}
	} else {
		controllerHealth.Status = models.HealthStatusUnhealthy
		controllerHealth.Details["error"] = "client not initialized"
	}
	resp.AddComponent("controller_connection", controllerHealth)

	return resp
}
