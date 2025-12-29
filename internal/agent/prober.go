// Package agent 实现 SD-WAN Agent 功能
package agent

import (
	"log"
	"sync"
	"time"

	probing "github.com/go-ping/ping"

	"github.com/example/lite-sdwan/pkg/models"
)

// Prober 链路探测器
type Prober struct {
	peerIPs    []string
	interval   time.Duration
	timeout    time.Duration
	windowSize int

	mu      sync.RWMutex
	buffers map[string]*SlidingWindow // target_ip -> measurements
	running bool
	stopCh  chan struct{}
}

// SlidingWindow 滑动窗口缓冲区
type SlidingWindow struct {
	data     []Measurement
	maxSize  int
	position int
	count    int
}

// Measurement 单次测量结果
type Measurement struct {
	RTTMs    *float64
	LossRate float64
	Time     time.Time
}

// NewSlidingWindow 创建新的滑动窗口
func NewSlidingWindow(size int) *SlidingWindow {
	return &SlidingWindow{
		data:    make([]Measurement, size),
		maxSize: size,
	}
}

// Add 添加测量结果
func (sw *SlidingWindow) Add(m Measurement) {
	sw.data[sw.position] = m
	sw.position = (sw.position + 1) % sw.maxSize
	if sw.count < sw.maxSize {
		sw.count++
	}
}

// GetAverage 获取平均值
func (sw *SlidingWindow) GetAverage() (avgRTT *float64, avgLoss float64) {
	if sw.count == 0 {
		return nil, 0
	}

	var rttSum float64
	var rttCount int
	var lossSum float64

	for i := 0; i < sw.count; i++ {
		m := sw.data[i]
		if m.RTTMs != nil {
			rttSum += *m.RTTMs
			rttCount++
		}
		lossSum += m.LossRate
	}

	avgLoss = lossSum / float64(sw.count)

	if rttCount > 0 {
		avg := rttSum / float64(rttCount)
		avgRTT = &avg
	}

	return avgRTT, avgLoss
}

// Len 返回当前数据量
func (sw *SlidingWindow) Len() int {
	return sw.count
}

// NewProber 创建新的探测器
func NewProber(peerIPs []string, interval, timeout time.Duration, windowSize int) *Prober {
	buffers := make(map[string]*SlidingWindow)
	for _, ip := range peerIPs {
		buffers[ip] = NewSlidingWindow(windowSize)
	}

	return &Prober{
		peerIPs:    peerIPs,
		interval:   interval,
		timeout:    timeout,
		windowSize: windowSize,
		buffers:    buffers,
		stopCh:     make(chan struct{}),
	}
}

// ProbeOnce 执行一次探测
func (p *Prober) ProbeOnce(targetIP string) Measurement {
	pinger, err := probing.NewPinger(targetIP)
	if err != nil {
		log.Printf("Failed to create pinger for %s: %v", targetIP, err)
		return Measurement{RTTMs: nil, LossRate: 1.0, Time: time.Now()}
	}

	pinger.Count = 1
	pinger.Timeout = p.timeout
	pinger.SetPrivileged(true) // 需要 root 权限

	err = pinger.Run()
	if err != nil {
		log.Printf("Ping failed for %s: %v", targetIP, err)
		return Measurement{RTTMs: nil, LossRate: 1.0, Time: time.Now()}
	}

	stats := pinger.Statistics()

	var rtt *float64
	var lossRate float64

	if stats.PacketsRecv > 0 {
		rttMs := float64(stats.AvgRtt.Microseconds()) / 1000.0
		rtt = &rttMs
		lossRate = float64(stats.PacketsSent-stats.PacketsRecv) / float64(stats.PacketsSent)
	} else {
		lossRate = 1.0
	}

	return Measurement{RTTMs: rtt, LossRate: lossRate, Time: time.Now()}
}

// Start 启动探测循环
func (p *Prober) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.mu.Unlock()

	go p.run()
}

// run 探测循环
func (p *Prober) run() {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// 立即执行一次
	p.probeAll()

	for {
		select {
		case <-ticker.C:
			p.probeAll()
		case <-p.stopCh:
			return
		}
	}
}

// probeAll 探测所有对等节点
func (p *Prober) probeAll() {
	for _, ip := range p.peerIPs {
		m := p.ProbeOnce(ip)

		p.mu.Lock()
		if sw, ok := p.buffers[ip]; ok {
			sw.Add(m)
		}
		p.mu.Unlock()

		if m.RTTMs != nil {
			log.Printf("Probe %s: RTT=%.2fms, Loss=%.1f%%", ip, *m.RTTMs, m.LossRate*100)
		} else {
			log.Printf("Probe %s: timeout", ip)
		}
	}
}

// Stop 停止探测
func (p *Prober) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	p.mu.Unlock()

	close(p.stopCh)
}

// GetMetrics 获取当前指标（使用移动平均）
func (p *Prober) GetMetrics() []models.Metric {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metrics := make([]models.Metric, 0, len(p.peerIPs))
	for _, ip := range p.peerIPs {
		sw := p.buffers[ip]
		avgRTT, avgLoss := sw.GetAverage()

		metrics = append(metrics, models.Metric{
			TargetIP: ip,
			RTTMs:    avgRTT,
			LossRate: avgLoss,
		})
	}

	return metrics
}

// GetRawMetrics 获取原始指标（最新一次测量）
func (p *Prober) GetRawMetrics() []models.Metric {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metrics := make([]models.Metric, 0, len(p.peerIPs))
	for _, ip := range p.peerIPs {
		sw := p.buffers[ip]
		if sw.count == 0 {
			continue
		}

		// 获取最新的测量
		idx := (sw.position - 1 + sw.maxSize) % sw.maxSize
		m := sw.data[idx]

		metrics = append(metrics, models.Metric{
			TargetIP: ip,
			RTTMs:    m.RTTMs,
			LossRate: m.LossRate,
		})
	}

	return metrics
}
