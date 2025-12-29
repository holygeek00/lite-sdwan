"""
Main Agent program for Lite SD-WAN Routing System.

This module coordinates the Prober and Executor threads, handles
communication with the Controller, and implements fallback mode.
"""

import logging
import time
import threading
from typing import Optional
from datetime import datetime

from agent.prober import Prober
from agent.executor import Executor
from agent.client import ControllerClient, RetryExhausted
from config.parser import AgentConfig
from models import TelemetryRequest, Metric


logger = logging.getLogger(__name__)


class AgentState:
    """
    Thread-safe state container for Agent.
    
    Stores shared state between Prober and Executor threads.
    
    Attributes:
        latest_metrics: Most recent probe metrics
        in_fallback_mode: Whether Agent is in fallback mode
        lock: Threading lock for synchronization
    """
    
    def __init__(self):
        self.latest_metrics: Optional[list[Metric]] = None
        self.in_fallback_mode: bool = False
        self.lock = threading.Lock()
    
    def set_metrics(self, metrics: list[Metric]) -> None:
        """Thread-safe setter for metrics."""
        with self.lock:
            self.latest_metrics = metrics
    
    def get_metrics(self) -> Optional[list[Metric]]:
        """Thread-safe getter for metrics."""
        with self.lock:
            return self.latest_metrics
    
    def set_fallback_mode(self, enabled: bool) -> None:
        """Thread-safe setter for fallback mode."""
        with self.lock:
            if self.in_fallback_mode != enabled:
                self.in_fallback_mode = enabled
                logger.info(f"Fallback mode: {'ENABLED' if enabled else 'DISABLED'}")
    
    def is_in_fallback_mode(self) -> bool:
        """Thread-safe getter for fallback mode."""
        with self.lock:
            return self.in_fallback_mode


