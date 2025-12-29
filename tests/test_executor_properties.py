"""
Property-based tests for Executor module.
Tests route command generation, subnet safety, and routing table diff calculation using Hypothesis.
"""

from hypothesis import given, settings, strategies as st
from agent.executor import Executor


# Hypothesis strategies for generating test data

@st.composite
def valid_subnet_ip(draw):
    """Generate valid IP addresses in the 10.254.0.0/24 subnet."""
    last_octet = draw(st.integers(min_value=1, max_value=254))
    return f"10.254.0.{last_octet}"


@st.composite
def route_change_request(draw):
    """
    Generate random route change requirements.
    
    Returns a tuple of (dst_ip, next_hop, operation) where:
    - dst_ip: destination IP address
    - next_hop: next hop IP address (or "direct" for delete operations)
    - operation: "add" or "delete"
    """
    last_octet_dst = draw(st.integers(min_value=1, max_value=254))
    dst_ip = f"10.254.0.{last_octet_dst}"
    
    operation = draw(st.sampled_from(["add", "delete"]))
    
    if operation == "add":
        # For add operations, next_hop is another IP in the subnet
        # Generate a different last octet to ensure next_hop != dst_ip
        last_octet_next = draw(st.integers(min_value=1, max_value=254).filter(lambda x: x != last_octet_dst))
        next_hop = f"10.254.0.{last_octet_next}"
    else:
        # For delete operations, next_hop is "direct"
        next_hop = "direct"
    
    return (dst_ip, next_hop, operation)


# Property-based tests

@settings(max_examples=100)
@given(request=route_change_request())
def test_property_route_command_generation(request):
    """
    Feature: lite-sdwan-routing, Property 19: Route Command Generation
    
    For any route change requirement (add relay or remove relay), the generated 
    ip route command should have correct syntax:
    - Add: `ip route replace <target>/32 via <relay> dev wg0`
    - Delete: `ip route del <target>/32 dev wg0`
    
    Validates: Requirements 6.2, 6.3
    """
    dst_ip, next_hop, operation = request
    executor = Executor(wg_interface="wg0", allowed_subnet="10.254.0.0/24")
    
    if operation == "add":
        # Test add/replace command generation
        command = executor.generate_route_add_command(dst_ip, next_hop)
        
        # Verify command is not None (validation passed)
        assert command is not None, f"Command generation failed for valid IPs: {dst_ip}, {next_hop}"
        
        # Verify command structure
        assert isinstance(command, list), "Command must be a list of strings"
        assert len(command) == 8, f"Add command must have 8 elements, got {len(command)}"
        
        # Verify command syntax: ["ip", "route", "replace", "<dst>/32", "via", "<next_hop>", "dev", "wg0"]
        assert command[0] == "ip", "First element must be 'ip'"
        assert command[1] == "route", "Second element must be 'route'"
        assert command[2] == "replace", "Third element must be 'replace'"
        assert command[3] == f"{dst_ip}/32", f"Fourth element must be '{dst_ip}/32'"
        assert command[4] == "via", "Fifth element must be 'via'"
        assert command[5] == next_hop, f"Sixth element must be '{next_hop}'"
        assert command[6] == "dev", "Seventh element must be 'dev'"
        assert command[7] == "wg0", "Eighth element must be 'wg0'"
        
    else:  # operation == "delete"
        # Test delete command generation
        command = executor.generate_route_del_command(dst_ip)
        
        # Verify command is not None (validation passed)
        assert command is not None, f"Command generation failed for valid IP: {dst_ip}"
        
        # Verify command structure
        assert isinstance(command, list), "Command must be a list of strings"
        assert len(command) == 6, f"Delete command must have 6 elements, got {len(command)}"
        
        # Verify command syntax: ["ip", "route", "del", "<dst>/32", "dev", "wg0"]
        assert command[0] == "ip", "First element must be 'ip'"
        assert command[1] == "route", "Second element must be 'route'"
        assert command[2] == "del", "Third element must be 'del'"
        assert command[3] == f"{dst_ip}/32", f"Fourth element must be '{dst_ip}/32'"
        assert command[4] == "dev", "Fifth element must be 'dev'"
        assert command[5] == "wg0", "Sixth element must be 'wg0'"


