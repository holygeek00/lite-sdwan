"""
HTTP client module for Agent - handles communication with Controller.

This module implements HTTP client functionality for sending telemetry
and fetching route configurations from the Controller.
"""

import logging
import time
from typing import List, Optional, Callable, Any
import requests

from models import TelemetryRequest, RouteConfig, RouteResponse


logger = logging.getLogger(__name__)


class RetryExhausted(Exception):
    """Raised when all retry attempts have been exhausted."""
    pass


class ControllerClient:
    """
    HTTP client for communicating with the Controller.
    
    Handles sending telemetry data and fetching route configurations
    via REST API endpoints. Implements exponential backoff retry logic.
    
    Attributes:
        controller_url: Base URL of the Controller (e.g., "http://10.254.0.1:8000")
        timeout: HTTP request timeout in seconds
        retry_attempts: Maximum number of retry attempts
        retry_backoff: List of backoff delays in seconds (e.g., [1, 2, 4])
    """
    
    def __init__(
        self,
        controller_url: str,
        timeout: int = 5,
        retry_attempts: int = 3,
        retry_backoff: List[int] = None
    ):
        """
        Initialize Controller client.
        
        Args:
            controller_url: Base URL of the Controller
            timeout: HTTP request timeout in seconds (default: 5)
            retry_attempts: Maximum retry attempts (default: 3)
            retry_backoff: Backoff delays in seconds (default: [1, 2, 4])
        """
        self.controller_url = controller_url.rstrip('/')
        self.timeout = timeout
        self.retry_attempts = retry_attempts
        self.retry_backoff = retry_backoff or [1, 2, 4]
        
        logger.info(f"ControllerClient initialized: url={controller_url}, "
                   f"timeout={timeout}s, retry_attempts={retry_attempts}")
    
    def _retry_with_backoff(self, operation: Callable[[], Any], operation_name: str) -> Any:
        """
        Execute an operation with exponential backoff retry logic.
        
        Implements retry mechanism with configurable backoff delays.
        After all retries are exhausted, raises RetryExhausted exception.
        
        Args:
            operation: Callable that performs the operation
            operation_name: Name of operation for logging
        
        Returns:
            Result from successful operation
        
        Raises:
            RetryExhausted: If all retry attempts fail
        """
        last_exception = None
        
        for attempt in range(self.retry_attempts):
            try:
                result = operation()
                if result is not None:
                    return result
                
                # Operation returned None (failure), retry
                logger.warning(f"{operation_name} failed (attempt {attempt + 1}/{self.retry_attempts})")
                
            except Exception as e:
                last_exception = e
                logger.warning(f"{operation_name} raised exception (attempt {attempt + 1}/{self.retry_attempts}): {e}")
            
            # Apply backoff delay before next retry (except after last attempt)
            if attempt < self.retry_attempts - 1:
                delay = self.retry_backoff[min(attempt, len(self.retry_backoff) - 1)]
                logger.debug(f"Backing off for {delay}s before retry")
                time.sleep(delay)
        
        # All retries exhausted
        logger.error(f"{operation_name} failed after {self.retry_attempts} attempts")
        raise RetryExhausted(f"{operation_name} failed after {self.retry_attempts} attempts")
    
    def send_telemetry_with_retry(self, telemetry: TelemetryRequest) -> bool:
        """
        Send telemetry data with exponential backoff retry.
        
        Args:
            telemetry: TelemetryRequest object containing probe metrics
        
        Returns:
            True if telemetry was sent successfully
        
        Raises:
            RetryExhausted: If all retry attempts fail
        """
        def operation():
            success = self.send_telemetry(telemetry)
            return success if success else None
        
        try:
            result = self._retry_with_backoff(operation, "send_telemetry")
            return result
        except RetryExhausted:
            return False
    
    def fetch_routes_with_retry(self, agent_id: str) -> Optional[List[RouteConfig]]:
        """
        Fetch route configurations with exponential backoff retry.
        
        Args:
            agent_id: Agent identifier to fetch routes for
        
        Returns:
            List of RouteConfig objects, or None if all retries fail
        
        Raises:
            RetryExhausted: If all retry attempts fail
        """
        def operation():
            return self.fetch_routes(agent_id)
        
        try:
            result = self._retry_with_backoff(operation, "fetch_routes")
            return result
        except RetryExhausted:
            return None
    
    def send_telemetry(self, telemetry: TelemetryRequest) -> bool:
        """
        Send telemetry data to Controller via POST /api/v1/telemetry.
        
        Args:
            telemetry: TelemetryRequest object containing probe metrics
        
        Returns:
            True if telemetry was sent successfully, False otherwise
        """
        endpoint = f"{self.controller_url}/api/v1/telemetry"
        
        try:
            # Convert Pydantic model to JSON
            payload = telemetry.model_dump()
            
            # Send POST request
            response = requests.post(
                endpoint,
                json=payload,
                timeout=self.timeout
            )
            
            # Check response status
            if response.status_code == 200:
                logger.debug(f"Telemetry sent successfully to {endpoint}")
                return True
            else:
                logger.error(f"Controller returned status {response.status_code}: {response.text}")
                return False
                
        except requests.exceptions.Timeout:
            logger.error(f"Timeout sending telemetry to {endpoint}")
            return False
        except requests.exceptions.ConnectionError as e:
            logger.error(f"Connection error sending telemetry: {e}")
            return False
        except requests.exceptions.RequestException as e:
            logger.error(f"Request error sending telemetry: {e}")
            return False
        except Exception as e:
            logger.error(f"Unexpected error sending telemetry: {e}", exc_info=True)
            return False
    
    def fetch_routes(self, agent_id: str) -> Optional[List[RouteConfig]]:
        """
        Fetch route configurations from Controller via GET /api/v1/routes.
        
        Args:
            agent_id: Agent identifier to fetch routes for
        
        Returns:
            List of RouteConfig objects, or None if request failed
        """
        endpoint = f"{self.controller_url}/api/v1/routes"
        
        try:
            # Send GET request with query parameter
            response = requests.get(
                endpoint,
                params={"agent_id": agent_id},
                timeout=self.timeout
            )
            
            # Check response status
            if response.status_code == 200:
                # Parse JSON response
                data = response.json()
                
                # Validate response structure
                if "routes" not in data:
                    logger.error(f"Invalid response format: missing 'routes' field")
                    return None
                
                # Parse routes using Pydantic
                route_response = RouteResponse(**data)
                logger.debug(f"Fetched {len(route_response.routes)} routes from Controller")
                return route_response.routes
                
            elif response.status_code == 404:
                logger.warning(f"Agent {agent_id} not found in Controller")
                return None
            else:
                logger.error(f"Controller returned status {response.status_code}: {response.text}")
                return None
                
        except requests.exceptions.Timeout:
            logger.error(f"Timeout fetching routes from {endpoint}")
            return None
        except requests.exceptions.ConnectionError as e:
            logger.error(f"Connection error fetching routes: {e}")
            return None
        except requests.exceptions.RequestException as e:
            logger.error(f"Request error fetching routes: {e}")
            return None
        except Exception as e:
            logger.error(f"Unexpected error fetching routes: {e}", exc_info=True)
            return None
