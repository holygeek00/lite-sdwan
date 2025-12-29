package agent

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/holygeek00/lite-sdwan/pkg/config"
	"github.com/holygeek00/lite-sdwan/pkg/models"
)

// Agent SD-WAN Agent 主程序
type Agent struct {
	cfg      *config.AgentConfig
	prober   *Prober
	executor *Executor
	client   *RetryClient

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewAgent 创建新的 Agent
func NewAgent(cfg *config.AgentConfig) (*Agent, error) {
	executor, err := NewExecutor(cfg.Network.WGInterface, cfg.Network.Subnet)
	if err != nil {
		return nil, err
	}

	prober := NewProber(
		cfg.Network.PeerIPs,
		cfg.Probe.Interval,
		cfg.Probe.Timeout,
		cfg.Probe.WindowSize,
	)

	client := NewRetryClient(
		cfg.Controller.URL,
		cfg.Controller.Timeout,
		cfg.Sync.RetryAttempts,
		cfg.Sync.RetryBackoff,
	)

	return &Agent{
		cfg:      cfg,
		prober:   prober,
		executor: executor,
		client:   client,
		stopCh:   make(chan struct{}),
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

	log.Printf("Agent %s starting...", a.cfg.AgentID)

	// 启动探测器
	a.prober.Start()

	// 启动遥测上报协程
	a.wg.Add(1)
	go a.telemetryLoop()

	// 启动路由同步协程
	a.wg.Add(1)
	go a.syncLoop()

	log.Printf("Agent %s started", a.cfg.AgentID)
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
		log.Printf("No metrics to send")
		return
	}

	req := &models.TelemetryRequest{
		AgentID:   a.cfg.AgentID,
		Timestamp: time.Now().Unix(),
		Metrics:   metrics,
	}

	err := a.client.SendTelemetryWithRetry(req)
	if err != nil {
		log.Printf("Failed to send telemetry: %v", err)

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
			log.Printf("Controller recovered, exiting fallback mode")
			a.client.ResetFailureCount()
		}
		return
	}

	routes, err := a.client.GetRoutesWithRetry(a.cfg.AgentID)
	if err != nil {
		log.Printf("Failed to get routes: %v", err)

		if a.client.ShouldEnterFallback() {
			a.enterFallback()
		}
		return
	}

	if len(routes.Routes) > 0 {
		log.Printf("Received %d routes from controller", len(routes.Routes))
		if syncErr := a.executor.SyncRoutes(routes.Routes); syncErr != nil {
			log.Printf("Failed to sync routes: %v", syncErr)
		}
	}
}

// enterFallback 进入 fallback 模式
func (a *Agent) enterFallback() {
	a.client.EnterFallback()
	log.Printf("Entering fallback mode, flushing routes...")

	if flushErr := a.executor.FlushRoutes(); flushErr != nil {
		log.Printf("Failed to flush routes: %v", flushErr)
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

	log.Printf("Agent %s stopping...", a.cfg.AgentID)

	// 停止探测器
	a.prober.Stop()

	// 停止协程
	close(a.stopCh)
	a.wg.Wait()

	log.Printf("Agent %s stopped", a.cfg.AgentID)
}

// Run 运行 Agent（阻塞直到收到信号）
func (a *Agent) Run() {
	a.Start()

	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	log.Printf("Received signal %v, shutting down...", sig)

	a.Stop()
}
