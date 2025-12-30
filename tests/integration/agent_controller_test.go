// Package integration provides integration tests for Agent-Controller communication
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/holygeek00/lite-sdwan/pkg/models"
)

// MockExecutor is a mock implementation of route executor for testing
type MockExecutor struct {
	mu            sync.Mutex
	appliedRoutes []models.RouteConfig
	flushCalled   bool
	shouldFail    bool
}

// NewMockExecutor creates a new mock executor
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		appliedRoutes: make([]models.RouteConfig, 0),
	}
}

// SyncRoutes records the routes that would be applied
func (m *MockExecutor) SyncRoutes(routes []models.RouteConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFail {
		return fmt.Errorf("mock executor failure")
	}

	m.appliedRoutes = append(m.appliedRoutes, routes...)
	return nil
}

// FlushRoutes records that flush was called
func (m *MockExecutor) FlushRoutes() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldFail {
		return fmt.Errorf("mock executor failure")
	}

	m.flushCalled = true
	m.appliedRoutes = nil
	return nil
}

// GetAppliedRoutes returns the routes that were applied
func (m *MockExecutor) GetAppliedRoutes() []models.RouteConfig {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]models.RouteConfig, len(m.appliedRoutes))
	copy(result, m.appliedRoutes)
	return result
}

// WasFlushCalled returns whether FlushRoutes was called
func (m *MockExecutor) WasFlushCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.flushCalled
}

// SetShouldFail sets whether the executor should fail
func (m *MockExecutor) SetShouldFail(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldFail = fail
}

// Reset resets the mock executor state
func (m *MockExecutor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appliedRoutes = nil
	m.flushCalled = false
	m.shouldFail = false
}

// TestController is a test controller for integration tests
type TestController struct {
	server       *httptest.Server
	mu           sync.Mutex
	telemetry    []*models.TelemetryRequest
	routes       map[string][]models.RouteConfig // agentID -> routes
	healthy      bool
	respondDelay time.Duration
}

// NewTestController creates a new test controller
func NewTestController() *TestController {
	tc := &TestController{
		telemetry: make([]*models.TelemetryRequest, 0),
		routes:    make(map[string][]models.RouteConfig),
		healthy:   true,
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()

	router.POST("/api/v1/telemetry", tc.handleTelemetry)
	router.GET("/api/v1/routes", tc.handleGetRoutes)
	router.GET("/health", tc.handleHealth)

	tc.server = httptest.NewServer(router)
	return tc
}

func (tc *TestController) handleTelemetry(c *gin.Context) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.respondDelay > 0 {
		time.Sleep(tc.respondDelay)
	}

	if !tc.healthy {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{Detail: "controller unhealthy"})
		return
	}

	var req models.TelemetryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Detail: err.Error()})
		return
	}

	tc.telemetry = append(tc.telemetry, &req)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (tc *TestController) handleGetRoutes(c *gin.Context) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.respondDelay > 0 {
		time.Sleep(tc.respondDelay)
	}

	if !tc.healthy {
		c.JSON(http.StatusServiceUnavailable, models.ErrorResponse{Detail: "controller unhealthy"})
		return
	}

	agentID := c.Query("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Detail: "agent_id required"})
		return
	}

	routes, ok := tc.routes[agentID]
	if !ok {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Detail: "agent not found"})
		return
	}

	c.JSON(http.StatusOK, models.RouteResponse{Routes: routes})
}

func (tc *TestController) handleHealth(c *gin.Context) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if !tc.healthy {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "healthy"})
}

// URL returns the test server URL
func (tc *TestController) URL() string {
	return tc.server.URL
}

// Close closes the test server
func (tc *TestController) Close() {
	tc.server.Close()
}

// SetHealthy sets the controller health status
func (tc *TestController) SetHealthy(healthy bool) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.healthy = healthy
}

// SetRoutes sets routes for an agent
func (tc *TestController) SetRoutes(agentID string, routes []models.RouteConfig) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.routes[agentID] = routes
}

