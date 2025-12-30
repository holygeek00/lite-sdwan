// Package controller 实现 SD-WAN Controller 功能
package controller

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/holygeek00/lite-sdwan/pkg/logging"
)

// StaleDataCleaner 陈旧数据清理器
type StaleDataCleaner struct {
	db        *TopologyDB
	threshold time.Duration
	interval  time.Duration
	logger    logging.Logger
	stopCh    chan struct{}
	wg        sync.WaitGroup

	// Metrics
	cleanupCount int64
}

// NewStaleDataCleaner 创建清理器
func NewStaleDataCleaner(db *TopologyDB, threshold, interval time.Duration, logger logging.Logger) *StaleDataCleaner {
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	return &StaleDataCleaner{
		db:        db,
		threshold: threshold,
		interval:  interval,
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// Start 启动清理循环
func (c *StaleDataCleaner) Start() {
	c.wg.Add(1)
	go c.run()
	c.logger.Info("Stale data cleaner started",
		logging.F("threshold", c.threshold.String()),
		logging.F("interval", c.interval.String()),
	)
}

// Stop 停止清理器
func (c *StaleDataCleaner) Stop() {
	close(c.stopCh)
	c.wg.Wait()
	c.logger.Info("Stale data cleaner stopped",
		logging.F("total_cleanups", c.GetCleanupCount()),
	)
}

// run 清理循环
func (c *StaleDataCleaner) run() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanOnce()
		case <-c.stopCh:
			return
		}
	}
}

// cleanOnce 执行单次清理
func (c *StaleDataCleaner) cleanOnce() {
	// 获取清理前的节点列表用于日志
	beforeIDs := c.db.GetAllAgentIDs()

	// 执行清理
	removed := c.db.CleanStale(c.threshold)

	if removed > 0 {
		// 获取清理后的节点列表，计算被移除的节点
		afterIDs := c.db.GetAllAgentIDs()
		afterSet := make(map[string]struct{}, len(afterIDs))
		for _, id := range afterIDs {
			afterSet[id] = struct{}{}
		}

		removedNodes := make([]string, 0, removed)
		for _, id := range beforeIDs {
			if _, exists := afterSet[id]; !exists {
				removedNodes = append(removedNodes, id)
			}
		}

		c.logger.Info("Cleaned stale data",
			logging.F("removed_count", removed),
			logging.F("removed_nodes", removedNodes),
			logging.F("remaining_nodes", len(afterIDs)),
		)

		// 更新清理计数
		atomic.AddInt64(&c.cleanupCount, int64(removed))
	}
}

// GetCleanupCount 获取清理计数
func (c *StaleDataCleaner) GetCleanupCount() int64 {
	return atomic.LoadInt64(&c.cleanupCount)
}
