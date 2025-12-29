package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/holygeek00/lite-sdwan/pkg/models"
)

// Client Controller HTTP 客户端
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient 创建新的客户端
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// SendTelemetry 发送遥测数据
func (c *Client) SendTelemetry(req *models.TelemetryRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal telemetry: %w", err)
	}

	url := c.baseURL + "/api/v1/telemetry"
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to send telemetry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telemetry request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetRoutes 获取路由配置
func (c *Client) GetRoutes(agentID string) (*models.RouteResponse, error) {
	url := fmt.Sprintf("%s/api/v1/routes?agent_id=%s", c.baseURL, agentID)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get routes: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, models.ErrAgentNotFound
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("routes request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var routes models.RouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		return nil, fmt.Errorf("failed to decode routes: %w", err)
	}

	return &routes, nil
}

// CheckHealth 检查 Controller 健康状态
func (c *Client) CheckHealth() error {
	url := c.baseURL + "/health"
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// RetryClient 带重试的客户端
type RetryClient struct {
	client       *Client
	maxRetries   int
	backoffSecs  []int
	failureCount int
	inFallback   bool
}

// NewRetryClient 创建带重试的客户端
func NewRetryClient(baseURL string, timeout time.Duration, maxRetries int, backoffSecs []int) *RetryClient {
	return &RetryClient{
		client:      NewClient(baseURL, timeout),
		maxRetries:  maxRetries,
		backoffSecs: backoffSecs,
	}
}

// SendTelemetryWithRetry 带重试的发送遥测数据
func (rc *RetryClient) SendTelemetryWithRetry(req *models.TelemetryRequest) error {
	var lastErr error

	for attempt := 0; attempt <= rc.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := rc.backoffSecs[min(attempt-1, len(rc.backoffSecs)-1)]
			log.Printf("Retrying telemetry in %d seconds (attempt %d/%d)", backoff, attempt, rc.maxRetries)
			time.Sleep(time.Duration(backoff) * time.Second)
		}

		err := rc.client.SendTelemetry(req)
		if err == nil {
			rc.failureCount = 0
			if rc.inFallback {
				log.Printf("Controller recovered, exiting fallback mode")
				rc.inFallback = false
			}
			return nil
		}

		lastErr = err
		log.Printf("Telemetry send failed: %v", err)
	}

	rc.failureCount++
	return lastErr
}

// GetRoutesWithRetry 带重试的获取路由
func (rc *RetryClient) GetRoutesWithRetry(agentID string) (*models.RouteResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= rc.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := rc.backoffSecs[min(attempt-1, len(rc.backoffSecs)-1)]
			log.Printf("Retrying get routes in %d seconds (attempt %d/%d)", backoff, attempt, rc.maxRetries)
			time.Sleep(time.Duration(backoff) * time.Second)
		}

		routes, err := rc.client.GetRoutes(agentID)
		if err == nil {
			rc.failureCount = 0
			if rc.inFallback {
				log.Printf("Controller recovered, exiting fallback mode")
				rc.inFallback = false
			}
			return routes, nil
		}

		lastErr = err
		log.Printf("Get routes failed: %v", err)
	}

	rc.failureCount++
	return nil, lastErr
}

// ShouldEnterFallback 检查是否应该进入 fallback 模式
func (rc *RetryClient) ShouldEnterFallback() bool {
	return rc.failureCount >= rc.maxRetries && !rc.inFallback
}

// EnterFallback 进入 fallback 模式
func (rc *RetryClient) EnterFallback() {
	rc.inFallback = true
	log.Printf("Entering fallback mode after %d consecutive failures", rc.failureCount)
}

// IsInFallback 检查是否在 fallback 模式
func (rc *RetryClient) IsInFallback() bool {
	return rc.inFallback
}

// ResetFailureCount 重置失败计数
func (rc *RetryClient) ResetFailureCount() {
	rc.failureCount = 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
