package models

import (
	"encoding/json"
	"testing"
)

func TestTelemetryRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     TelemetryRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: TelemetryRequest{
				AgentID:   "10.254.0.1",
				Timestamp: 1234567890,
				Metrics: []Metric{
					{TargetIP: "10.254.0.2", RTTMs: ptrFloat64(10.5), LossRate: 0.0},
				},
			},
			wantErr: nil,
		},
		{
			name: "empty agent_id",
			req: TelemetryRequest{
				AgentID:   "",
				Timestamp: 1234567890,
				Metrics:   []Metric{{TargetIP: "10.254.0.2", LossRate: 0.0}},
			},
			wantErr: ErrEmptyAgentID,
		},
		{
			name: "invalid timestamp",
			req: TelemetryRequest{
				AgentID:   "10.254.0.1",
				Timestamp: 0,
				Metrics:   []Metric{{TargetIP: "10.254.0.2", LossRate: 0.0}},
			},
			wantErr: ErrInvalidTimestamp,
		},
		{
			name: "empty metrics",
			req: TelemetryRequest{
				AgentID:   "10.254.0.1",
				Timestamp: 1234567890,
				Metrics:   []Metric{},
			},
			wantErr: ErrEmptyMetrics,
		},
		{
			name: "negative RTT",
			req: TelemetryRequest{
				AgentID:   "10.254.0.1",
				Timestamp: 1234567890,
				Metrics:   []Metric{{TargetIP: "10.254.0.2", RTTMs: ptrFloat64(-1.0), LossRate: 0.0}},
			},
			wantErr: ErrNegativeRTT,
		},
		{
			name: "invalid loss rate",
			req: TelemetryRequest{
				AgentID:   "10.254.0.1",
				Timestamp: 1234567890,
				Metrics:   []Metric{{TargetIP: "10.254.0.2", LossRate: 1.5}},
			},
			wantErr: ErrInvalidLossRate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTelemetrySerializationRoundTrip(t *testing.T) {
	original := TelemetryRequest{
		AgentID:   "10.254.0.1",
		Timestamp: 1234567890,
		Metrics: []Metric{
			{TargetIP: "10.254.0.2", RTTMs: ptrFloat64(10.5), LossRate: 0.0},
			{TargetIP: "10.254.0.3", RTTMs: nil, LossRate: 1.0},
		},
	}

	// 序列化
	data, err := original.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	// 反序列化
	var decoded TelemetryRequest
	if err := decoded.FromJSON(data); err != nil {
		t.Fatalf("FromJSON() error = %v", err)
	}

	// 比较
	if original.AgentID != decoded.AgentID {
		t.Errorf("AgentID mismatch: %v != %v", original.AgentID, decoded.AgentID)
	}
	if original.Timestamp != decoded.Timestamp {
		t.Errorf("Timestamp mismatch: %v != %v", original.Timestamp, decoded.Timestamp)
	}
	if len(original.Metrics) != len(decoded.Metrics) {
		t.Errorf("Metrics length mismatch: %v != %v", len(original.Metrics), len(decoded.Metrics))
	}
}

func TestMetricJSONSerialization(t *testing.T) {
	metric := Metric{
		TargetIP: "10.254.0.2",
		RTTMs:    ptrFloat64(35.5),
		LossRate: 0.05,
	}

	data, err := json.Marshal(metric)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Metric
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if metric.TargetIP != decoded.TargetIP {
		t.Errorf("TargetIP mismatch")
	}
	if *metric.RTTMs != *decoded.RTTMs {
		t.Errorf("RTTMs mismatch")
	}
	if metric.LossRate != decoded.LossRate {
		t.Errorf("LossRate mismatch")
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}
