# Deployment Tuning Guide

## Overview

Pipeline uses sensible defaults that work for most deployments, but can be tuned via environment variables for specific hardware/workload requirements.

**Default profile**: 1GB container, moderate load (1K-10K events/sec), <10ms latency

---

## Configuration Methods

### 1. Use Defaults (Recommended for Most Deployments)

```go
// Just use defaults - works out of the box
eng := engine.New()
```

### 2. Environment Variables (Recommended for Containers)

```bash
# Set environment variables before starting
export PIPELINE_MEMORY_LIMIT_BYTES=2147483648  # 2GB
export PIPELINE_MEM_ENTER_PCT=0.75
export PIPELINE_QUEUE_MAX=2048
export PIPELINE_MAX_WORKERS=16

./my-app
```

### 3. Programmatic Configuration

```go
cfg := engine.DefaultConfig()

// Override specific values
cfg.MemoryLimitBytes = 2 * 1024 * 1024 * 1024  // 2GB
cfg.MaxWorkers = 16
cfg.QueueSizeMax = 2048

eng := engine.NewWithConfig(cfg)
```

---

## Deployment Profiles

### Small Container (512MB, Light Load)

**Use case**: Development, testing, low-traffic services

```bash
# Memory constraints - be conservative
export PIPELINE_MEMORY_LIMIT_BYTES=536870912    # 512MB
export PIPELINE_MEM_ENTER_PCT=0.65              # Enter degraded earlier
export PIPELINE_MEM_EXIT_PCT=0.50
export PIPELINE_BUFFER_MEMORY_PCT=0.40          # Less buffer memory

# Smaller queues
export PIPELINE_QUEUE_START=64
export PIPELINE_QUEUE_MAX=512

# Fewer workers
export PIPELINE_MIN_WORKERS=1
export PIPELINE_MAX_WORKERS=4

# More aggressive control
export PIPELINE_CONTROL_INTERVAL=2s
```

**Expected behavior**:
- Enters degraded mode at 332 MB (65%)
- Exits at 256 MB (50%)
- Max 512 events buffered per queue
- 1-4 workers

---

### Standard Container (1GB, Moderate Load)

**Use case**: Production services, typical workloads

```bash
# Use defaults - they're tuned for this!
# Or explicitly:

export PIPELINE_MEMORY_LIMIT_BYTES=1073741824   # 1GB
export PIPELINE_MEM_ENTER_PCT=0.70
export PIPELINE_MEM_EXIT_PCT=0.55

export PIPELINE_QUEUE_START=128
export PIPELINE_QUEUE_MIN=8
export PIPELINE_QUEUE_MAX=1024

export PIPELINE_MIN_WORKERS=2
export PIPELINE_MAX_WORKERS=8
```

**Expected behavior**:
- Enters degraded at 717 MB (70%)
- Exits at 574 MB (55%)
- Buffers adapt 8-1024 based on saturation
- 2-8 workers based on lag

---

### Large Container (4GB+, High Load)

**Use case**: High-throughput services, large payload processing

```bash
# More headroom before degrading
export PIPELINE_MEMORY_LIMIT_BYTES=4294967296   # 4GB
export PIPELINE_MEM_ENTER_PCT=0.80              # Can go higher before action
export PIPELINE_MEM_EXIT_PCT=0.65

# Larger queues
export PIPELINE_QUEUE_START=256
export PIPELINE_QUEUE_MAX=4096

# More workers
export PIPELINE_MIN_WORKERS=4
export PIPELINE_MAX_WORKERS=32

# More buffer memory
export PIPELINE_BUFFER_MEMORY_PCT=0.60

# Less frequent control adjustments (stable)
export PIPELINE_CONTROL_INTERVAL=5s
export PIPELINE_CONTROL_COOLDOWN=60s
```

**Expected behavior**:
- Enters degraded at 3.2 GB (80%)
- Exits at 2.6 GB (65%)
- Max 4096 events buffered per queue
- 4-32 workers
- Stable under high load

---

### Latency-Sensitive (Real-time Requirements)

**Use case**: Low-latency services, gaming, trading, real-time analytics

