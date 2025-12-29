"""
Unit tests for Prober module - RTT and packet loss rate calculation.
Tests normal response RTT calculation and timeout packet loss scenarios.
Requirements: 2.2, 2.3
"""

import pytest
from unittest.mock import patch, MagicMock
from hypothesis import given, settings, strategies as st
from agent.prober import Prober, SlidingWindowBuffer


class TestRTTCalculation:
    """Test RTT (Round-Trip Time) calculation."""
    
    def test_rtt_calculation_normal_response(self):
        """
        Test RTT calculation for normal ping responses.
        Requirement 2.2: Calculate round-trip time in milliseconds.
        """
        prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0)
        
        # Mock ping3.ping to return 0.0355 seconds (35.5 ms)
        with patch('agent.prober.ping3.ping') as mock_ping:
            mock_ping.return_value = 0.0355  # 35.5 ms in seconds
            
            result = prober.probe_once("10.254.0.2")
            
            # Verify RTT is converted to milliseconds
            assert result["target"] == "10.254.0.2"
            assert result["rtt_ms"] == pytest.approx(35.5, rel=1e-6)
            assert result["loss"] == 0.0
    
    def test_rtt_calculation_various_values(self):
        """Test RTT calculation with various response times."""
        prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0)
        
        test_cases = [
            (0.001, 1.0),      # 1 ms
            (0.050, 50.0),     # 50 ms
            (0.100, 100.0),    # 100 ms
            (0.500, 500.0),    # 500 ms
            (1.000, 1000.0),   # 1000 ms
        ]
        
        for rtt_seconds, expected_ms in test_cases:
            with patch('agent.prober.ping3.ping') as mock_ping:
                mock_ping.return_value = rtt_seconds
                
                result = prober.probe_once("10.254.0.2")
                
                assert result["rtt_ms"] == pytest.approx(expected_ms, rel=1e-6)
                assert result["loss"] == 0.0
    
    def test_rtt_non_negative(self):
        """Test that RTT is always non-negative for successful pings."""
        prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0)
        
        with patch('agent.prober.ping3.ping') as mock_ping:
            mock_ping.return_value = 0.0  # Edge case: zero RTT
            
            result = prober.probe_once("10.254.0.2")
            
            assert result["rtt_ms"] == 0.0
            assert result["rtt_ms"] >= 0.0
            assert result["loss"] == 0.0


