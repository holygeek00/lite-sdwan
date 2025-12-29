"""
Configuration parser for Lite SD-WAN Routing System.
"""

import yaml
from pathlib import Path
from typing import Dict, Any, List


class ConfigurationError(Exception):
    """Raised when configuration is invalid or missing required fields."""
    pass


class AgentConfig:
    """Agent configuration parser and validator."""
    
    REQUIRED_FIELDS = {
        'agent_id': str,
        'controller.url': str,
        'probe.interval': (int, float),
        'sync.interval': (int, float),
        'network.wg_interface': str,
        'network.peer_ips': list,
    }
    
    def __init__(self, config_dict: Dict[str, Any]):
        self._config = config_dict
        self._validate()
    
    def _validate(self):
        """Validate that all required fields are present and have correct types."""
        for field_path, expected_type in self.REQUIRED_FIELDS.items():
            value = self._get_nested_value(field_path)
            if value is None:
                raise ConfigurationError(f"Missing required field: {field_path}")
            
            if not isinstance(value, expected_type):
                raise ConfigurationError(
                    f"Field {field_path} has incorrect type. "
                    f"Expected {expected_type}, got {type(value)}"
                )
    
    def _get_nested_value(self, field_path: str) -> Any:
        """Get value from nested dictionary using dot notation."""
        keys = field_path.split('.')
        value = self._config
        for key in keys:
            if isinstance(value, dict):
                value = value.get(key)
            else:
                return None
        return value
    
    @property
    def agent_id(self) -> str:
        return self._config['agent_id']
    
    @property
    def controller_url(self) -> str:
        return self._config['controller']['url']
    
    @property
    def controller_timeout(self) -> int:
        return self._config.get('controller', {}).get('timeout', 5)
    
    @property
    def probe_interval(self) -> int:
        return self._config['probe']['interval']
    
    @property
    def probe_timeout(self) -> int:
        return self._config.get('probe', {}).get('timeout', 2)
    
    @property
    def probe_window_size(self) -> int:
        return self._config.get('probe', {}).get('window_size', 10)
    
    @property
    def sync_interval(self) -> int:
        return self._config['sync']['interval']
    
    @property
    def sync_retry_attempts(self) -> int:
        return self._config.get('sync', {}).get('retry_attempts', 3)
    
    @property
    def sync_retry_backoff(self) -> List[int]:
        return self._config.get('sync', {}).get('retry_backoff', [1, 2, 4])
    
    @property
    def wg_interface(self) -> str:
        return self._config['network']['wg_interface']
    
    @property
    def subnet(self) -> str:
        return self._config.get('network', {}).get('subnet', '10.254.0.0/24')
    
    @property
    def peer_ips(self) -> List[str]:
        return self._config['network']['peer_ips']
    
    @classmethod
    def from_file(cls, config_path: str) -> 'AgentConfig':
        """Load configuration from YAML file."""
        path = Path(config_path)
        if not path.exists():
            raise ConfigurationError(f"Configuration file not found: {config_path}")
        
        try:
            with open(path, 'r') as f:
                config_dict = yaml.safe_load(f)
        except yaml.YAMLError as e:
            raise ConfigurationError(f"Failed to parse YAML: {e}")
        
        if config_dict is None:
            raise ConfigurationError("Configuration file is empty")
        
        return cls(config_dict)


class ControllerConfig:
    """Controller configuration parser and validator."""
    
    REQUIRED_FIELDS = {
        'server.listen_address': str,
        'server.port': int,
        'algorithm.penalty_factor': (int, float),
        'algorithm.hysteresis': (int, float),
    }
    
    def __init__(self, config_dict: Dict[str, Any]):
        self._config = config_dict
        self._validate()
    
    def _validate(self):
        """Validate that all required fields are present and have correct types."""
        for field_path, expected_type in self.REQUIRED_FIELDS.items():
            value = self._get_nested_value(field_path)
            if value is None:
                raise ConfigurationError(f"Missing required field: {field_path}")
            
            if not isinstance(value, expected_type):
                raise ConfigurationError(
                    f"Field {field_path} has incorrect type. "
                    f"Expected {expected_type}, got {type(value)}"
                )
    
    def _get_nested_value(self, field_path: str) -> Any:
        """Get value from nested dictionary using dot notation."""
        keys = field_path.split('.')
        value = self._config
        for key in keys:
            if isinstance(value, dict):
                value = value.get(key)
            else:
                return None
        return value
    
    @property
    def listen_address(self) -> str:
        return self._config['server']['listen_address']
    
    @property
    def port(self) -> int:
        return self._config['server']['port']
    
    @property
    def penalty_factor(self) -> float:
        return self._config['algorithm']['penalty_factor']
    
    @property
    def hysteresis(self) -> float:
        return self._config['algorithm']['hysteresis']
    
    @property
    def stale_threshold(self) -> int:
        return self._config.get('topology', {}).get('stale_threshold', 60)
    
    @classmethod
    def from_file(cls, config_path: str) -> 'ControllerConfig':
        """Load configuration from YAML file."""
        path = Path(config_path)
        if not path.exists():
            raise ConfigurationError(f"Configuration file not found: {config_path}")
        
        try:
            with open(path, 'r') as f:
                config_dict = yaml.safe_load(f)
        except yaml.YAMLError as e:
            raise ConfigurationError(f"Failed to parse YAML: {e}")
        
        if config_dict is None:
            raise ConfigurationError("Configuration file is empty")
        
        return cls(config_dict)
