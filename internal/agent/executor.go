package agent

import (
	"fmt"
	"log"
	"net"
	"os/exec"
	"strings"
	"sync"

	"github.com/holygeek00/lite-sdwan/pkg/models"
)

// Executor 路由执行器
type Executor struct {
	wgInterface string
	subnet      *net.IPNet
	mu          sync.Mutex
}

// NewExecutor 创建新的路由执行器
func NewExecutor(wgInterface, subnet string) (*Executor, error) {
	_, ipNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet: %w", err)
	}

	return &Executor{
		wgInterface: wgInterface,
		subnet:      ipNet,
	}, nil
}

// CurrentRoute 当前路由信息
type CurrentRoute struct {
	Destination string
	NextHop     string // 空字符串表示直连
}

// GetCurrentRoutes 获取当前路由表
func (e *Executor) GetCurrentRoutes() ([]CurrentRoute, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	cmd := exec.Command("ip", "route", "show", "table", "main")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get routes: %w", err)
	}

	routes := make([]CurrentRoute, 0)
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 只处理 WireGuard 接口的路由
		if !strings.Contains(line, e.wgInterface) {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}

		dst := parts[0]

		// 检查是否在允许的子网内
		if !e.isInSubnet(dst) {
			continue
		}

		route := CurrentRoute{Destination: dst}

		// 查找 via 关键字
		for i, p := range parts {
			if p == "via" && i+1 < len(parts) {
				route.NextHop = parts[i+1]
				break
			}
		}

		routes = append(routes, route)
	}

	return routes, nil
}

// isInSubnet 检查 IP 是否在允许的子网内
func (e *Executor) isInSubnet(dst string) bool {
	// 移除 CIDR 后缀
	ip := strings.Split(dst, "/")[0]
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return e.subnet.Contains(parsedIP)
}

// ValidateIP 验证 IP 是否在允许的子网内
func (e *Executor) ValidateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	return e.subnet.Contains(parsedIP)
}

// GenerateAddCommand 生成添加/替换路由的命令
func (e *Executor) GenerateAddCommand(dstIP, nextHop string) []string {
	return []string{
		"ip", "route", "replace",
		dstIP + "/32",
		"via", nextHop,
		"dev", e.wgInterface,
	}
}

// GenerateDelCommand 生成删除路由的命令
func (e *Executor) GenerateDelCommand(dstIP string) []string {
	return []string{
		"ip", "route", "del",
		dstIP + "/32",
		"dev", e.wgInterface,
	}
}

// ApplyRoute 应用单条路由
func (e *Executor) ApplyRoute(route models.RouteConfig) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// 提取目标 IP
	dstIP := strings.TrimSuffix(route.DstCIDR, "/32")

	// 安全检查
	if !e.ValidateIP(dstIP) {
		return fmt.Errorf("IP %s is not in allowed subnet %s", dstIP, e.subnet.String())
	}

	var args []string
	if route.NextHop == "direct" {
		// 删除中继路由，恢复直连
		args = e.GenerateDelCommand(dstIP)
		log.Printf("Removing relay route: %s", strings.Join(args, " "))
	} else {
		// 添加/替换中继路由
		if !e.ValidateIP(route.NextHop) {
			return fmt.Errorf("next_hop %s is not in allowed subnet %s", route.NextHop, e.subnet.String())
		}
		args = e.GenerateAddCommand(dstIP, route.NextHop)
		log.Printf("Adding relay route: %s", strings.Join(args, " "))
	}

	// #nosec G204 - args are validated above via ValidateIP
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	output, err := cmd.CombinedOutput()
	if err != nil {
		// 删除不存在的路由不算错误
		if route.NextHop == "direct" && strings.Contains(string(output), "No such process") {
			return nil
		}
		return fmt.Errorf("route command failed: %s, output: %s", err, string(output))
	}

	return nil
}

// SyncRoutes 同步路由配置
func (e *Executor) SyncRoutes(desired []models.RouteConfig) error {
	for _, route := range desired {
		if err := e.ApplyRoute(route); err != nil {
			log.Printf("Failed to apply route %s: %v", route.DstCIDR, err)
			// 继续处理其他路由
		}
	}
	return nil
}

// FlushRoutes 清空所有动态添加的路由
func (e *Executor) FlushRoutes() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	log.Printf("Flushing all dynamic routes for %s", e.wgInterface)

	// 获取当前路由
	cmd := exec.Command("ip", "route", "show", "table", "main")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get routes: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 只处理有 via 的路由（中继路由）
		if !strings.Contains(line, e.wgInterface) || !strings.Contains(line, "via") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 1 {
			continue
		}

		dst := parts[0]
		if !e.isInSubnet(dst) {
			continue
		}

		// 删除路由
		delCmd := exec.Command("ip", "route", "del", dst, "dev", e.wgInterface) //nolint:gosec
		if delErr := delCmd.Run(); delErr != nil {
			log.Printf("Failed to delete route %s: %v", dst, delErr)
		} else {
			log.Printf("Deleted route: %s", dst)
		}
	}

	return nil
}

// CalculateDiff 计算路由差异
func CalculateDiff(current []CurrentRoute, desired []models.RouteConfig) (toAdd, toRemove []models.RouteConfig) {
	currentMap := make(map[string]string) // dst -> nextHop
	for _, r := range current {
		currentMap[r.Destination] = r.NextHop
	}

	desiredMap := make(map[string]string) // dst -> nextHop
	for _, r := range desired {
		if r.NextHop != "direct" {
			desiredMap[r.DstCIDR] = r.NextHop
		}
	}

	// 预分配切片
	toAdd = make([]models.RouteConfig, 0, len(desiredMap))
	toRemove = make([]models.RouteConfig, 0, len(currentMap))

	// 需要添加或修改的路由
	for dst, nextHop := range desiredMap {
		if currentNextHop, exists := currentMap[dst]; !exists || currentNextHop != nextHop {
			toAdd = append(toAdd, models.RouteConfig{
				DstCIDR: dst,
				NextHop: nextHop,
				Reason:  "optimized_path",
			})
		}
	}

	// 需要删除的路由（当前有但期望没有）
	for dst := range currentMap {
		if _, exists := desiredMap[dst]; !exists {
			toRemove = append(toRemove, models.RouteConfig{
				DstCIDR: dst,
				NextHop: "direct",
				Reason:  "default",
			})
		}
	}

	return toAdd, toRemove
}
