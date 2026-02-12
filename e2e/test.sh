#!/bin/bash

# Mycelium Spine E2E Test Suite
# This script orchestrates end-to-end tests for the Mycelium Spine UMS

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
COMPOSE_FILE="docker-compose.e2e.yml"
TEST_ORG="test-org"
TEST_SERVER_1="test-server-01"
TEST_SERVER_2="test-server-02"
TEST_CLUSTER="test-cluster-west"
TEST_CLUSTER_NODE="test-cluster-node-01"

# Counter for test results
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((TESTS_PASSED++))
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((TESTS_FAILED++))
}

log_test() {
    echo -e "${YELLOW}[TEST]${NC} $1"
    ((TESTS_RUN++))
}

# Helper to execute mspinectl in a container
exec_mspinectl() {
    local container=$1
    shift
    docker-compose -f "$COMPOSE_FILE" exec -T "$container" mspinectl "$@"
}

# Helper to execute commands in a container
exec_in_container() {
    local container=$1
    shift
    docker-compose -f "$COMPOSE_FILE" exec -T "$container" "$@"
}

# Cleanup function
cleanup() {
    log_info "Cleaning up..."
    docker-compose -f "$COMPOSE_FILE" down -v
}

# Setup trap for cleanup
trap cleanup EXIT

# Start services
start_services() {
    log_info "Starting services..."
    docker-compose -f "$COMPOSE_FILE" up -d --build
    
    log_info "Waiting for services to be healthy..."
    sleep 10
    
    # Check Mycelium Spine health
    local retries=30
    while [ $retries -gt 0 ]; do
        if docker-compose -f "$COMPOSE_FILE" exec -T mycelium-spine nc -z localhost 9090 2>/dev/null; then
            log_success "Mycelium Spine is healthy"
            return 0
        fi
        retries=$((retries - 1))
        sleep 2
    done
    
    log_error "Mycelium Spine failed to start"
    return 1
}

# Test 1: Basic Publish and Subscribe
test_basic_publish_subscribe() {
    log_test "Test 1: Basic Publish and Subscribe"
    
    # Start subscriber in background
    log_info "Starting subscriber on test-client-1..."
    exec_mspinectl test-client-1 subscribe \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --count 1 \
        --auto-ack > /tmp/subscriber_output.txt 2>&1 &
    local sub_pid=$!
    
    sleep 2
    
    # Publish message
    log_info "Publishing message from publisher-client..."
    exec_mspinectl publisher-client publish \
        --type command.test \
        --qos command \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --data '{"test":"message1"}'
    
    # Wait for subscriber
    wait $sub_pid || true
    
    # Check if message was received
    if grep -q "command.test" /tmp/subscriber_output.txt; then
        log_success "Message successfully delivered"
    else
        log_error "Message was not received"
        cat /tmp/subscriber_output.txt
    fi
}

# Test 2: Multiple Subscribers Same Target
test_multiple_subscribers() {
    log_test "Test 2: Multiple Subscribers to Same Target"
    
    # Start two subscribers
    log_info "Starting subscribers on test-client-1 and test-client-2..."
    exec_mspinectl test-client-1 subscribe \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --count 1 \
        --auto-ack > /tmp/sub1_output.txt 2>&1 &
    local sub1_pid=$!
    
    exec_mspinectl test-client-2 subscribe \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --count 1 \
        --auto-ack > /tmp/sub2_output.txt 2>&1 &
    local sub2_pid=$!
    
    sleep 2
    
    # Publish message
    log_info "Publishing message to both subscribers..."
    exec_mspinectl publisher-client publish \
        --type command.broadcast \
        --qos command \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --data '{"test":"multi-subscriber"}'
    
    # Wait for subscribers
    wait $sub1_pid || true
    wait $sub2_pid || true
    
    # Check both received the message
    local success=true
    if grep -q "command.broadcast" /tmp/sub1_output.txt; then
        log_info "Subscriber 1 received message"
    else
        log_error "Subscriber 1 did not receive message"
        success=false
    fi
    
    if grep -q "command.broadcast" /tmp/sub2_output.txt; then
        log_info "Subscriber 2 received message"
    else
        log_error "Subscriber 2 did not receive message"
        success=false
    fi
    
    if [ "$success" = true ]; then
        log_success "Multiple subscribers received message"
    fi
}