class Agent:
    """
    Main Agent coordinator.
    
    Manages Prober and Executor threads, handles Controller communication,
    and implements fallback mode logic.
    
    Attributes:
        config: Agent configuration
        prober: Network quality prober
        executor: Route executor
        client: Controller HTTP client
        state: Shared state between threads
    """
    
    def __init__(self, config: AgentConfig):
        """
        Initialize Agent.
        
        Args:
            config: AgentConfig object with all settings
        """
        self.config = config
        self.state = AgentState()
        
        # Initialize components
        self.prober = Prober(
            peer_ips=config.peer_ips,
            interval=config.probe_interval,
            timeout=config.probe_timeout,
            window_size=config.probe_window_size
        )
        
        self.executor = Executor(
            wg_interface=config.wg_interface,
            allowed_subnet=config.subnet
        )
        
        self.client = ControllerClient(
            controller_url=config.controller_url,
            timeout=config.controller_timeout,
            retry_attempts=config.sync_retry_attempts,
            retry_backoff=config.sync_retry_backoff
        )
        
        # Thread control
        self.running = False
        self.prober_thread: Optional[threading.Thread] = None
        self.executor_thread: Optional[threading.Thread] = None
        
        logger.info(f"Agent initialized: agent_id={config.agent_id}")
    
    def _prober_loop(self) -> None:
        """
        Prober thread main loop.
        
        Runs probe cycles at configured interval and stores results in shared state.
        """
        logger.info("Prober thread started")
        
        while self.running:
            try:
                # Run probe cycle
                metrics = self.prober.run_once()
                
                # Store metrics in shared state
                self.state.set_metrics(metrics)
                
                logger.debug(f"Probe cycle complete: {len(metrics)} metrics collected")
                
                # Sleep until next cycle
                time.sleep(self.config.probe_interval)
                
            except Exception as e:
                logger.error(f"Error in prober loop: {e}", exc_info=True)
                time.sleep(self.config.probe_interval)
        
        logger.info("Prober thread stopped")
    
    def _executor_loop(self) -> None:
        """
        Executor thread main loop.
        
        Sends telemetry to Controller, fetches routes, and applies them.
        Implements fallback mode logic when Controller is unreachable.
        """
        logger.info("Executor thread started")
        
        while self.running:
            try:
                # Get latest metrics from shared state
                metrics = self.state.get_metrics()
                
                if metrics is None:
                    logger.debug("No metrics available yet, skipping sync cycle")
                    time.sleep(self.config.sync_interval)
                    continue
                
                # Create telemetry request
                telemetry = TelemetryRequest(
                    agent_id=self.config.agent_id,
                    timestamp=int(datetime.now().timestamp()),
                    metrics=metrics
                )
                
                # Try to send telemetry with retry
                try:
                    success = self.client.send_telemetry_with_retry(telemetry)
                    
                    if not success:
                        # All retries failed, enter fallback mode
                        self._enter_fallback_mode()
                        time.sleep(self.config.sync_interval)
                        continue
                    
                    # Telemetry sent successfully
                    logger.debug("Telemetry sent successfully")
                    
                    # If we were in fallback mode, we've recovered
                    if self.state.is_in_fallback_mode():
                        self._exit_fallback_mode()
                    
                except RetryExhausted:
                    # All retries failed, enter fallback mode
                    self._enter_fallback_mode()
                    time.sleep(self.config.sync_interval)
                    continue
                
                # Fetch routes from Controller
                try:
                    routes = self.client.fetch_routes_with_retry(self.config.agent_id)
                    
                    if routes is None:
                        # Failed to fetch routes, enter fallback mode
                        self._enter_fallback_mode()
                        time.sleep(self.config.sync_interval)
                        continue
                    
                    # Routes fetched successfully
                    logger.debug(f"Fetched {len(routes)} routes from Controller")
                    
                    # Apply routes
                    self.executor.sync_routes(routes)
                    
                except RetryExhausted:
                    # All retries failed, enter fallback mode
                    self._enter_fallback_mode()
                    time.sleep(self.config.sync_interval)
                    continue
                
                # Sleep until next sync cycle
                time.sleep(self.config.sync_interval)
                
            except Exception as e:
                logger.error(f"Error in executor loop: {e}", exc_info=True)
                time.sleep(self.config.sync_interval)
        
        logger.info("Executor thread stopped")
    
    def _enter_fallback_mode(self) -> None:
        """
        Enter fallback mode.
        
        Flushes all dynamic routes and relies on WireGuard default routing.
        """
        if not self.state.is_in_fallback_mode():
            logger.warning("Entering fallback mode: Controller unreachable")
            self.state.set_fallback_mode(True)
            
            # Flush all routes
            logger.info("Flushing all dynamic routes")
            self.executor.flush_all_routes()
    
    def _exit_fallback_mode(self) -> None:
        """
        Exit fallback mode.
        
        Called when Controller becomes reachable again.
        """
        if self.state.is_in_fallback_mode():
            logger.info("Exiting fallback mode: Controller reachable")
            self.state.set_fallback_mode(False)
    
    def start(self) -> None:
        """
        Start the Agent.
        
        Launches Prober and Executor threads.
        """
        if self.running:
            logger.warning("Agent already running")
            return
        
        logger.info("Starting Agent")
        self.running = True
        
        # Start prober thread
        self.prober_thread = threading.Thread(
            target=self._prober_loop,
            name="ProberThread",
            daemon=True
        )
        self.prober_thread.start()
        
        # Start executor thread
        self.executor_thread = threading.Thread(
            target=self._executor_loop,
            name="ExecutorThread",
            daemon=True
        )
        self.executor_thread.start()
        
        logger.info("Agent started successfully")
    
    def stop(self) -> None:
        """
        Stop the Agent.
        
        Signals threads to stop and waits for them to finish.
        """
        if not self.running:
            logger.warning("Agent not running")
            return
        
        logger.info("Stopping Agent")
        self.running = False
        
        # Wait for threads to finish
        if self.prober_thread and self.prober_thread.is_alive():
            self.prober_thread.join(timeout=10)
        
        if self.executor_thread and self.executor_thread.is_alive():
            self.executor_thread.join(timeout=10)
        
        logger.info("Agent stopped")
    
    def run(self) -> None:
        """
        Run the Agent (blocking).
        
        Starts the Agent and blocks until interrupted.
        """
        self.start()
        
        try:
            # Keep main thread alive
            while self.running:
                time.sleep(1)
        except KeyboardInterrupt:
            logger.info("Received interrupt signal")
        finally:
            self.stop()


def main():
    """
    Main entry point for Agent program.
    """
    import sys
    
    # Configure logging
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )
    
    # Load configuration
    if len(sys.argv) < 2:
        print("Usage: python -m agent.main <config_file>")
        sys.exit(1)
    
    config_file = sys.argv[1]
    
    try:
        config = AgentConfig.from_file(config_file)
    except Exception as e:
        logger.error(f"Failed to load configuration: {e}")
        sys.exit(1)
    
    # Create and run agent
    agent = Agent(config)
    agent.run()


if __name__ == "__main__":
    main()