class TestPacketLossCalculation:
    """Test packet loss rate calculation."""
    
    def test_packet_loss_on_timeout(self):
        """
        Test packet loss calculation when ping times out.
        Requirement 2.3: Calculate packet loss rate as a percentage.
        """
        prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0)
        
        # Mock ping3.ping to return None (timeout)
        with patch('agent.prober.ping3.ping') as mock_ping:
            mock_ping.return_value = None
            
            result = prober.probe_once("10.254.0.2")
            
            # Verify timeout is recorded as 100% loss
            assert result["target"] == "10.254.0.2"
            assert result["rtt_ms"] is None
            assert result["loss"] == 1.0
    
    def test_packet_loss_rate_all_success(self):
        """Test loss rate when all pings succeed."""
        prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0)
        
        # Simulate 5 successful pings
        with patch('agent.prober.ping3.ping') as mock_ping:
            mock_ping.return_value = 0.050  # 50 ms
            
            for _ in range(5):
                prober.probe_once("10.254.0.2")
            
            # Get smoothed metrics
            metrics = prober.get_smoothed_metrics()
            
            # Loss rate should be 0.0 (0%)
            assert len(metrics) == 1
            assert metrics[0].loss_rate == 0.0
    
    def test_packet_loss_rate_all_failures(self):
        """Test loss rate when all pings fail."""
        prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0)
        
        # Simulate 5 failed pings
        with patch('agent.prober.ping3.ping') as mock_ping:
            mock_ping.return_value = None  # Timeout
            
            for _ in range(5):
                prober.probe_all()  # Use probe_all to update buffers
            
            # Get smoothed metrics
            metrics = prober.get_smoothed_metrics()
            
            # Loss rate should be 1.0 (100%)
            assert len(metrics) == 1
            assert metrics[0].loss_rate == 1.0
            assert metrics[0].rtt_ms is None  # No RTT data
    
    def test_packet_loss_rate_partial_failures(self):
        """Test loss rate with mixed success and failure."""
        prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0)
        
        # Simulate 10 pings: 3 failures, 7 successes
        with patch('agent.prober.ping3.ping') as mock_ping:
            # First 3 fail
            mock_ping.return_value = None
            for _ in range(3):
                prober.probe_all()  # Use probe_all to update buffers
            
            # Next 7 succeed
            mock_ping.return_value = 0.050
            for _ in range(7):
                prober.probe_all()  # Use probe_all to update buffers
            
            # Get smoothed metrics
            metrics = prober.get_smoothed_metrics()
            
            # Loss rate should be 0.3 (30%)
            assert len(metrics) == 1
            assert metrics[0].loss_rate == pytest.approx(0.3, rel=1e-6)
    
    def test_packet_loss_rate_range(self):
        """Test that loss rate is always between 0.0 and 1.0."""
        prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0)
        
        test_scenarios = [
            (0, 10),   # 0 failures out of 10
            (5, 10),   # 5 failures out of 10
            (10, 10),  # 10 failures out of 10
        ]
        
        for failures, total in test_scenarios:
            # Reset buffer
            prober.buffers["10.254.0.2"]["loss"].clear()
            prober.buffers["10.254.0.2"]["rtt"].clear()
            
            with patch('agent.prober.ping3.ping') as mock_ping:
                # Simulate failures
                mock_ping.return_value = None
                for _ in range(failures):
                    prober.probe_all()  # Use probe_all to update buffers
                
                # Simulate successes
                mock_ping.return_value = 0.050
                for _ in range(total - failures):
                    prober.probe_all()  # Use probe_all to update buffers
                
                metrics = prober.get_smoothed_metrics()
                loss_rate = metrics[0].loss_rate
                
                # Verify loss rate is in valid range
                assert 0.0 <= loss_rate <= 1.0
                assert loss_rate == pytest.approx(failures / total, rel=1e-6)
    
    def test_packet_loss_on_exception(self):
        """Test that exceptions during ping are treated as packet loss."""
        prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0)
        
        # Mock ping3.ping to raise an exception
        with patch('agent.prober.ping3.ping') as mock_ping:
            mock_ping.side_effect = Exception("Network error")
            
            result = prober.probe_once("10.254.0.2")
            
            # Exception should be treated as loss
            assert result["target"] == "10.254.0.2"
            assert result["rtt_ms"] is None
            assert result["loss"] == 1.0


class TestProberIntegration:
    """Integration tests for Prober with RTT and loss calculations."""
    
    def test_probe_all_mixed_results(self):
        """Test probing multiple peers with mixed success/failure."""
        peer_ips = ["10.254.0.2", "10.254.0.3", "10.254.0.4"]
        prober = Prober(peer_ips=peer_ips, interval=5, timeout=2.0)
        
        with patch('agent.prober.ping3.ping') as mock_ping:
            # Configure different responses for different IPs
            def ping_side_effect(ip, timeout):
                if ip == "10.254.0.2":
                    return 0.035  # 35 ms
                elif ip == "10.254.0.3":
                    return None   # Timeout
                elif ip == "10.254.0.4":
                    return 0.150  # 150 ms
                return None
            
            mock_ping.side_effect = ping_side_effect
            
            results = prober.probe_all()
            
            # Verify all peers were probed
            assert len(results) == 3
            
            # Check individual results
            assert results[0]["target"] == "10.254.0.2"
            assert results[0]["rtt_ms"] == pytest.approx(35.0, rel=1e-6)
            assert results[0]["loss"] == 0.0
            
            assert results[1]["target"] == "10.254.0.3"
            assert results[1]["rtt_ms"] is None
            assert results[1]["loss"] == 1.0
            
            assert results[2]["target"] == "10.254.0.4"
            assert results[2]["rtt_ms"] == pytest.approx(150.0, rel=1e-6)
            assert results[2]["loss"] == 0.0
    
    def test_smoothed_metrics_after_multiple_probes(self):
        """Test that smoothed metrics correctly average multiple probe cycles."""
        prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0, window_size=5)
        
        with patch('agent.prober.ping3.ping') as mock_ping:
            # Simulate 5 probes with varying RTT
            rtt_values = [0.030, 0.040, 0.050, 0.060, 0.070]  # 30, 40, 50, 60, 70 ms
            
            for rtt in rtt_values:
                mock_ping.return_value = rtt
                prober.probe_all()
            
            metrics = prober.get_smoothed_metrics()
            
            # Average should be 50 ms
            expected_avg = sum([v * 1000 for v in rtt_values]) / len(rtt_values)
            assert metrics[0].rtt_ms == pytest.approx(expected_avg, rel=1e-6)
            assert metrics[0].loss_rate == 0.0



