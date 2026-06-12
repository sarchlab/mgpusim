# CI Runner Configuration

## Hardware

| Spec | Value |
|------|-------|
| Machines | 7 × Mac mini |
| Memory | 6 × 24 GB, 1 × 16 GB |
| Runners per machine | 3 self-hosted GitHub Actions runners |
| Total runners | 21 |

## Memory Controls in `benchmark.yml`

Each benchmark process is governed by four layers of memory control:

### 1. `GOMEMLIMIT=4GiB`
The **primary** memory control. Tells the Go runtime to keep live heap
under 4 GiB. This is the knob that actually limits resident memory
(RSS typically stays at 4–6 GiB including stacks and OS overhead).

### 2. `GOGC=50`
Triggers garbage collection when the heap grows 50 % above the
previous live-heap size (default is 100 %). This makes GC run more
frequently, trading CPU time for lower peak memory.

### 3. `ulimit -v 16777216` (16 GiB virtual address space)
A safety net only. Go's runtime may map 10–20 GiB of virtual address
space even when RSS is well under 6 GiB. The 16 GiB cap catches truly
runaway allocations without interfering with normal Go virtual-memory
behaviour.

**Why 16 GiB?** With 3 runners on a 24 GiB Mac mini, each runner can
use up to 8 GiB. 16 GiB of *virtual* space is generous (virtual ≠
resident) while still catching leaks. It also stays within range for
the single 16 GiB machine.

### 4. `max-parallel: 14`
Limits the number of concurrent matrix jobs across all runners.
Prevents every runner from executing a memory-intensive benchmark at
the same time.

## Per-Machine Capacity

| Machine | RAM | Runners | RAM per runner |
|---------|-----|---------|----------------|
| 24 GB Mac mini (×6) | 24 GiB | 3 | ~8 GiB |
| 16 GB Mac mini (×1) | 16 GiB | 3 | ~5.3 GiB |

> **Recommendation:** Consider reducing the 16 GB Mac mini to
> **2 runners** instead of 3. Three runners at ~5 GiB RSS each total
> 15 GiB, leaving little room for macOS and background processes.

## Summary Formula

```
Each benchmark process ≈ 4–6 GiB RSS
  = 4 GiB Go heap (GOMEMLIMIT)
  + goroutine stacks
  + OS/runtime overhead
```

Keep `GOMEMLIMIT × runners_per_machine` well below the machine's
physical RAM to avoid OOM kills.