# Test 3: QoS Levels
test_qos_levels() {
    log_test "Test 3: Different QoS Levels"
    
    # Start subscriber
    exec_mspinectl test-client-1 subscribe \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --count 3 \
        --auto-ack > /tmp/qos_output.txt 2>&1 &
    local sub_pid=$!
    
    sleep 2
    
    # Publish with different QoS levels
    log_info "Publishing COMMAND message..."
    exec_mspinectl publisher-client publish \
        --type test.command \
        --qos command \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --data '{"qos":"command"}'
    
    sleep 1
    
    log_info "Publishing CONTROL message..."
    exec_mspinectl publisher-client publish \
        --type test.control \
        --qos control \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --data '{"qos":"control"}'
    
    sleep 1
    
    log_info "Publishing TELEMETRY message..."
    exec_mspinectl publisher-client publish \
        --type test.telemetry \
        --qos telemetry \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --data '{"qos":"telemetry"}'
    
    # Wait for subscriber
    wait $sub_pid || true
    
    # Verify all QoS levels received
    local success=true
    if grep -q "COMMAND" /tmp/qos_output.txt; then
        log_info "COMMAND message received"
    else
        log_error "COMMAND message not received"
        success=false
    fi
    
    if grep -q "CONTROL" /tmp/qos_output.txt; then
        log_info "CONTROL message received"
    else
        log_error "CONTROL message not received"
        success=false
    fi
    
    if grep -q "TELEMETRY" /tmp/qos_output.txt; then
        log_info "TELEMETRY message received"
    else
        log_error "TELEMETRY message not received"
        success=false
    fi
    
    if [ "$success" = true ]; then
        log_success "All QoS levels working correctly"
    fi
}

# Test 4: Cluster Target
test_cluster_target() {
    log_test "Test 4: Cluster Target Delivery"
    
    # Start subscriber for cluster
    log_info "Starting cluster node subscriber..."
    exec_mspinectl test-client-3 subscribe \
        --target-type cluster \
        --target-id "$TEST_CLUSTER" \
        --count 1 \
        --auto-ack > /tmp/cluster_output.txt 2>&1 &
    local sub_pid=$!
    
    sleep 2
    
    # Publish to cluster
    log_info "Publishing to cluster target..."
    exec_mspinectl publisher-client publish \
        --type command.cluster \
        --qos command \
        --target-type cluster \
        --target-id "$TEST_CLUSTER" \
        --data '{"target":"cluster"}'
    
    # Wait for subscriber
    wait $sub_pid || true
    
    # Verify delivery
    if grep -q "command.cluster" /tmp/cluster_output.txt; then
        log_success "Cluster target delivery successful"
    else
        log_error "Cluster target delivery failed"
        cat /tmp/cluster_output.txt
    fi
}

# Test 5: Acknowledgment Flow
test_acknowledgment() {
    log_test "Test 5: Message Acknowledgment"
    
    # Start subscriber without auto-ack
    log_info "Starting subscriber without auto-ack..."
    timeout 10 exec_mspinectl test-client-1 subscribe \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --count 1 > /tmp/ack_output.txt 2>&1 &
    local sub_pid=$!
    
    sleep 2
    
    # Publish message
    log_info "Publishing message..."
    exec_mspinectl publisher-client publish \
        --type command.ack \
        --qos command \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --data '{"requires":"ack"}'
    
    sleep 2
    
    # Extract mailbox ID and seq from subscriber output
    wait $sub_pid || true
    
    if grep -q "command.ack" /tmp/ack_output.txt; then
        local mailbox_id=$(grep "Mailbox:" /tmp/ack_output.txt | awk '{print $2}')
        local seq=$(grep "Seq:" /tmp/ack_output.txt | awk '{print $2}')
        
        if [ -n "$mailbox_id" ] && [ -n "$seq" ]; then
            log_info "Sending explicit acknowledgment..."
            exec_mspinectl test-client-1 ack \
                --mailbox-id "$mailbox_id" \
                --seq "$seq"
            log_success "Acknowledgment sent successfully"
        else
            log_error "Could not extract mailbox ID or seq"
        fi
    else
        log_error "Message not received"
    fi
}

