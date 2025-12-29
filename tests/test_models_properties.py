"""
Property-based tests for data models.
Tests serialization, validation, and data integrity properties using Hypothesis.
"""

import json
from hypothesis import given, settings, strategies as st
from models import Metric, TelemetryRequest


# Hypothesis strategies for generating test data

@st.composite
def valid_ip_address(draw):
    """Generate valid IP addresses in the 10.254.0.0/24 subnet."""
    last_octet = draw(st.integers(min_value=1, max_value=254))
    return f"10.254.0.{last_octet}"


@st.composite
def metric_strategy(draw):
    """Generate valid Metric instances."""
    target_ip = draw(valid_ip_address())
    # RTT can be None (unreachable) or a positive float
    rtt_ms = draw(st.one_of(
        st.none(),
        st.floats(min_value=0.0, max_value=5000.0, allow_nan=False, allow_infinity=False)
    ))
    # Loss rate must be between 0.0 and 1.0
    loss_rate = draw(st.floats(min_value=0.0, max_value=1.0, allow_nan=False, allow_infinity=False))
    
    return Metric(target_ip=target_ip, rtt_ms=rtt_ms, loss_rate=loss_rate)


@st.composite
def telemetry_request_strategy(draw):
    """Generate valid TelemetryRequest instances."""
    # Agent ID: alphanumeric string with underscores
    agent_id = draw(st.text(
        alphabet=st.characters(whitelist_categories=('Lu', 'Ll', 'Nd'), whitelist_characters='_-'),
        min_size=1,
        max_size=50
    ))
    # Timestamp: positive integer (Unix timestamp)
    timestamp = draw(st.integers(min_value=1, max_value=2**31 - 1))
    # Metrics: list of 1-10 metrics
    metrics = draw(st.lists(metric_strategy(), min_size=1, max_size=10))
    
    return TelemetryRequest(agent_id=agent_id, timestamp=timestamp, metrics=metrics)


# Property-based tests

@settings(max_examples=100)
@given(telemetry=telemetry_request_strategy())
def test_property_telemetry_serialization_round_trip(telemetry):
    """
    Feature: lite-sdwan-routing, Property 6: Telemetry Serialization Round-Trip
    
    For any valid telemetry data structure, serializing to JSON and then 
    deserializing should produce an equivalent data structure.
    
    Validates: Requirements 3.1
    """
    # Serialize to JSON
    json_str = telemetry.model_dump_json()
    
    # Verify it's valid JSON
    json_dict = json.loads(json_str)
    assert isinstance(json_dict, dict)
    
    # Deserialize back to TelemetryRequest
    deserialized = TelemetryRequest.model_validate_json(json_str)
    
    # Verify equivalence
    assert deserialized.agent_id == telemetry.agent_id
    assert deserialized.timestamp == telemetry.timestamp
    assert len(deserialized.metrics) == len(telemetry.metrics)
    
    # Check each metric
    for original_metric, deserialized_metric in zip(telemetry.metrics, deserialized.metrics):
        assert deserialized_metric.target_ip == original_metric.target_ip
        assert deserialized_metric.loss_rate == original_metric.loss_rate
        
        # Handle None RTT values
        if original_metric.rtt_ms is None:
            assert deserialized_metric.rtt_ms is None
        else:
            # For float comparison, use approximate equality
            assert abs(deserialized_metric.rtt_ms - original_metric.rtt_ms) < 0.001
    
    # Alternative: use Pydantic's equality comparison
    # This should work if all fields match
    assert deserialized == telemetry


@settings(max_examples=100)
@given(telemetry=telemetry_request_strategy())
def test_property_telemetry_payload_completeness(telemetry):
    """
    Feature: lite-sdwan-routing, Property 7: Telemetry Payload Completeness
    
    For any telemetry payload sent to the Controller, it should contain 
    agent_id, timestamp, and metrics fields with all target nodes included.
    
    Validates: Requirements 3.3
    """
    # Serialize to JSON to simulate what would be sent over the network
    json_str = telemetry.model_dump_json()
    json_dict = json.loads(json_str)
    
    # Verify all required fields are present
    assert "agent_id" in json_dict, "Telemetry payload must contain agent_id"
    assert "timestamp" in json_dict, "Telemetry payload must contain timestamp"
    assert "metrics" in json_dict, "Telemetry payload must contain metrics"
    
    # Verify agent_id is not empty
    assert json_dict["agent_id"], "agent_id must not be empty"
    
    # Verify timestamp is a positive integer
    assert isinstance(json_dict["timestamp"], int), "timestamp must be an integer"
    assert json_dict["timestamp"] > 0, "timestamp must be positive"
    
    # Verify metrics is a list with at least one entry
    assert isinstance(json_dict["metrics"], list), "metrics must be a list"
    assert len(json_dict["metrics"]) >= 1, "metrics must contain at least one target node"
    
    # Verify each metric has required fields
    for metric in json_dict["metrics"]:
        assert "target_ip" in metric, "Each metric must contain target_ip"
        assert "loss_rate" in metric, "Each metric must contain loss_rate"
        # rtt_ms can be None (for unreachable nodes), but the field must exist
        assert "rtt_ms" in metric, "Each metric must contain rtt_ms field"
        
        # Verify target_ip is not empty
        assert metric["target_ip"], "target_ip must not be empty"
        
        # Verify loss_rate is in valid range
        assert isinstance(metric["loss_rate"], (int, float)), "loss_rate must be numeric"
        assert 0.0 <= metric["loss_rate"] <= 1.0, "loss_rate must be between 0.0 and 1.0"