# Property-based tests

@settings(max_examples=100)
@given(
    maxlen=st.integers(min_value=1, max_value=100),
    measurements=st.lists(
        st.floats(min_value=0.0, max_value=10000.0, allow_nan=False, allow_infinity=False),
        min_size=0,
        max_size=200
    )
)
def test_property_sliding_window_buffer_behavior(maxlen, measurements):
    """
    Feature: lite-sdwan-routing, Property 5: Sliding Window Buffer Behavior
    
    For any sequence of measurements added to a sliding window buffer with max size N,
    the buffer should never exceed N entries and should evict the oldest entry when full.
    
    Validates: Requirements 2.4
    """
    # Create buffer with specified max length
    buffer = SlidingWindowBuffer(maxlen=maxlen)
    
    # Track what we expect to be in the buffer
    expected_contents = []
    
    # Add measurements one by one
    for idx, measurement in enumerate(measurements):
        buffer.append(measurement)
        
        # Update expected contents (FIFO with max size)
        expected_contents.append(measurement)
        if len(expected_contents) > maxlen:
            expected_contents.pop(0)  # Remove oldest
        
        # Property 1: Buffer size never exceeds maxlen
        assert len(buffer) <= maxlen, f"Buffer size {len(buffer)} exceeds maxlen {maxlen}"
        
        # Property 2: Buffer size equals min(measurements_added, maxlen)
        measurements_added = idx + 1
        expected_size = min(measurements_added, maxlen)
        assert len(buffer) == expected_size, \
            f"Buffer size {len(buffer)} != expected {expected_size}"
        
        # Property 3: Buffer contents match expected FIFO behavior
        actual_contents = list(buffer)
        assert actual_contents == expected_contents, \
            f"Buffer contents {actual_contents} != expected {expected_contents}"
    
    # Final verification: buffer size is correct
    final_expected_size = min(len(measurements), maxlen)
    assert len(buffer) == final_expected_size, \
        f"Final buffer size {len(buffer)} != expected {final_expected_size}"
    
    # Verify buffer contains the last N measurements (where N = min(len(measurements), maxlen))
    if len(measurements) > 0:
        expected_final_contents = measurements[-maxlen:] if len(measurements) >= maxlen else measurements
        actual_final_contents = list(buffer)
        assert actual_final_contents == expected_final_contents, \
            f"Final buffer contents don't match last {maxlen} measurements"


@settings(max_examples=100)
@given(
    measurements=st.lists(
        st.floats(min_value=0.0, max_value=10000.0, allow_nan=False, allow_infinity=False),
        min_size=1,
        max_size=100
    )
)
def test_property_moving_average_calculation(measurements):
    """
    Feature: lite-sdwan-routing, Property 21: Moving Average Calculation
    
    For any sequence of N measurements, the moving average should equal
    the sum of all measurements divided by N.
    
    Validates: Requirements 7.4
    """
    # Create buffer large enough to hold all measurements
    buffer = SlidingWindowBuffer(maxlen=len(measurements))
    
    # Add all measurements to buffer
    for measurement in measurements:
        buffer.append(measurement)
    
    # Get moving average from buffer
    actual_average = buffer.get_moving_average()
    
    # Calculate expected average manually
    expected_average = sum(measurements) / len(measurements)
    
    # Property: Moving average equals sum / count
    assert actual_average is not None, "Moving average should not be None for non-empty buffer"
    assert actual_average == pytest.approx(expected_average, rel=1e-9, abs=1e-9), \
        f"Moving average {actual_average} != expected {expected_average}"
    
    # Additional property: Average should be within the range of measurements
    min_val = min(measurements)
    max_val = max(measurements)
    assert min_val <= actual_average <= max_val, \
        f"Average {actual_average} is outside range [{min_val}, {max_val}]"


