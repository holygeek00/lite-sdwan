"""
Unit tests for configuration parsing.
Tests YAML configuration file parsing and error handling for missing fields.
Requirements: 11.2, 11.3
"""

import pytest
import tempfile
import os
from pathlib import Path
from config.parser import AgentConfig, ControllerConfig, ConfigurationError


class TestAgentConfigParsing:
    """Test Agent configuration parsing."""
    
    def test_valid_agent_config(self):
        """Test parsing a valid agent configuration."""
        config_dict = {
            'agent_id': 'node_test',
            'controller': {
                'url': 'http://10.254.0.1:8000',
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
        
        assert config.agent_id == 'node_test'
        assert config.controller_url == 'http://10.254.0.1:8000'
        assert config.controller_timeout == 5
        assert config.probe_interval == 5
        assert config.probe_timeout == 2
        assert config.probe_window_size == 10
        assert config.sync_interval == 10
        assert config.sync_retry_attempts == 3
        assert config.sync_retry_backoff == [1, 2, 4]
        assert config.wg_interface == 'wg0'
        assert config.subnet == '10.254.0.0/24'
        assert config.peer_ips == ['10.254.0.2', '10.254.0.3']
    
    def test_agent_config_missing_agent_id(self):
        """Test that missing agent_id raises ConfigurationError."""
        config_dict = {
            'controller': {'url': 'http://10.254.0.1:8000'},
            'probe': {'interval': 5},
            'sync': {'interval': 10},
            'network': {'wg_interface': 'wg0', 'peer_ips': []}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            AgentConfig(config_dict)
        
        assert 'agent_id' in str(exc_info.value)
    
    def test_agent_config_missing_controller_url(self):
        """Test that missing controller.url raises ConfigurationError."""
        config_dict = {
            'agent_id': 'node_test',
            'controller': {},
            'probe': {'interval': 5},
            'sync': {'interval': 10},
            'network': {'wg_interface': 'wg0', 'peer_ips': []}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            AgentConfig(config_dict)
        
        assert 'controller.url' in str(exc_info.value)
    
    def test_agent_config_missing_probe_interval(self):
        """Test that missing probe.interval raises ConfigurationError."""
        config_dict = {
            'agent_id': 'node_test',
            'controller': {'url': 'http://10.254.0.1:8000'},
            'probe': {},
            'sync': {'interval': 10},
            'network': {'wg_interface': 'wg0', 'peer_ips': []}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            AgentConfig(config_dict)
        
        assert 'probe.interval' in str(exc_info.value)
    
    def test_agent_config_missing_sync_interval(self):
        """Test that missing sync.interval raises ConfigurationError."""
        config_dict = {
            'agent_id': 'node_test',
            'controller': {'url': 'http://10.254.0.1:8000'},
            'probe': {'interval': 5},
            'sync': {},
            'network': {'wg_interface': 'wg0', 'peer_ips': []}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            AgentConfig(config_dict)
        
        assert 'sync.interval' in str(exc_info.value)
    
    def test_agent_config_missing_wg_interface(self):
        """Test that missing network.wg_interface raises ConfigurationError."""
        config_dict = {
            'agent_id': 'node_test',
            'controller': {'url': 'http://10.254.0.1:8000'},
            'probe': {'interval': 5},
            'sync': {'interval': 10},
            'network': {'peer_ips': []}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            AgentConfig(config_dict)
        
        assert 'network.wg_interface' in str(exc_info.value)
    
    def test_agent_config_missing_peer_ips(self):
        """Test that missing network.peer_ips raises ConfigurationError."""
        config_dict = {
            'agent_id': 'node_test',
            'controller': {'url': 'http://10.254.0.1:8000'},
            'probe': {'interval': 5},
            'sync': {'interval': 10},
            'network': {'wg_interface': 'wg0'}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            AgentConfig(config_dict)
        
        assert 'network.peer_ips' in str(exc_info.value)
    
    def test_agent_config_wrong_type_agent_id(self):
        """Test that wrong type for agent_id raises ConfigurationError."""
        config_dict = {
            'agent_id': 123,  # Should be string
            'controller': {'url': 'http://10.254.0.1:8000'},
            'probe': {'interval': 5},
            'sync': {'interval': 10},
            'network': {'wg_interface': 'wg0', 'peer_ips': []}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            AgentConfig(config_dict)
        
        assert 'incorrect type' in str(exc_info.value).lower()
    
    def test_agent_config_from_file(self):
        """Test loading agent configuration from YAML file."""
        yaml_content = """
agent_id: "node_test"
controller:
  url: "http://10.254.0.1:8000"
  timeout: 5
probe:
  interval: 5
  timeout: 2
  window_size: 10
sync:
  interval: 10
  retry_attempts: 3
  retry_backoff: [1, 2, 4]
network:
  wg_interface: "wg0"
  subnet: "10.254.0.0/24"
  peer_ips:
    - "10.254.0.2"
    - "10.254.0.3"
"""
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(yaml_content)
            temp_path = f.name
        
        try:
            config = AgentConfig.from_file(temp_path)
            assert config.agent_id == 'node_test'
            assert config.controller_url == 'http://10.254.0.1:8000'
            assert config.peer_ips == ['10.254.0.2', '10.254.0.3']
        finally:
            os.unlink(temp_path)
    
    def test_agent_config_from_nonexistent_file(self):
        """Test that loading from nonexistent file raises ConfigurationError."""
        with pytest.raises(ConfigurationError) as exc_info:
            AgentConfig.from_file('/nonexistent/path/config.yaml')
        
        assert 'not found' in str(exc_info.value).lower()
    
    def test_agent_config_from_invalid_yaml(self):
        """Test that invalid YAML raises ConfigurationError."""
        yaml_content = """
agent_id: "node_test"
controller:
  url: "http://10.254.0.1:8000"
  invalid: yaml: syntax: here
"""
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(yaml_content)
            temp_path = f.name
        
        try:
            with pytest.raises(ConfigurationError) as exc_info:
                AgentConfig.from_file(temp_path)
            
            assert 'yaml' in str(exc_info.value).lower()
        finally:
            os.unlink(temp_path)
    
    def test_agent_config_from_empty_file(self):
        """Test that empty YAML file raises ConfigurationError."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write('')
            temp_path = f.name
        
        try:
            with pytest.raises(ConfigurationError) as exc_info:
                AgentConfig.from_file(temp_path)
            
            assert 'empty' in str(exc_info.value).lower()
        finally:
            os.unlink(temp_path)


class TestControllerConfigParsing:
    """Test Controller configuration parsing."""
    
    def test_valid_controller_config(self):
        """Test parsing a valid controller configuration."""
        config_dict = {
            'server': {
                'listen_address': '0.0.0.0',
                'port': 8000
            },
            'algorithm': {
                'penalty_factor': 100,
                'hysteresis': 0.15
            },
            'topology': {
                'stale_threshold': 60
            }
        }
        
        config = ControllerConfig(config_dict)
        
        assert config.listen_address == '0.0.0.0'
        assert config.port == 8000
        assert config.penalty_factor == 100
        assert config.hysteresis == 0.15
        assert config.stale_threshold == 60
    
    def test_controller_config_missing_listen_address(self):
        """Test that missing server.listen_address raises ConfigurationError."""
        config_dict = {
            'server': {'port': 8000},
            'algorithm': {'penalty_factor': 100, 'hysteresis': 0.15}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            ControllerConfig(config_dict)
        
        assert 'server.listen_address' in str(exc_info.value)
    
    def test_controller_config_missing_port(self):
        """Test that missing server.port raises ConfigurationError."""
        config_dict = {
            'server': {'listen_address': '0.0.0.0'},
            'algorithm': {'penalty_factor': 100, 'hysteresis': 0.15}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            ControllerConfig(config_dict)
        
        assert 'server.port' in str(exc_info.value)
    
    def test_controller_config_missing_penalty_factor(self):
        """Test that missing algorithm.penalty_factor raises ConfigurationError."""
        config_dict = {
            'server': {'listen_address': '0.0.0.0', 'port': 8000},
            'algorithm': {'hysteresis': 0.15}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            ControllerConfig(config_dict)
        
        assert 'algorithm.penalty_factor' in str(exc_info.value)
    
    def test_controller_config_missing_hysteresis(self):
        """Test that missing algorithm.hysteresis raises ConfigurationError."""
        config_dict = {
            'server': {'listen_address': '0.0.0.0', 'port': 8000},
            'algorithm': {'penalty_factor': 100}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            ControllerConfig(config_dict)
        
        assert 'algorithm.hysteresis' in str(exc_info.value)
    
    def test_controller_config_wrong_type_port(self):
        """Test that wrong type for server.port raises ConfigurationError."""
        config_dict = {
            'server': {'listen_address': '0.0.0.0', 'port': '8000'},  # Should be int
            'algorithm': {'penalty_factor': 100, 'hysteresis': 0.15}
        }
        
        with pytest.raises(ConfigurationError) as exc_info:
            ControllerConfig(config_dict)
        
        assert 'incorrect type' in str(exc_info.value).lower()
    
    def test_controller_config_from_file(self):
        """Test loading controller configuration from YAML file."""
        yaml_content = """
server:
  listen_address: "0.0.0.0"
  port: 8000
algorithm:
  penalty_factor: 100
  hysteresis: 0.15
topology:
  stale_threshold: 60
"""
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(yaml_content)
            temp_path = f.name
        
        try:
            config = ControllerConfig.from_file(temp_path)
            assert config.listen_address == '0.0.0.0'
            assert config.port == 8000
            assert config.penalty_factor == 100
            assert config.hysteresis == 0.15
        finally:
            os.unlink(temp_path)
    
    def test_controller_config_from_nonexistent_file(self):
        """Test that loading from nonexistent file raises ConfigurationError."""
        with pytest.raises(ConfigurationError) as exc_info:
            ControllerConfig.from_file('/nonexistent/path/config.yaml')
        
        assert 'not found' in str(exc_info.value).lower()
    
    def test_controller_config_from_invalid_yaml(self):
        """Test that invalid YAML raises ConfigurationError."""
        yaml_content = """
server:
  listen_address: "0.0.0.0"
  port: 8000
  invalid: yaml: syntax: here
"""
        
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write(yaml_content)
            temp_path = f.name
        
        try:
            with pytest.raises(ConfigurationError) as exc_info:
                ControllerConfig.from_file(temp_path)
            
            assert 'yaml' in str(exc_info.value).lower()
        finally:
            os.unlink(temp_path)
    
    def test_controller_config_from_empty_file(self):
        """Test that empty YAML file raises ConfigurationError."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yaml', delete=False) as f:
            f.write('')
            temp_path = f.name
        
        try:
            with pytest.raises(ConfigurationError) as exc_info:
                ControllerConfig.from_file(temp_path)
            
            assert 'empty' in str(exc_info.value).lower()
        finally:
            os.unlink(temp_path)
    
    def test_controller_config_default_stale_threshold(self):
        """Test that stale_threshold has a default value when not specified."""
        config_dict = {
            'server': {'listen_address': '0.0.0.0', 'port': 8000},
            'algorithm': {'penalty_factor': 100, 'hysteresis': 0.15}
        }
        
        config = ControllerConfig(config_dict)
        assert config.stale_threshold == 60  # Default value