```bash
# Keep queues small to minimize latency
export PIPELINE_QUEUE_START=32
export PIPELINE_QUEUE_MIN=8
export PIPELINE_QUEUE_MAX=256

# Lower target lag
export PIPELINE_TARGET_LAG_MS=5

# More aggressive RED to prevent queue buildup
export PIPELINE_RED_MIN_FILL=0.5                # Start dropping at 50%
export PIPELINE_RED_MAX_PROB=0.4                # More aggressive drops

# Faster control loop
export PIPELINE_CONTROL_INTERVAL=1s
export PIPELINE_GOVERNOR_POLL_MS=25ms
```

**Expected behavior**:
- Max 256 buffered events (low latency)
- Scales workers to keep lag <5ms
- Starts dropping at 50% full (prevents buildup)

---

### Throughput-Optimized (Batch Processing)

**Use case**: Log processing, ETL, analytics pipelines

```bash
# Large queues to smooth bursts
export PIPELINE_QUEUE_START=512
export PIPELINE_QUEUE_MAX=8192

# Higher target lag (throughput > latency)
export PIPELINE_TARGET_LAG_MS=50

# Less aggressive RED
export PIPELINE_RED_MIN_FILL=0.8                # Only drop when very full
export PIPELINE_RED_MAX_PROB=0.2

# More workers for parallelism
export PIPELINE_MAX_WORKERS=64

# Slower control loop (stable throughput)
export PIPELINE_CONTROL_INTERVAL=10s
```

**Expected behavior**:
- Up to 8192 buffered events per queue
- Can tolerate 50ms lag
- Up to 64 workers for high parallelism
- Optimizes for throughput over latency

---

### Embedded/Edge (Raspberry Pi, IoT)

**Use case**: Edge computing, IoT devices, embedded systems

```bash
# Very constrained memory
export PIPELINE_MEMORY_LIMIT_BYTES=134217728    # 128MB
export PIPELINE_MEM_ENTER_PCT=0.60
export PIPELINE_MEM_EXIT_PCT=0.45
export PIPELINE_BUFFER_MEMORY_PCT=0.30          # Minimal buffer memory

# Tiny queues
export PIPELINE_QUEUE_START=16
export PIPELINE_QUEUE_MIN=4
export PIPELINE_QUEUE_MAX=128

# Single worker most of the time
export PIPELINE_MIN_WORKERS=1
export PIPELINE_MAX_WORKERS=2

# Disable expensive features
export PIPELINE_FLIGHT_RECORDER_SIZE=20         # Smaller recorder
export PIPELINE_PSI_ENABLED=false               # May not be available
```

**Expected behavior**:
- Degrades at 77 MB (60%)
- Very small buffers (4-128)
- 1-2 workers max
- Minimal overhead

---

## Environment Variable Reference

### Memory Thresholds

| Variable | Default | Range | Description |
|----------|---------|-------|-------------|
| `PIPELINE_MEMORY_LIMIT_BYTES` | auto-detect | >0 | Container memory limit (0=auto) |
| `PIPELINE_MEM_ENTER_PCT` | 0.70 | 0.0-1.0 | Enter degraded mode threshold |
| `PIPELINE_MEM_EXIT_PCT` | 0.55 | 0.0-1.0 | Exit degraded mode threshold |
| `PIPELINE_MEM_CRITICAL_PCT` | 0.90 | 0.0-1.0 | Critical memory threshold |
| `PIPELINE_BUFFER_MEMORY_PCT` | 0.50 | 0.0-1.0 | % of limit for buffers |

### Queue Configuration

| Variable | Default | Range | Description |
|----------|---------|-------|-------------|
| `PIPELINE_QUEUE_START` | 128 | >0 | Initial queue size |
| `PIPELINE_QUEUE_MIN` | 8 | >0 | Minimum queue size |
| `PIPELINE_QUEUE_MAX` | 1024 | >0 | Maximum queue size |
| `PIPELINE_RED_MIN_FILL` | 0.6 | 0.0-1.0 | RED start threshold |
| `PIPELINE_RED_MAX_PROB` | 0.3 | 0.0-1.0 | RED max drop probability |

### Worker Scaling