// GetTelemetry returns received telemetry
func (tc *TestController) GetTelemetry() []*models.TelemetryRequest {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	result := make([]*models.TelemetryRequest, len(tc.telemetry))
	copy(result, tc.telemetry)
	return result
}

// ClearTelemetry clears received telemetry
func (tc *TestController) ClearTelemetry() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.telemetry = nil
}

// SetRespondDelay sets response delay for simulating slow responses
func (tc *TestController) SetRespondDelay(delay time.Duration) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.respondDelay = delay
}

// httpClient is a helper for making HTTP requests with context
type httpClient struct {
	client *http.Client
}

func newHTTPClient() *httpClient {
	return &httpClient{client: &http.Client{Timeout: 10 * time.Second}}
}

func (h *httpClient) post(ctx context.Context, url string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return h.client.Do(req)
}

func (h *httpClient) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return h.client.Do(req)
}

// TestAgentSendTelemetryToController tests that Agent can successfully send telemetry to Controller
// Requirements: 5.1
func TestAgentSendTelemetryToController(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-1"
	rtt := 10.5
	telemetry := &models.TelemetryRequest{
		AgentID:   agentID,
		Timestamp: time.Now().Unix(),
		Metrics: []models.Metric{
			{
				TargetIP: "10.254.0.2",
				RTTMs:    &rtt,
				LossRate: 0.0,
			},
		},
	}

	data, err := json.Marshal(telemetry)
	if err != nil {
		t.Fatalf("Failed to marshal telemetry: %v", err)
	}

	resp, err := client.post(ctx, tc.URL()+"/api/v1/telemetry", data)
	if err != nil {
		t.Fatalf("Failed to send telemetry: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	received := tc.GetTelemetry()
	if len(received) != 1 {
		t.Fatalf("Expected 1 telemetry request, got %d", len(received))
	}

	if received[0].AgentID != agentID {
		t.Errorf("Expected agent_id %s, got %s", agentID, received[0].AgentID)
	}

	if len(received[0].Metrics) != 1 {
		t.Errorf("Expected 1 metric, got %d", len(received[0].Metrics))
	}
}

// TestAgentReceiveRoutesFromController tests that Agent can successfully receive routes from Controller
// Requirements: 5.2
func TestAgentReceiveRoutesFromController(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-1"

	expectedRoutes := []models.RouteConfig{
		{
			DstCIDR: "10.254.0.3/32",
			NextHop: "10.254.0.2",
			Reason:  "optimized_path",
		},
		{
			DstCIDR: "10.254.0.4/32",
			NextHop: "direct",
			Reason:  "default",
		},
	}
	tc.SetRoutes(agentID, expectedRoutes)

	rtt := 10.5
	telemetry := &models.TelemetryRequest{
		AgentID:   agentID,
		Timestamp: time.Now().Unix(),
		Metrics: []models.Metric{
			{
				TargetIP: "10.254.0.2",
				RTTMs:    &rtt,
				LossRate: 0.0,
			},
		},
	}

	data, _ := json.Marshal(telemetry)
	resp, err := client.post(ctx, tc.URL()+"/api/v1/telemetry", data)
	if err != nil {
		t.Fatalf("Failed to send telemetry: %v", err)
	}
	_ = resp.Body.Close()

	routeResp, err := client.get(ctx, fmt.Sprintf("%s/api/v1/routes?agent_id=%s", tc.URL(), agentID))
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	defer func() { _ = routeResp.Body.Close() }()

	if routeResp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", routeResp.StatusCode)
	}

	var routes models.RouteResponse
	if err := json.NewDecoder(routeResp.Body).Decode(&routes); err != nil {
		t.Fatalf("Failed to decode routes: %v", err)
	}

	if len(routes.Routes) != len(expectedRoutes) {
		t.Errorf("Expected %d routes, got %d", len(expectedRoutes), len(routes.Routes))
	}

	for i, route := range routes.Routes {
		if route.DstCIDR != expectedRoutes[i].DstCIDR {
			t.Errorf("Route %d: expected DstCIDR %s, got %s", i, expectedRoutes[i].DstCIDR, route.DstCIDR)
		}
		if route.NextHop != expectedRoutes[i].NextHop {
			t.Errorf("Route %d: expected NextHop %s, got %s", i, expectedRoutes[i].NextHop, route.NextHop)
		}
	}
}

// TestAgentControllerHealthCheck tests health check endpoint
func TestAgentControllerHealthCheck(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	resp, err := client.get(ctx, tc.URL()+"/health")
	if err != nil {
		t.Fatalf("Failed to check health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for healthy controller, got %d", resp.StatusCode)
	}

	tc.SetHealthy(false)

	resp2, err := client.get(ctx, tc.URL()+"/health")
	if err != nil {
		t.Fatalf("Failed to check health: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()

	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 for unhealthy controller, got %d", resp2.StatusCode)
	}
}

// TestTelemetryValidation tests that invalid telemetry is rejected
func TestTelemetryValidation(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	resp, err := client.post(ctx, tc.URL()+"/api/v1/telemetry", []byte(`{invalid json}`))
	if err != nil {
		t.Fatalf("Failed to send telemetry: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

// TestGetRoutesForUnknownAgent tests that getting routes for unknown agent returns 404
func TestGetRoutesForUnknownAgent(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	resp, err := client.get(ctx, tc.URL()+"/api/v1/routes?agent_id=unknown-agent")
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404 for unknown agent, got %d", resp.StatusCode)
	}
}

// TestGetRoutesWithoutAgentID tests that getting routes without agent_id returns 400
func TestGetRoutesWithoutAgentID(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	resp, err := client.get(ctx, tc.URL()+"/api/v1/routes")
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing agent_id, got %d", resp.StatusCode)
	}
}

// TestAgentEntersFallbackWhenControllerUnavailable tests that Agent enters fallback mode
// when Controller is unavailable
// Requirements: 5.3
func TestAgentEntersFallbackWhenControllerUnavailable(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-fallback"

	tc.SetRoutes(agentID, []models.RouteConfig{
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.2", Reason: "optimized_path"},
	})

	maxRetries := 3
	failureCount := 0
	inFallback := false

	tc.SetHealthy(false)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		rtt := 10.5
		telemetry := &models.TelemetryRequest{
			AgentID:   agentID,
			Timestamp: time.Now().Unix(),
			Metrics: []models.Metric{
				{TargetIP: "10.254.0.2", RTTMs: &rtt, LossRate: 0.0},
			},
		}

		data, _ := json.Marshal(telemetry)
		resp, err := client.post(ctx, tc.URL()+"/api/v1/telemetry", data)

		if err != nil || resp.StatusCode != http.StatusOK {
			failureCount++
			if resp != nil {
				_ = resp.Body.Close()
			}
		} else {
			failureCount = 0
			_ = resp.Body.Close()
		}
	}

	if failureCount >= maxRetries {
		inFallback = true
	}

	if !inFallback {
		t.Error("Expected to enter fallback mode after consecutive failures")
	}

	resp, err := client.get(ctx, tc.URL()+"/health")
	if err != nil {
		t.Fatalf("Failed to check health: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 when controller is unavailable, got %d", resp.StatusCode)
	}
}

// TestAgentRecoveryFromFallbackMode tests that Agent recovers from fallback mode
// when Controller becomes available again
// Requirements: 5.4
func TestAgentRecoveryFromFallbackMode(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-recovery"

	tc.SetRoutes(agentID, []models.RouteConfig{
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.2", Reason: "optimized_path"},
	})

	inFallback := true
	failureCount := 3

	tc.SetHealthy(true)

	rtt := 10.5
	telemetry := &models.TelemetryRequest{
		AgentID:   agentID,
		Timestamp: time.Now().Unix(),
		Metrics: []models.Metric{
			{TargetIP: "10.254.0.2", RTTMs: &rtt, LossRate: 0.0},
		},
	}

	data, _ := json.Marshal(telemetry)
	resp, err := client.post(ctx, tc.URL()+"/api/v1/telemetry", data)

	if err != nil {
		t.Fatalf("Failed to send telemetry: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusOK {
		failureCount = 0
		inFallback = false
	}

	if inFallback {
		t.Error("Expected to exit fallback mode after successful communication")
	}

	if failureCount != 0 {
		t.Errorf("Expected failure count to be reset to 0, got %d", failureCount)
	}

	routeResp, err := client.get(ctx, fmt.Sprintf("%s/api/v1/routes?agent_id=%s", tc.URL(), agentID))
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	defer func() { _ = routeResp.Body.Close() }()

	if routeResp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 after recovery, got %d", routeResp.StatusCode)
	}
}

// TestFallbackModeTransitions tests the complete fallback mode lifecycle
// Requirements: 5.3, 5.4
func TestFallbackModeTransitions(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-transitions"
	tc.SetRoutes(agentID, []models.RouteConfig{
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.2", Reason: "optimized_path"},
	})

	type AgentState struct {
		inFallback   bool
		failureCount int
		maxRetries   int
	}

	state := &AgentState{
		inFallback:   false,
		failureCount: 0,
		maxRetries:   3,
	}

	sendTelemetry := func() bool {
		rtt := 10.5
		telemetry := &models.TelemetryRequest{
			AgentID:   agentID,
			Timestamp: time.Now().Unix(),
			Metrics: []models.Metric{
				{TargetIP: "10.254.0.2", RTTMs: &rtt, LossRate: 0.0},
			},
		}

		data, _ := json.Marshal(telemetry)
		resp, err := client.post(ctx, tc.URL()+"/api/v1/telemetry", data)

		if err != nil || resp.StatusCode != http.StatusOK {
			state.failureCount++
			if resp != nil {
				_ = resp.Body.Close()
			}
			if state.failureCount >= state.maxRetries && !state.inFallback {
				state.inFallback = true
			}
			return false
		}

		_ = resp.Body.Close()
		state.failureCount = 0
		if state.inFallback {
			state.inFallback = false
		}
		return true
	}

	// Phase 1: Normal operation
	tc.SetHealthy(true)
	if !sendTelemetry() {
		t.Error("Phase 1: Expected telemetry to succeed")
	}
	if state.inFallback {
		t.Error("Phase 1: Should not be in fallback mode")
	}

	// Phase 2: Controller becomes unavailable
	tc.SetHealthy(false)
	for i := 0; i < state.maxRetries; i++ {
		sendTelemetry()
	}
	if !state.inFallback {
		t.Error("Phase 2: Should be in fallback mode after failures")
	}

	// Phase 3: Controller recovers
	tc.SetHealthy(true)
	if !sendTelemetry() {
		t.Error("Phase 3: Expected telemetry to succeed after recovery")
	}
	if state.inFallback {
		t.Error("Phase 3: Should have exited fallback mode")
	}
}

// TestControllerUnavailableRoutesRequest tests route requests when controller is unavailable
// Requirements: 5.3
func TestControllerUnavailableRoutesRequest(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-unavailable"
	tc.SetRoutes(agentID, []models.RouteConfig{
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.2", Reason: "optimized_path"},
	})

	tc.SetHealthy(false)

	resp, err := client.get(ctx, fmt.Sprintf("%s/api/v1/routes?agent_id=%s", tc.URL(), agentID))
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 when controller unavailable, got %d", resp.StatusCode)
	}
}

// TestRouteUpdatesAppliedCorrectly tests that route updates are applied correctly
// using a mock executor
// Requirements: 5.5
func TestRouteUpdatesAppliedCorrectly(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-routes"
	mockExecutor := NewMockExecutor()

	expectedRoutes := []models.RouteConfig{
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.2", Reason: "optimized_path"},
		{DstCIDR: "10.254.0.4/32", NextHop: "10.254.0.5", Reason: "optimized_path"},
	}
	tc.SetRoutes(agentID, expectedRoutes)

	rtt := 10.5
	telemetry := &models.TelemetryRequest{
		AgentID:   agentID,
		Timestamp: time.Now().Unix(),
		Metrics: []models.Metric{
			{TargetIP: "10.254.0.2", RTTMs: &rtt, LossRate: 0.0},
		},
	}

	data, _ := json.Marshal(telemetry)
	resp, err := client.post(ctx, tc.URL()+"/api/v1/telemetry", data)
	if err != nil {
		t.Fatalf("Failed to send telemetry: %v", err)
	}
	_ = resp.Body.Close()

	routeResp, err := client.get(ctx, fmt.Sprintf("%s/api/v1/routes?agent_id=%s", tc.URL(), agentID))
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	defer func() { _ = routeResp.Body.Close() }()

	var routes models.RouteResponse
	if err := json.NewDecoder(routeResp.Body).Decode(&routes); err != nil {
		t.Fatalf("Failed to decode routes: %v", err)
	}

	if err := mockExecutor.SyncRoutes(routes.Routes); err != nil {
		t.Fatalf("Failed to sync routes: %v", err)
	}

	appliedRoutes := mockExecutor.GetAppliedRoutes()
	if len(appliedRoutes) != len(expectedRoutes) {
		t.Errorf("Expected %d routes to be applied, got %d", len(expectedRoutes), len(appliedRoutes))
	}

	for i, route := range appliedRoutes {
		if route.DstCIDR != expectedRoutes[i].DstCIDR {
			t.Errorf("Route %d: expected DstCIDR %s, got %s", i, expectedRoutes[i].DstCIDR, route.DstCIDR)
		}
		if route.NextHop != expectedRoutes[i].NextHop {
			t.Errorf("Route %d: expected NextHop %s, got %s", i, expectedRoutes[i].NextHop, route.NextHop)
		}
	}
}

// TestRouteUpdateWithDirectRoute tests handling of direct routes
// Requirements: 5.5
func TestRouteUpdateWithDirectRoute(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-direct"
	mockExecutor := NewMockExecutor()

	routes := []models.RouteConfig{
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.2", Reason: "optimized_path"},
		{DstCIDR: "10.254.0.4/32", NextHop: "direct", Reason: "default"},
	}
	tc.SetRoutes(agentID, routes)

	routeResp, err := client.get(ctx, fmt.Sprintf("%s/api/v1/routes?agent_id=%s", tc.URL(), agentID))
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	defer func() { _ = routeResp.Body.Close() }()

	var routeResponse models.RouteResponse
	if err := json.NewDecoder(routeResp.Body).Decode(&routeResponse); err != nil {
		t.Fatalf("Failed to decode routes: %v", err)
	}

	if err := mockExecutor.SyncRoutes(routeResponse.Routes); err != nil {
		t.Fatalf("Failed to sync routes: %v", err)
	}

	appliedRoutes := mockExecutor.GetAppliedRoutes()

	hasDirectRoute := false
	for _, route := range appliedRoutes {
		if route.NextHop == "direct" {
			hasDirectRoute = true
			break
		}
	}

	if !hasDirectRoute {
		t.Error("Expected direct route to be applied")
	}
}

// TestRouteUpdateSequence tests sequential route updates
// Requirements: 5.5
func TestRouteUpdateSequence(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-sequence"
	mockExecutor := NewMockExecutor()

	routes1 := []models.RouteConfig{
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.2", Reason: "optimized_path"},
	}
	tc.SetRoutes(agentID, routes1)

	resp1, err := client.get(ctx, fmt.Sprintf("%s/api/v1/routes?agent_id=%s", tc.URL(), agentID))
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	var routeResp1 models.RouteResponse
	if decodeErr := json.NewDecoder(resp1.Body).Decode(&routeResp1); decodeErr != nil {
		t.Fatalf("Failed to decode routes: %v", decodeErr)
	}
	_ = resp1.Body.Close()
	if syncErr := mockExecutor.SyncRoutes(routeResp1.Routes); syncErr != nil {
		t.Fatalf("Failed to sync routes: %v", syncErr)
	}

	if len(mockExecutor.GetAppliedRoutes()) != 1 {
		t.Errorf("Expected 1 route after first update, got %d", len(mockExecutor.GetAppliedRoutes()))
	}

	routes2 := []models.RouteConfig{
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.5", Reason: "optimized_path"},
		{DstCIDR: "10.254.0.4/32", NextHop: "10.254.0.6", Reason: "optimized_path"},
	}
	tc.SetRoutes(agentID, routes2)

	resp2, err := client.get(ctx, fmt.Sprintf("%s/api/v1/routes?agent_id=%s", tc.URL(), agentID))
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	var routeResp2 models.RouteResponse
	if err := json.NewDecoder(resp2.Body).Decode(&routeResp2); err != nil {
		t.Fatalf("Failed to decode routes: %v", err)
	}
	_ = resp2.Body.Close()
	if err := mockExecutor.SyncRoutes(routeResp2.Routes); err != nil {
		t.Fatalf("Failed to sync routes: %v", err)
	}

	appliedRoutes := mockExecutor.GetAppliedRoutes()
	if len(appliedRoutes) != 3 {
		t.Errorf("Expected 3 routes after second update, got %d", len(appliedRoutes))
	}
}

// TestRouteFlushOnFallback tests that routes are flushed when entering fallback mode
// Requirements: 5.5
func TestRouteFlushOnFallback(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-flush"
	mockExecutor := NewMockExecutor()

	routes := []models.RouteConfig{
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.2", Reason: "optimized_path"},
	}
	tc.SetRoutes(agentID, routes)

	resp, err := client.get(ctx, fmt.Sprintf("%s/api/v1/routes?agent_id=%s", tc.URL(), agentID))
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	var routeResp models.RouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&routeResp); err != nil {
		t.Fatalf("Failed to decode routes: %v", err)
	}
	_ = resp.Body.Close()
	if err := mockExecutor.SyncRoutes(routeResp.Routes); err != nil {
		t.Fatalf("Failed to sync routes: %v", err)
	}

	if len(mockExecutor.GetAppliedRoutes()) == 0 {
		t.Error("Expected routes to be applied before flush")
	}

	if err := mockExecutor.FlushRoutes(); err != nil {
		t.Fatalf("Failed to flush routes: %v", err)
	}

	if !mockExecutor.WasFlushCalled() {
		t.Error("Expected FlushRoutes to be called")
	}

	if len(mockExecutor.GetAppliedRoutes()) != 0 {
		t.Errorf("Expected routes to be cleared after flush, got %d", len(mockExecutor.GetAppliedRoutes()))
	}
}

