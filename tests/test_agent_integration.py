"""
Integration tests for Agent main program.

Tests the coordination between Prober, Executor, and Controller client.
"""

import pytest
import time
from unittest.mock import Mock, patch, MagicMock
from datetime import datetime

from agent.main import Agent, AgentState
from agent.client import ControllerClient, RetryExhausted
from config.parser import AgentConfig
from models import TelemetryRequest, RouteConfig, Metric


class TestAgentState:
    """Test thread-safe state management."""
    
    def test_metrics_storage(self):
        """Test storing and retrieving metrics."""
        state = AgentState()
        
        metrics = [
            Metric(target_ip="10.254.0.2", rtt_ms=35.5, loss_rate=0.0),
            Metric(target_ip="10.254.0.3", rtt_ms=150.0, loss_rate=0.05)
        ]
        
        state.set_metrics(metrics)
        retrieved = state.get_metrics()
        
        assert retrieved == metrics
    
    def test_fallback_mode_toggle(self):
        """Test fallback mode state management."""
        state = AgentState()
        
        assert not state.is_in_fallback_mode()
        
        state.set_fallback_mode(True)
        assert state.is_in_fallback_mode()
        
        state.set_fallback_mode(False)
        assert not state.is_in_fallback_mode()


class TestControllerClient:
    """Test HTTP client functionality."""
    
    def test_send_telemetry_success(self):
        """Test successful telemetry sending."""
        client = ControllerClient("http://localhost:8000", timeout=5)
        
        telemetry = TelemetryRequest(
            agent_id="test_agent",
            timestamp=int(datetime.now().timestamp()),
            metrics=[
                Metric(target_ip="10.254.0.2", rtt_ms=35.5, loss_rate=0.0)
            ]
        )
        
        with patch('requests.post') as mock_post:
            mock_response = Mock()
            mock_response.status_code = 200
            mock_post.return_value = mock_response
            
            result = client.send_telemetry(telemetry)
            
            assert result is True
            mock_post.assert_called_once()
    
    def test_send_telemetry_failure(self):
        """Test telemetry sending failure."""
        client = ControllerClient("http://localhost:8000", timeout=5)
        
        telemetry = TelemetryRequest(
            agent_id="test_agent",
            timestamp=int(datetime.now().timestamp()),
            metrics=[
                Metric(target_ip="10.254.0.2", rtt_ms=35.5, loss_rate=0.0)
            ]
        )
        
        with patch('requests.post') as mock_post:
            mock_response = Mock()
            mock_response.status_code = 500
            mock_response.text = "Internal Server Error"
            mock_post.return_value = mock_response
            
            result = client.send_telemetry(telemetry)
            
            assert result is False
    
    def test_fetch_routes_success(self):
        """Test successful route fetching."""
        client = ControllerClient("http://localhost:8000", timeout=5)
        
        with patch('requests.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 200
            mock_response.json.return_value = {
                "routes": [
                    {
                        "dst_cidr": "10.254.0.3/32",
                        "next_hop": "10.254.0.2",
                        "reason": "optimized_path"
                    }
                ]
            }
            mock_get.return_value = mock_response
            
            routes = client.fetch_routes("test_agent")
            
            assert routes is not None
            assert len(routes) == 1
            assert routes[0].dst_cidr == "10.254.0.3/32"
            assert routes[0].next_hop == "10.254.0.2"
    
    def test_fetch_routes_not_found(self):
        """Test route fetching when agent not found."""
        client = ControllerClient("http://localhost:8000", timeout=5)
        
        with patch('requests.get') as mock_get:
            mock_response = Mock()
            mock_response.status_code = 404
            mock_get.return_value = mock_response
            
            routes = client.fetch_routes("unknown_agent")
            
            assert routes is None
    
    def test_retry_with_backoff(self):
        """Test exponential backoff retry mechanism."""
        client = ControllerClient(
            "http://localhost:8000",
            timeout=5,
            retry_attempts=3,
            retry_backoff=[0.1, 0.2, 0.4]  # Shorter delays for testing
        )
        
        telemetry = TelemetryRequest(
            agent_id="test_agent",
            timestamp=int(datetime.now().timestamp()),
            metrics=[
                Metric(target_ip="10.254.0.2", rtt_ms=35.5, loss_rate=0.0)
            ]
        )
        
        with patch('requests.post') as mock_post:
            # First two attempts fail, third succeeds
            mock_response_fail = Mock()
            mock_response_fail.status_code = 500
            
            mock_response_success = Mock()
            mock_response_success.status_code = 200
            
            mock_post.side_effect = [
                mock_response_fail,
                mock_response_fail,
                mock_response_success
            ]
            
            start_time = time.time()
            result = client.send_telemetry_with_retry(telemetry)
            elapsed = time.time() - start_time
            
            assert result is True
            assert mock_post.call_count == 3
            # Should have waited at least 0.1 + 0.2 = 0.3 seconds
            assert elapsed >= 0.3
    
    def test_retry_exhausted(self):
        """Test retry exhaustion after all attempts fail."""
        client = ControllerClient(
            "http://localhost:8000",
            timeout=5,
            retry_attempts=3,
            retry_backoff=[0.1, 0.2, 0.4]
        )
        
        telemetry = TelemetryRequest(
            agent_id="test_agent",
            timestamp=int(datetime.now().timestamp()),
            metrics=[
                Metric(target_ip="10.254.0.2", rtt_ms=35.5, loss_rate=0.0)
            ]
        )
        
        with patch('requests.post') as mock_post:
            # All attempts fail
            mock_response = Mock()
            mock_response.status_code = 500
            mock_post.return_value = mock_response
            
            result = client.send_telemetry_with_retry(telemetry)
            
            assert result is False
            assert mock_post.call_count == 3


class TestAgentInitialization:
    """Test Agent initialization and configuration."""
    
    def test_agent_initialization(self):
        """Test Agent initializes with valid configuration."""
        config_dict = {
            'agent_id': 'test_agent',
            'controller': {
                'url': 'http://localhost:8000',
                'timeout': 5
            },
            'probe': {
                'interval': 5,
                'timeout': 2,
                'window_size': 10
            },
            'sync': {
                'interval': 10,
                'retry_attempts': 3,
                'retry_backoff': [1, 2, 4]
            },
            'network': {
                'wg_interface': 'wg0',
                'subnet': '10.254.0.0/24',
                'peer_ips': ['10.254.0.2', '10.254.0.3']
            }
        }
        
        config = AgentConfig(config_dict)
        agent = Agent(config)
        
        assert agent.config.agent_id == 'test_agent'
        assert agent.prober is not None
        assert agent.executor is not None
        assert agent.client is not None
        assert agent.state is not None
        assert not agent.running


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
