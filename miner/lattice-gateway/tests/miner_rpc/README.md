# Miner RPC Tests

This directory contains comprehensive tests for the Miner RPC communication system in LatticeGateway.

## Test Structure

### Unit Tests (`test_server.py`)
- **TestMinerRpcServerInit**: Server initialization tests
- **TestMinerRpcServerTransport**: TCP and UDS transport tests
- **TestMinerRpcServerHandlers**: RPC method handler tests
- **TestMinerRpcServerJsonRpc**: JSON-RPC protocol tests
- **TestMinerRpcServerIntegration**: Basic integration tests

### Schema Validation Tests (`test_schemas.py`)
- **TestJsonRpcEnvelopeSchema**: JSON-RPC envelope validation
- **TestGetMiningInfoSchema**: getMiningInfo method validation
- **TestSubmitBlockSchema**: submitBlock method validation
- **TestMethodSchemas**: Method schema mapping tests
- **TestEdgeCases**: Edge case and boundary condition tests

### Integration Tests (`test_integration.py`)
- **TestMinerRpcIntegrationTcp**: End-to-end TCP communication tests
- **TestMinerRpcIntegrationUds**: End-to-end UDS communication tests
- **TestMinerRpcStressTests**: Performance and stress tests
- **TestMinerRpcErrorScenarios**: Error handling integration tests

### Performance Tests (`test_performance.py`)
- **TestMinerRpcPerformanceThroughput**: Sequential and concurrent throughput measurement
- **TestMinerRpcPerformanceLatency**: Baseline and under-load latency measurement
- **TestMinerRpcPerformanceMixed**: Mixed workload performance (getMiningInfo + submitBlock)
- **TestMinerRpcPerformanceLongRunning**: Sustained load stability tests (marked as `slow`)

## Architecture

The `MinerRpcServer` implements a **raw JSON-RPC over sockets** interface for local miner communication:

- **Transport**: TCP sockets or Unix Domain Sockets (UDS)
- **Protocol**: Line-delimited JSON-RPC 2.0 over raw sockets
- **Format**: Each request/response is a single line of JSON followed by `\n`
- **Authentication**: None required (local communication only)

This design provides:
- **High Performance**: Direct socket communication without HTTP overhead
- **Low Latency**: Minimal protocol overhead for local miner communication
- **Simplicity**: Standard JSON-RPC protocol familiar to miners

## Test Utilities

### Mock Client (`tests/miner_rpc/mock_miner_client.py`)
- `MockMinerClient`: Miner client simulation for testing
- Supports both TCP and Unix Domain Socket transports
- Helper functions for creating test matrices and invalid parameters
- **Used by integration tests** for realistic client simulation

### Performance Test Client (`tests/miner_rpc/test_performance.py`)
- `RawJsonRpcClient`: Direct socket JSON-RPC client for performance testing
- Sends pre-prepared line-delimited JSON-RPC requests
- Measures pure server performance without client overhead
- Supports both TCP and UDS transports

### Test Fixtures (`tests/fixtures/miner_fixtures.py`)
- Configuration fixtures for different transport types
- Mock service fixtures
- Test data generators for various scenarios

## Running Tests

### Run All Miner RPC Tests
```bash
pytest tests/miner_rpc/ -v
```

### Run Specific Test Categories
```bash
# Unit tests only
pytest tests/miner_rpc/test_server.py -v

# Schema validation tests
pytest tests/miner_rpc/test_schemas.py -v

# Integration tests only
pytest tests/miner_rpc/test_integration.py -v

# Performance tests only
pytest tests/miner_rpc/test_performance.py -v
```

### Run Tests with Coverage
```bash
pytest tests/miner_rpc/ --cov=lattice_gateway.miner_rpc --cov-report=html
```

### Run Tests by Markers
```bash
# Integration tests only
pytest tests/miner_rpc/ -m integration -v

# Performance tests only
pytest tests/miner_rpc/ -m performance -v

# Exclude slow tests
pytest tests/miner_rpc/ -m "not slow" -v
```

## Test Coverage

The tests cover:

1. **Protocol Compliance**: JSON-RPC 2.0 specification adherence
2. **Transport Layer**: Both TCP and Unix Domain Socket transports
3. **Method Handlers**: getMiningInfo and submitBlock methods
4. **Error Handling**: Various error scenarios and edge cases
5. **Schema Validation**: Input parameter validation
6. **Performance**: Raw socket performance testing (both TCP and UDS)
7. **Integration**: End-to-end communication workflows
8. **Sustained Load**: Long-running stability under continuous load

## Performance Test Metrics

The performance tests measure:

- **Throughput**: Requests per second (RPS) under various conditions
- **Latency**: Response time percentiles (average, P95, P99)
- **Success Rate**: Percentage of successful requests
- **Concurrency**: Performance under concurrent client load
- **Mixed Workloads**: Performance with different request type ratios

### Performance Requirements

- Sequential throughput: ≥1000 RPS
- Concurrent throughput: ≥1000 RPS  
- Mixed workload throughput: ≥800 RPS
- Baseline latency: ≤2ms average, ≤5ms P95
- Under-load latency: ≤10ms average, ≤20ms P95
- Success rate: ≥95% under load, ≥99% sequential

### Recent Performance Results

**TCP Performance:**
- Sequential throughput: **1,408.9 RPS** ✅
- Average response time: **0.71ms** ✅
- P95 response time: **0.81ms** ✅

**UDS Performance:**
- Sequential throughput: **1,574.1 RPS** ✅
- Average response time: **0.63ms** ✅
- P95 response time: **0.70ms** ✅

## Test Configuration

Tests use different ports to avoid conflicts:
- Unit tests: 18443-18445
- Integration tests: 18446-18449
- Performance tests: 18450

UDS tests use temporary socket files that are automatically cleaned up.

## Dependencies

The tests require:
- pytest
- pytest-asyncio


All dependencies are included in the project's requirements. 