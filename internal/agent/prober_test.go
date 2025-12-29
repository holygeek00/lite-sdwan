package agent

import (
	"testing"
)

func TestSlidingWindow(t *testing.T) {
	sw := NewSlidingWindow(3)

	// 初始状态
	if sw.Len() != 0 {
		t.Errorf("Initial length should be 0, got %d", sw.Len())
	}

	// 添加数据
	sw.Add(Measurement{RTTMs: ptrFloat64(10.0), LossRate: 0.0})
	if sw.Len() != 1 {
		t.Errorf("Length should be 1, got %d", sw.Len())
	}

	sw.Add(Measurement{RTTMs: ptrFloat64(20.0), LossRate: 0.0})
	sw.Add(Measurement{RTTMs: ptrFloat64(30.0), LossRate: 0.0})

	if sw.Len() != 3 {
		t.Errorf("Length should be 3, got %d", sw.Len())
	}

	// 添加第 4 个，应该覆盖第 1 个
	sw.Add(Measurement{RTTMs: ptrFloat64(40.0), LossRate: 0.0})

	if sw.Len() != 3 {
		t.Errorf("Length should still be 3, got %d", sw.Len())
	}
}

func TestSlidingWindowAverage(t *testing.T) {
	sw := NewSlidingWindow(3)

	sw.Add(Measurement{RTTMs: ptrFloat64(10.0), LossRate: 0.0})
	sw.Add(Measurement{RTTMs: ptrFloat64(20.0), LossRate: 0.1})
	sw.Add(Measurement{RTTMs: ptrFloat64(30.0), LossRate: 0.2})

	avgRTT, avgLoss := sw.GetAverage()

	if avgRTT == nil {
		t.Fatal("avgRTT should not be nil")
	}

	expectedRTT := 20.0 // (10 + 20 + 30) / 3
	if *avgRTT != expectedRTT {
		t.Errorf("avgRTT = %v, want %v", *avgRTT, expectedRTT)
	}

	expectedLoss := 0.1 // (0 + 0.1 + 0.2) / 3
	if avgLoss < expectedLoss-0.001 || avgLoss > expectedLoss+0.001 {
		t.Errorf("avgLoss = %v, want ~%v", avgLoss, expectedLoss)
	}
}

func TestSlidingWindowWithTimeout(t *testing.T) {
	sw := NewSlidingWindow(3)

	sw.Add(Measurement{RTTMs: ptrFloat64(10.0), LossRate: 0.0})
	sw.Add(Measurement{RTTMs: nil, LossRate: 1.0}) // 超时
	sw.Add(Measurement{RTTMs: ptrFloat64(20.0), LossRate: 0.0})

	avgRTT, avgLoss := sw.GetAverage()

	if avgRTT == nil {
		t.Fatal("avgRTT should not be nil")
	}

	// RTT 平均只计算非 nil 的值
	expectedRTT := 15.0 // (10 + 20) / 2
	if *avgRTT != expectedRTT {
		t.Errorf("avgRTT = %v, want %v", *avgRTT, expectedRTT)
	}

	// Loss 平均计算所有值
	expectedLoss := 1.0 / 3.0 // (0 + 1 + 0) / 3
	if avgLoss < expectedLoss-0.01 || avgLoss > expectedLoss+0.01 {
		t.Errorf("avgLoss = %v, want ~%v", avgLoss, expectedLoss)
	}
}

func TestSlidingWindowAllTimeout(t *testing.T) {
	sw := NewSlidingWindow(3)

	sw.Add(Measurement{RTTMs: nil, LossRate: 1.0})
	sw.Add(Measurement{RTTMs: nil, LossRate: 1.0})
	sw.Add(Measurement{RTTMs: nil, LossRate: 1.0})

	avgRTT, avgLoss := sw.GetAverage()

	if avgRTT != nil {
		t.Errorf("avgRTT should be nil when all timeouts, got %v", *avgRTT)
	}

	if avgLoss != 1.0 {
		t.Errorf("avgLoss should be 1.0, got %v", avgLoss)
	}
}

func TestSlidingWindowEmpty(t *testing.T) {
	sw := NewSlidingWindow(3)

	avgRTT, avgLoss := sw.GetAverage()

	if avgRTT != nil {
		t.Errorf("avgRTT should be nil for empty window")
	}

	if avgLoss != 0 {
		t.Errorf("avgLoss should be 0 for empty window")
	}
}

func TestSlidingWindowOverflow(t *testing.T) {
	sw := NewSlidingWindow(3)

	// 添加 5 个值，验证只保留最后 3 个
	for i := 1; i <= 5; i++ {
		sw.Add(Measurement{RTTMs: ptrFloat64(float64(i * 10)), LossRate: 0.0})
	}

	if sw.Len() != 3 {
		t.Errorf("Length should be 3, got %d", sw.Len())
	}

	avgRTT, _ := sw.GetAverage()
	// 最后 3 个值是 30, 40, 50
	expectedRTT := 40.0 // (30 + 40 + 50) / 3
	if *avgRTT != expectedRTT {
		t.Errorf("avgRTT = %v, want %v", *avgRTT, expectedRTT)
	}
}

func ptrFloat64(v float64) *float64 {
	return &v
}
