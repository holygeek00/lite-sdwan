package models

import "errors"

var (
	// 验证错误
	ErrEmptyAgentID     = errors.New("agent_id cannot be empty")
	ErrInvalidTimestamp = errors.New("timestamp must be positive")
	ErrEmptyMetrics     = errors.New("metrics cannot be empty")
	ErrEmptyTargetIP    = errors.New("target_ip cannot be empty")
	ErrNegativeRTT      = errors.New("rtt_ms cannot be negative")
	ErrInvalidLossRate  = errors.New("loss_rate must be between 0.0 and 1.0")

	// 业务错误
	ErrAgentNotFound = errors.New("agent not found")
	ErrNoPath        = errors.New("no path available")
)