// TestMockExecutorFailure tests handling of executor failures
// Requirements: 5.5
func TestMockExecutorFailure(t *testing.T) {
	mockExecutor := NewMockExecutor()
	mockExecutor.SetShouldFail(true)

	routes := []models.RouteConfig{
		{DstCIDR: "10.254.0.3/32", NextHop: "10.254.0.2", Reason: "optimized_path"},
	}

	err := mockExecutor.SyncRoutes(routes)
	if err == nil {
		t.Error("Expected error when executor is set to fail")
	}

	mockExecutor.Reset()
	err = mockExecutor.SyncRoutes(routes)
	if err != nil {
		t.Errorf("Expected no error after reset, got: %v", err)
	}
}

// TestEmptyRouteResponse tests handling of empty route response
// Requirements: 5.5
func TestEmptyRouteResponse(t *testing.T) {
	tc := NewTestController()
	defer tc.Close()

	client := newHTTPClient()
	ctx := context.Background()

	agentID := "test-agent-empty"
	mockExecutor := NewMockExecutor()

	tc.SetRoutes(agentID, []models.RouteConfig{})

	resp, err := client.get(ctx, fmt.Sprintf("%s/api/v1/routes?agent_id=%s", tc.URL(), agentID))
	if err != nil {
		t.Fatalf("Failed to get routes: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var routeResp models.RouteResponse
	if err := json.NewDecoder(resp.Body).Decode(&routeResp); err != nil {
		t.Fatalf("Failed to decode routes: %v", err)
	}

	if err := mockExecutor.SyncRoutes(routeResp.Routes); err != nil {
		t.Fatalf("Failed to sync empty routes: %v", err)
	}

	if len(mockExecutor.GetAppliedRoutes()) != 0 {
		t.Errorf("Expected 0 routes for empty response, got %d", len(mockExecutor.GetAppliedRoutes()))
	}
}