| Variable | Default | Range | Description |
|----------|---------|-------|-------------|
| `PIPELINE_MIN_WORKERS` | 2 | >0 | Minimum worker count |
| `PIPELINE_MAX_WORKERS` | 8 | >0 | Maximum worker count |
| `PIPELINE_TARGET_LAG_MS` | 10 | >0 | Target queue lag (ms) |

### Control Loop

| Variable | Default | Range | Description |
|----------|---------|-------|-------------|
| `PIPELINE_CONTROL_INTERVAL` | 3s | >0 | Control loop interval |
| `PIPELINE_CONTROL_COOLDOWN` | 30s | >0 | Min time between actions |
| `PIPELINE_MAX_ACTIONS` | 1 | >0 | Max actions per loop |
| `PIPELINE_GOVERNOR_POLL_MS` | 50ms | >0 | Governor polling interval |

### AIMD Governor

| Variable | Default | Range | Description |
|----------|---------|-------|-------------|
| `PIPELINE_AIMD_INCR` | 0.05 | >0 | Additive increase step |
| `PIPELINE_AIMD_DECR` | 0.5 | 0.0-1.0 | Multiplicative decrease |
| `PIPELINE_AIMD_MAX_TICK` | 0.1 | >0 | Max change per tick |

### PSI (Linux Only)

| Variable | Default | Range | Description |
|----------|---------|-------|-------------|
| `PIPELINE_PSI_ENABLED` | true | bool | Enable PSI monitoring |
| `PIPELINE_PSI_THRESHOLD` | 0.2 | 0.0-1.0 | avg10 threshold |
| `PIPELINE_PSI_SUSTAIN` | 2s | >0 | Sustain duration |
| `PIPELINE_PSI_POLL_INTERVAL` | 1s | >0 | Polling interval |

### Diagnostics

| Variable | Default | Range | Description |
|----------|---------|-------|-------------|
| `PIPELINE_FLIGHT_RECORDER_SIZE` | 100 | >0 | Snapshot count |
| `PIPELINE_FLIGHT_RECORDER_INTERVAL` | 1s | >0 | Snapshot interval |
| `PIPELINE_ERROR_BUS_BUFFER` | 32 | >0 | Error event buffer size |
| `PIPELINE_ERROR_SAMPLING` | false | bool | Sample high-freq errors |

---

## Tuning Strategy

### 1. Start with Defaults

Deploy with defaults, observe behavior:

```bash
# No env vars - use defaults
./my-app
```

Monitor:
- Memory usage patterns
- Queue saturation
- Latency (p50, p99)
- Drop rate

### 2. Adjust Based on Observations

#### If you see frequent degraded mode entries:

```bash
# Increase headroom
export PIPELINE_MEM_ENTER_PCT=0.80
```

#### If queues are always saturated:

```bash
# Increase max queue size
export PIPELINE_QUEUE_MAX=2048

# Or add more workers
export PIPELINE_MAX_WORKERS=16
```

#### If memory usage is stable and low:

```bash
# Reduce memory reservation
export PIPELINE_BUFFER_MEMORY_PCT=0.40
```

#### If latency is too high:

```bash
# Lower target lag
export PIPELINE_TARGET_LAG_MS=5

# Smaller queues
export PIPELINE_QUEUE_MAX=512
```

### 3. Load Test and Iterate

```bash
# Run your stress tests
./stress-test --payload-size=10MB --rate=1000/sec

# Observe:
# - Does it degrade gracefully?
# - Does it recover?
# - Are drops acceptable?

# Adjust accordingly
```

---

## Docker Compose Example

```yaml
version: '3.8'

services:
  relay-node:
    image: my-relay-node:latest
    environment:
      # Memory limits
      - PIPELINE_MEMORY_LIMIT_BYTES=2147483648  # 2GB
      - PIPELINE_MEM_ENTER_PCT=0.75
      - PIPELINE_MEM_EXIT_PCT=0.60

      # Queue sizing for high throughput
      - PIPELINE_QUEUE_START=256
      - PIPELINE_QUEUE_MAX=4096

      # Workers for 4-core container
      - PIPELINE_MIN_WORKERS=2
      - PIPELINE_MAX_WORKERS=16

    # Match cgroup limits
    deploy:
      resources:
        limits:
          memory: 2G
          cpus: '4'
```

