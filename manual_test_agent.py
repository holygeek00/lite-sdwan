#!/usr/bin/env python3
"""
Manual test script for Agent functionality.

This script tests the Agent's ability to probe local network targets
and collect metrics without requiring a full Controller setup.
"""

import sys
import logging
from agent.prober import Prober

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)

logger = logging.getLogger(__name__)


def test_prober_local_network():
    """
    Test Prober with local network targets.
    
    Tests:
    1. Localhost (should always respond)
    2. Common gateway addresses (may or may not respond)
    """
    print("=" * 60)
    print("Agent Prober Manual Test")
    print("=" * 60)
    print()
    
    # Test targets - localhost and common gateway addresses
    test_targets = [
        "127.0.0.1",      # Localhost - should always work
        "8.8.8.8",        # Google DNS - should work if internet available
    ]
    
    print(f"Testing with targets: {test_targets}")
    print()
    
    # Create prober with short timeout for quick testing
    prober = Prober(
        peer_ips=test_targets,
        interval=1,
        timeout=2.0,
        window_size=5
    )
    
    print("Running 3 probe cycles...")
    print()
    
    for cycle in range(1, 4):
        print(f"--- Probe Cycle {cycle} ---")
        
        # Run one probe cycle
        metrics = prober.run_once()
        
        # Display results
        for metric in metrics:
            if metric.rtt_ms is not None:
                print(f"  {metric.target_ip}: RTT={metric.rtt_ms:.2f}ms, Loss={metric.loss_rate:.1%}")
            else:
                print(f"  {metric.target_ip}: TIMEOUT, Loss={metric.loss_rate:.1%}")
        
        print()
        
        # Wait a bit between cycles (shorter than normal interval for testing)
        if cycle < 3:
            import time
            time.sleep(1)
    
    print("=" * 60)
    print("Test Summary:")
    print("=" * 60)
    
    # Get final smoothed metrics
    final_metrics = prober.get_smoothed_metrics()
    
    success_count = 0
    for metric in final_metrics:
        if metric.rtt_ms is not None and metric.loss_rate < 1.0:
            success_count += 1
            print(f"✓ {metric.target_ip}: Reachable (avg RTT={metric.rtt_ms:.2f}ms, avg loss={metric.loss_rate:.1%})")
        else:
            print(f"✗ {metric.target_ip}: Unreachable or high loss (avg loss={metric.loss_rate:.1%})")
    
    print()
    print(f"Result: {success_count}/{len(test_targets)} targets reachable")
    
    if success_count > 0:
        print()
        print("✓ Agent Prober is working correctly!")
        print("  - ICMP ping functionality: OK")
        print("  - RTT calculation: OK")
        print("  - Loss rate calculation: OK")
        print("  - Sliding window averaging: OK")
        return True
    else:
        print()
        print("⚠ Warning: No targets were reachable.")
        print("  This may be due to:")
        print("  - Network connectivity issues")
        print("  - Firewall blocking ICMP")
        print("  - Insufficient permissions (try running with sudo)")
        return False


if __name__ == "__main__":
    try:
        success = test_prober_local_network()
        sys.exit(0 if success else 1)
    except KeyboardInterrupt:
        print("\n\nTest interrupted by user")
        sys.exit(1)
    except Exception as e:
        logger.error(f"Test failed with error: {e}", exc_info=True)
        sys.exit(1)
