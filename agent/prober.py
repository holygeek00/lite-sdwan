"""
Prober module for Agent - handles network quality measurements.

This module implements ICMP ping probing and sliding window buffering
for network quality metrics collection.
"""

import time
import logging
from typing import List, Dict, Optional
from collections import deque
import ping3

from models import Metric


logger = logging.getLogger(__name__)


class SlidingWindowBuffer:
    """
    Fixed-size sliding window buffer for storing measurements.
    
    Implements a FIFO buffer that automatically evicts oldest entries
    when the buffer reaches maximum capacity.
    
    Attributes:
        maxlen: Maximum number of entries in the buffer
        buffer: Internal deque storing measurements
    """
    
    def __init__(self, maxlen: int = 10):
        """
        Initialize sliding window buffer.
        
        Args:
            maxlen: Maximum buffer size (default: 10)
        """
        if maxlen <= 0:
            raise ValueError("maxlen must be positive")
        self.maxlen = maxlen
        self.buffer = deque(maxlen=maxlen)
    
    def append(self, value: float) -> None:
        """
        Add a measurement to the buffer.
        
        If buffer is at capacity, oldest entry is automatically evicted.
        
        Args:
            value: Measurement value to add
        """
        self.buffer.append(value)
    
    def get_moving_average(self) -> Optional[float]:
        """
        Calculate moving average of all values in buffer.
        
        Returns:
            Moving average, or None if buffer is empty
        """
        if not self.buffer:
            return None
        return sum(self.buffer) / len(self.buffer)
    
    def __len__(self) -> int:
        """Return current buffer size."""
        return len(self.buffer)
    
    def __iter__(self):
        """Allow iteration over buffer contents."""
        return iter(self.buffer)
    
    def clear(self) -> None:
        """Clear all entries from buffer."""
        self.buffer.clear()


class Prober:
    """
    Network quality prober using ICMP ping.
    
    Collects RTT and packet loss metrics for peer nodes using ICMP echo requests.
    Maintains sliding window buffers for smoothing measurements.
    
    Attributes:
        peer_ips: List of peer IP addresses to probe
        interval: Probe interval in seconds
        timeout: Ping timeout in seconds
        window_size: Size of sliding window for averaging
    """
    
    def __init__(
        self,
        peer_ips: List[str],
        interval: int = 5,
        timeout: float = 2.0,
        window_size: int = 10
    ):
        """
        Initialize Prober.
        
        Args:
            peer_ips: List of peer IP addresses to probe
            interval: Probe interval in seconds (default: 5)
            timeout: Ping timeout in seconds (default: 2.0)
            window_size: Sliding window size for averaging (default: 10)
        """
        self.peer_ips = peer_ips
        self.interval = interval
        self.timeout = timeout
        self.window_size = window_size
        
        # Sliding window buffers for each peer: {ip: {"rtt": buffer, "loss": buffer}}
        self.buffers: Dict[str, Dict[str, SlidingWindowBuffer]] = {}
        for ip in peer_ips:
            self.buffers[ip] = {
                "rtt": SlidingWindowBuffer(maxlen=window_size),
                "loss": SlidingWindowBuffer(maxlen=window_size)
            }
        
        logger.info(f"Prober initialized with {len(peer_ips)} peers, "
                   f"interval={interval}s, timeout={timeout}s")
    
    def probe_once(self, target_ip: str) -> Dict[str, any]:
        """
        Send a single ICMP ping to target and measure response.
        
        Args:
            target_ip: Target IP address to probe
        
        Returns:
            Dictionary with keys:
                - target: Target IP address
                - rtt_ms: RTT in milliseconds (None if timeout)
                - loss: Loss indicator (1.0 for timeout, 0.0 for success)
        """
        try:
            # ping3.ping returns RTT in seconds, or None on timeout/error
            rtt_seconds = ping3.ping(target_ip, timeout=self.timeout)
            
            if rtt_seconds is None:
                # Timeout or unreachable
                logger.debug(f"Ping to {target_ip} timed out")
                return {
                    "target": target_ip,
                    "rtt_ms": None,
                    "loss": 1.0
                }
            else:
                # Successful ping
                rtt_ms = rtt_seconds * 1000  # Convert to milliseconds
                logger.debug(f"Ping to {target_ip}: {rtt_ms:.2f}ms")
                return {
                    "target": target_ip,
                    "rtt_ms": rtt_ms,
                    "loss": 0.0
                }
        except Exception as e:
            # Handle any unexpected errors
            logger.error(f"Error probing {target_ip}: {e}")
            return {
                "target": target_ip,
                "rtt_ms": None,
                "loss": 1.0
            }
    
    def probe_all(self) -> List[Dict[str, any]]:
        """
        Probe all peer nodes and return raw results.
        
        Returns:
            List of probe results (one per peer)
        """
        results = []
        for peer_ip in self.peer_ips:
            result = self.probe_once(peer_ip)
            results.append(result)
            
            # Update sliding window buffers
            if result["rtt_ms"] is not None:
                self.buffers[peer_ip]["rtt"].append(result["rtt_ms"])
            self.buffers[peer_ip]["loss"].append(result["loss"])
        
        return results
    
    def get_smoothed_metrics(self) -> List[Metric]:
        """
        Get smoothed metrics using moving averages from sliding windows.
        
        Returns:
            List of Metric objects with averaged values
        """
        metrics = []
        
        for peer_ip in self.peer_ips:
            rtt_buffer = self.buffers[peer_ip]["rtt"]
            loss_buffer = self.buffers[peer_ip]["loss"]
            
            # Calculate moving averages
            avg_rtt = rtt_buffer.get_moving_average()
            avg_loss = loss_buffer.get_moving_average()
            
            # If we have no RTT measurements (all timeouts), avg_loss should be 1.0
            if avg_loss is None:
                avg_loss = 0.0  # No data yet
            
            metric = Metric(
                target_ip=peer_ip,
                rtt_ms=avg_rtt,
                loss_rate=avg_loss
            )
            metrics.append(metric)
        
        return metrics
    
    def run_once(self) -> List[Metric]:
        """
        Run one probe cycle and return smoothed metrics.
        
        This is the main method to call for getting current network metrics.
        
        Returns:
            List of Metric objects with smoothed values
        """
        self.probe_all()
        return self.get_smoothed_metrics()
    
    def run_loop(self) -> None:
        """
        Run continuous probing loop (blocking).
        
        This method runs indefinitely, probing all peers at the configured interval.
        Intended to be run in a separate thread.
        """
        logger.info("Starting probe loop")
        while True:
            try:
                metrics = self.run_once()
                logger.info(f"Probe cycle complete: {len(metrics)} metrics collected")
                time.sleep(self.interval)
            except KeyboardInterrupt:
                logger.info("Probe loop interrupted")
                break
            except Exception as e:
                logger.error(f"Error in probe loop: {e}", exc_info=True)
                time.sleep(self.interval)