---

## Kubernetes Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pipeline-service
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: app
        image: my-app:latest
        env:
        # Pipeline will auto-detect these from cgroups
        - name: PIPELINE_MEMORY_LIMIT_BYTES
          value: "1073741824"  # 1GB

        # Tune for Kubernetes environment
        - name: PIPELINE_MEM_ENTER_PCT
          value: "0.70"
        - name: PIPELINE_QUEUE_MAX
          value: "1024"
        - name: PIPELINE_MAX_WORKERS
          value: "8"

        resources:
          limits:
            memory: "1Gi"
            cpu: "2"
          requests:
            memory: "1Gi"
            cpu: "1"
```

---

## Monitoring Your Configuration

### Log Startup Config

Pipeline logs its active configuration at startup:

```
INFO: Pipeline Configuration:
  Memory Thresholds:
    Enter Degraded: 70%
    Exit Degraded:  55%
    Critical:       90%
  Queues:
    Start Size: 128
    Min Size:   8
    Max Size:   1024
    RED Start:  60%
  Workers:
    Min: 2
    Max: 8
    Target Lag: 10ms
  Memory Limit: 1.0 GB (detected from cgroup-v2)
```

### Verify with Prometheus

Monitor these metrics to validate configuration:

```promql
# Memory usage vs thresholds
pipeline_memory_usage_bytes / pipeline_memory_limit_bytes

# Queue sizes
pipeline_subscription_buffer_size

# Worker count
pipeline_workers_active

# Control loop actions
rate(pipeline_control_actions_total[5m])
```

---

## Troubleshooting

### Problem: Constant degraded mode

**Symptom**: Pipeline enters degraded mode and never exits

**Solutions**:
1. Increase `PIPELINE_MEM_ENTER_PCT` (more headroom)
2. Decrease `PIPELINE_BUFFER_MEMORY_PCT` (less buffer memory)
3. Add more memory to container
4. Reduce `PIPELINE_QUEUE_MAX` (less buffering)

### Problem: High drop rate

**Symptom**: `pipeline_events_dropped_total` increasing rapidly

**Solutions**:
1. Increase `PIPELINE_QUEUE_MAX` (more capacity)
2. Increase `PIPELINE_MAX_WORKERS` (more parallelism)
3. Increase `PIPELINE_RED_MIN_FILL` (delay drops)
4. Accept drops as expected (legitimate overload)

### Problem: High latency

**Symptom**: p99 latency >100ms

**Solutions**:
1. Decrease `PIPELINE_TARGET_LAG_MS` (faster scaling)
2. Decrease `PIPELINE_QUEUE_MAX` (less queueing)
3. Increase `PIPELINE_MIN_WORKERS` (always ready)
4. Set more aggressive RED (`PIPELINE_RED_MIN_FILL=0.5`)

### Problem: Frequent OOM despite monitoring

**Symptom**: Still getting OOM kills even with error signaling

**Possible causes**:
1. Memory limit not detected (check logs) - set `PIPELINE_MEMORY_LIMIT_BYTES` explicitly
2. External memory pressure (non-pipeline allocations) - reduce buffer budget
3. PSI disabled (non-Linux) - enable if available
4. Thresholds too high - lower `PIPELINE_MEM_ENTER_PCT`

---

## Best Practices

1. **Always set memory limits explicitly in production**
   ```bash
   export PIPELINE_MEMORY_LIMIT_BYTES=<your_container_limit>
   ```

2. **Match container limits and PIPELINE config**
   - If Docker has 2GB limit, set `PIPELINE_MEMORY_LIMIT_BYTES=2147483648`

3. **Start conservative, then relax**
   - Begin with lower thresholds (e.g., `MEM_ENTER_PCT=0.65`)
   - Increase as you gain confidence

4. **Monitor for at least 24 hours before tuning**
   - Let system stabilize
   - Observe natural load patterns

5. **Test failure modes**
   - Inject load spikes
   - Verify graceful degradation
   - Ensure recovery after relief

6. **Document your tuning**
   - Note why you changed defaults
   - Track performance before/after
   - Version control env var files