@settings(max_examples=100)
@given(
    maxlen=st.integers(min_value=1, max_value=50),
    measurements=st.lists(
        st.floats(min_value=0.0, max_value=10000.0, allow_nan=False, allow_infinity=False),
        min_size=1,
        max_size=100
    )
)
def test_property_moving_average_with_sliding_window(maxlen, measurements):
    """
    Feature: lite-sdwan-routing, Property 21: Moving Average Calculation (Sliding Window)
    
    For any sequence of measurements added to a sliding window buffer,
    the moving average should equal the sum of the current buffer contents divided by
    the current buffer size.
    
    Validates: Requirements 7.4
    """
    buffer = SlidingWindowBuffer(maxlen=maxlen)
    
    for idx, measurement in enumerate(measurements):
        buffer.append(measurement)
        
        # Calculate expected average based on current buffer contents
        current_contents = list(buffer)
        expected_average = sum(current_contents) / len(current_contents)
        
        # Get actual moving average
        actual_average = buffer.get_moving_average()
        
        # Property: Moving average equals sum of current contents / current size
        assert actual_average is not None, \
            f"Moving average should not be None at index {idx}"
        assert actual_average == pytest.approx(expected_average, rel=1e-9, abs=1e-9), \
            f"At index {idx}: Moving average {actual_average} != expected {expected_average}"
        
        # Additional property: Average should be within range of current buffer
        if len(current_contents) > 0:
            min_val = min(current_contents)
            max_val = max(current_contents)
            assert min_val <= actual_average <= max_val, \
                f"At index {idx}: Average {actual_average} outside range [{min_val}, {max_val}]"


def test_moving_average_empty_buffer():
    """Test that moving average returns None for empty buffer."""
    buffer = SlidingWindowBuffer(maxlen=10)
    
    # Empty buffer should return None
    assert buffer.get_moving_average() is None, \
        "Moving average should be None for empty buffer"


@settings(max_examples=100)
@given(
    ping_results=st.lists(
        st.booleans(),  # True = success, False = failure
        min_size=1,
        max_size=100
    )
)
def test_property_packet_loss_rate_calculation(ping_results):
    """
    Feature: lite-sdwan-routing, Property 4: Packet Loss Rate Calculation
    
    For any sequence of ping attempts with some failures, the calculated loss rate
    should be between 0.0 and 1.0 (inclusive) and equal to failed_pings / total_pings.
    
    Validates: Requirements 2.3
    """
    # Create a prober with a single peer
    prober = Prober(peer_ips=["10.254.0.2"], interval=5, timeout=2.0, window_size=len(ping_results))
    
    # Simulate ping results
    with patch('agent.prober.ping3.ping') as mock_ping:
        for is_success in ping_results:
            if is_success:
                # Successful ping - return some RTT value
                mock_ping.return_value = 0.050  # 50 ms
            else:
                # Failed ping - return None (timeout)
                mock_ping.return_value = None
            
            prober.probe_all()
    
    # Get smoothed metrics
    metrics = prober.get_smoothed_metrics()
    assert len(metrics) == 1, "Should have metrics for one peer"
    
    loss_rate = metrics[0].loss_rate
    
    # Property 1: Loss rate must be between 0.0 and 1.0 (inclusive)
    assert 0.0 <= loss_rate <= 1.0, \
        f"Loss rate {loss_rate} is outside valid range [0.0, 1.0]"
    
    # Property 2: Loss rate should equal failed_pings / total_pings
    total_pings = len(ping_results)
    failed_pings = sum(1 for result in ping_results if not result)
    expected_loss_rate = failed_pings / total_pings
    
    assert loss_rate == pytest.approx(expected_loss_rate, rel=1e-9, abs=1e-9), \
        f"Loss rate {loss_rate} != expected {expected_loss_rate} " \
        f"(failed={failed_pings}, total={total_pings})"
    
    # Property 3: Edge cases verification
    if all(ping_results):
        # All successful - loss rate should be exactly 0.0
        assert loss_rate == 0.0, "Loss rate should be 0.0 when all pings succeed"
    
    if not any(ping_results):
        # All failed - loss rate should be exactly 1.0
        assert loss_rate == 1.0, "Loss rate should be 1.0 when all pings fail"
