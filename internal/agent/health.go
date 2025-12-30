// Package agent 实现 SD-WAN Agent 功能
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HealthServer Agent 健康检查 HTTP 服务器
type HealthServer struct {
	agent  *Agent
	server *http.Server
	port   int
}

// NewHealthServer 创建健康检查服务器
func NewHealthServer(agent *Agent, port int) *HealthServer {
	hs := &HealthServer{
		agent: agent,
		port:  port,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", hs.handleHealth)

	hs.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return hs
}

// Start 启动健康检查服务器
func (hs *HealthServer) Start() error {
	go func() {
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// 记录错误但不阻塞
			fmt.Printf("Health server error: %v\n", err)
		}
	}()
	return nil
}

// Stop 停止健康检查服务器
func (hs *HealthServer) Stop(ctx context.Context) error {
	return hs.server.Shutdown(ctx)
}

// handleHealth 处理健康检查请求
func (hs *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := hs.agent.GetHealthStatus()

	w.Header().Set("Content-Type", "application/json")

	if resp.IsHealthy() {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	_ = json.NewEncoder(w).Encode(resp)
}