@settings(max_examples=100)
@given(
    first_octet=st.integers(min_value=0, max_value=255),
    second_octet=st.integers(min_value=0, max_value=255),
    third_octet=st.integers(min_value=0, max_value=255),
    fourth_octet=st.integers(min_value=0, max_value=255)
)
def test_property_subnet_safety_constraint(first_octet, second_octet, third_octet, fourth_octet):
    """
    Feature: lite-sdwan-routing, Property 20: Subnet Safety Constraint
    
    For any IP address, the subnet safety check should correctly identify whether
    the IP is within the allowed subnet (10.254.0.0/24). Route operations should
    only be permitted for IPs within this subnet.
    
    Validates: Requirements 6.5
    """
    ip_address = f"{first_octet}.{second_octet}.{third_octet}.{fourth_octet}"
    executor = Executor(wg_interface="wg0", allowed_subnet="10.254.0.0/24")
    
    # Determine if IP should be in allowed subnet
    is_in_subnet = (first_octet == 10 and second_octet == 254 and third_octet == 0)
    
    # Test the internal subnet check method
    result = executor._is_ip_in_allowed_subnet(ip_address)
    assert result == is_in_subnet, \
        f"Subnet check failed for {ip_address}: expected {is_in_subnet}, got {result}"
    
    # Test that route commands respect the subnet constraint
    if is_in_subnet:
        # IP is in allowed subnet - commands should be generated
        # Test with a valid next_hop also in subnet
        next_hop = "10.254.0.1"
        
        add_command = executor.generate_route_add_command(ip_address, next_hop)
        assert add_command is not None, \
            f"Route add command should be generated for IP in subnet: {ip_address}"
        
        del_command = executor.generate_route_del_command(ip_address)
        assert del_command is not None, \
            f"Route del command should be generated for IP in subnet: {ip_address}"
    else:
        # IP is outside allowed subnet - commands should be rejected (return None)
        next_hop = "10.254.0.1"
        
        add_command = executor.generate_route_add_command(ip_address, next_hop)
        assert add_command is None, \
            f"Route add command should be rejected for IP outside subnet: {ip_address}"
        
        del_command = executor.generate_route_del_command(ip_address)
        assert del_command is None, \
            f"Route del command should be rejected for IP outside subnet: {ip_address}"


@st.composite
def route_config_pair(draw):
    """
    Generate a pair of routing configurations (desired and current).
    
    Returns a tuple of (desired_routes, current_routes) where each is a
    dictionary mapping destination IP to next hop.
    """
    # Generate number of routes for each configuration
    num_desired = draw(st.integers(min_value=0, max_value=10))
    num_current = draw(st.integers(min_value=0, max_value=10))
    
    # Generate pool of unique IPs to use as destinations
    # Use a larger pool to allow for overlap and differences
    pool_size = max(num_desired + num_current, 1)
    ip_pool = [f"10.254.0.{i}" for i in range(1, min(pool_size + 1, 255))]
    
    # Sample destinations for desired routes
    desired_dsts = draw(st.lists(
        st.sampled_from(ip_pool),
        min_size=num_desired,
        max_size=num_desired,
        unique=True
    ))
    
    # Sample destinations for current routes
    current_dsts = draw(st.lists(
        st.sampled_from(ip_pool),
        min_size=num_current,
        max_size=num_current,
        unique=True
    ))
    
    # Generate next hops for desired routes
    desired_routes = {}
    for dst in desired_dsts:
        # Next hop can be "direct" or another IP in the subnet
        use_direct = draw(st.booleans())
        if use_direct:
            next_hop = "direct"
        else:
            # Generate a different IP as next hop
            next_hop_octet = draw(st.integers(min_value=1, max_value=254))
            next_hop = f"10.254.0.{next_hop_octet}"
        desired_routes[dst] = next_hop
    
    # Generate next hops for current routes
    current_routes = {}
    for dst in current_dsts:
        # Next hop can be "direct" or another IP in the subnet
        use_direct = draw(st.booleans())
        if use_direct:
            next_hop = "direct"
        else:
            # Generate a different IP as next hop
            next_hop_octet = draw(st.integers(min_value=1, max_value=254))
            next_hop = f"10.254.0.{next_hop_octet}"
        current_routes[dst] = next_hop
    
    return (desired_routes, current_routes)