# Test 6: Concurrent Publishers
test_concurrent_publishers() {
    log_test "Test 6: Concurrent Publishers"
    
    # Start subscriber
    log_info "Starting subscriber..."
    timeout 20 exec_mspinectl test-client-1 subscribe \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --count 5 \
        --auto-ack > /tmp/concurrent_output.txt 2>&1 &
    local sub_pid=$!
    
    sleep 2
    
    # Publish from multiple clients concurrently
    log_info "Publishing from multiple clients..."
    (
        exec_mspinectl publisher-client publish \
            --type command.concurrent1 \
            --qos command \
            --target-type server \
            --target-id "$TEST_SERVER_1" \
            --data '{"source":"pub1"}'
    ) &
    
    (
        exec_mspinectl test-client-2 publish \
            --type command.concurrent2 \
            --qos command \
            --target-type server \
            --target-id "$TEST_SERVER_1" \
            --data '{"source":"pub2"}'
    ) &
    
    (
        exec_mspinectl test-client-3 publish \
            --type command.concurrent3 \
            --qos command \
            --target-type server \
            --target-id "$TEST_SERVER_1" \
            --data '{"source":"pub3"}'
    ) &
    
    # Wait for all publishers
    wait
    
    sleep 2
    
    # Stop subscriber
    kill $sub_pid 2>/dev/null || true
    wait $sub_pid 2>/dev/null || true
    
    # Check if at least some messages were received
    local received=$(grep -c "command.concurrent" /tmp/concurrent_output.txt || echo "0")
    if [ "$received" -ge 3 ]; then
        log_success "Concurrent publishing successful (received $received messages)"
    else
        log_error "Concurrent publishing failed (only received $received messages)"
        cat /tmp/concurrent_output.txt
    fi
}

# Test 7: Payload Types (JSON validation)
test_payload_types() {
    log_test "Test 7: Different Payload Types"
    
    # Start subscriber
    timeout 15 exec_mspinectl test-client-1 subscribe \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --count 2 \
        --auto-ack > /tmp/payload_output.txt 2>&1 &
    local sub_pid=$!
    
    sleep 2
    
    # Test with different JSON structures
    log_info "Publishing simple JSON..."
    exec_mspinectl publisher-client publish \
        --type test.simple \
        --qos command \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --data '{"key":"value"}'
    
    sleep 1
    
    log_info "Publishing complex JSON..."
    exec_mspinectl publisher-client publish \
        --type test.complex \
        --qos command \
        --target-type server \
        --target-id "$TEST_SERVER_1" \
        --data '{"nested":{"data":[1,2,3]},"array":[{"a":1},{"b":2}]}'
    
    # Wait for subscriber
    wait $sub_pid || true
    
    # Verify both received
    if grep -q "test.simple" /tmp/payload_output.txt && grep -q "test.complex" /tmp/payload_output.txt; then
        log_success "Different payload types handled correctly"
    else
        log_error "Payload type handling failed"
        cat /tmp/payload_output.txt
    fi
}

# Main test execution
main() {
    echo "========================================"
    echo "  Mycelium Spine E2E Test Suite"
    echo "========================================"
    echo ""
    
    # Start services
    if ! start_services; then
        log_error "Failed to start services"
        exit 1
    fi
    
    echo ""
    log_info "Running E2E tests..."
    echo ""
    
    # Run all tests
    test_basic_publish_subscribe
    echo ""
    
    test_multiple_subscribers
    echo ""
    
    test_qos_levels
    echo ""
    
    test_cluster_target
    echo ""
    
    test_acknowledgment
    echo ""
    
    test_concurrent_publishers
    echo ""
    
    test_payload_types
    echo ""
    
    # Print summary
    echo "========================================"
    echo "  Test Summary"
    echo "========================================"
    echo -e "Tests Run:    ${TESTS_RUN}"
    echo -e "Tests Passed: ${GREEN}${TESTS_PASSED}${NC}"
    echo -e "Tests Failed: ${RED}${TESTS_FAILED}${NC}"
    echo "========================================"
    
    if [ $TESTS_FAILED -eq 0 ]; then
        log_success "All tests passed!"
        exit 0
    else
        log_error "$TESTS_FAILED test(s) failed"
        exit 1
    fi
}

# Run main
main "$@"