@settings(max_examples=100)
@given(config_pair=route_config_pair())
def test_property_routing_table_diff_calculation(config_pair):
    """
    Feature: lite-sdwan-routing, Property 18: Routing Table Diff Calculation
    
    For any pair of routing configurations (desired and current), the diff
    calculation should correctly identify:
    - Routes to add: present in desired but not in current
    - Routes to modify: present in both but with different next_hop
    - Routes to delete: present in current but not in desired
    
    Validates: Requirements 6.1
    """
    desired_routes, current_routes = config_pair
    executor = Executor(wg_interface="wg0", allowed_subnet="10.254.0.0/24")
    
    # Calculate diff
    to_add, to_modify, to_delete = executor.calculate_route_diff(desired_routes, current_routes)
    
    # Verify routes to add
    # These should be destinations in desired but not in current
    expected_to_add = {}
    for dst, next_hop in desired_routes.items():
        if dst not in current_routes:
            expected_to_add[dst] = next_hop
    
    assert to_add == expected_to_add, \
        f"Routes to add mismatch. Expected: {expected_to_add}, Got: {to_add}"
    
    # Verify routes to modify
    # These should be destinations in both but with different next_hop
    expected_to_modify = {}
    for dst, next_hop in desired_routes.items():
        if dst in current_routes and current_routes[dst] != next_hop:
            expected_to_modify[dst] = next_hop
    
    assert to_modify == expected_to_modify, \
        f"Routes to modify mismatch. Expected: {expected_to_modify}, Got: {to_modify}"
    
    # Verify routes to delete
    # These should be destinations in current but not in desired
    expected_to_delete = []
    for dst in current_routes:
        if dst not in desired_routes:
            expected_to_delete.append(dst)
    
    # Sort both lists for comparison (order doesn't matter)
    assert sorted(to_delete) == sorted(expected_to_delete), \
        f"Routes to delete mismatch. Expected: {sorted(expected_to_delete)}, Got: {sorted(to_delete)}"
    
    # Verify completeness: all routes are accounted for
    # Union of add, modify, and unchanged should equal desired routes
    all_desired_dsts = set(desired_routes.keys())
    accounted_dsts = set(to_add.keys()) | set(to_modify.keys())
    
    # Add unchanged routes (in both with same next_hop)
    unchanged_dsts = set()
    for dst in desired_routes:
        if dst in current_routes and current_routes[dst] == desired_routes[dst]:
            unchanged_dsts.add(dst)
    
    accounted_dsts |= unchanged_dsts
    
    assert accounted_dsts == all_desired_dsts, \
        f"Not all desired routes accounted for. Desired: {all_desired_dsts}, Accounted: {accounted_dsts}"
    
    # Verify no overlap between add, modify, and delete
    add_dsts = set(to_add.keys())
    modify_dsts = set(to_modify.keys())
    delete_dsts = set(to_delete)
    
    assert len(add_dsts & modify_dsts) == 0, \
        "Routes to add and modify should not overlap"
    assert len(add_dsts & delete_dsts) == 0, \
        "Routes to add and delete should not overlap"
    assert len(modify_dsts & delete_dsts) == 0, \
        "Routes to modify and delete should not overlap"
