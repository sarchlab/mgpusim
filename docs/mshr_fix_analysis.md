# MSHR Bank Latency Fix & L1→L2 Latency Analysis

## Summary

Commit `b64faf3b` introduced a structural fix to the L2 MSHR (Miss Status
Holding Register) bank pipeline. Commit `7413e285` simultaneously added a
40-cycle-per-hop latency to the L1→L2 interconnect. While the MSHR fix is
correct and beneficial, the L1→L2 latency caused severe accuracy regressions
in capacity-miss-dominated benchmarks. This document explains both changes and
why the latency was reverted.

## What the MSHR Fix Does

The L2 cache previously processed coalesced reads (multiple L1 misses mapping
to the same cache line) as instant merges with zero additional pipeline cost.
In real hardware, every MSHR lookup still traverses the bank pipeline—each
coalesced read incurs a bank-access latency even when it hits an existing MSHR
entry.

The fix routes coalesced reads through the same bank-latency pipeline as fresh
misses, adding `l2BankLatency` cycles (default 14) per coalesced access. This
more accurately models contention in the L2 bank array.

## Why L1→L2 Latency Breaks atax / bicg

With `latencyconn` at 40 cycles/hop (80 cycles round-trip), benchmarks with
high L1 miss rates see dramatically inflated execution times:

| Benchmark | WMAPE (before) | WMAPE (with lat=40) | Δ    |
|-----------|---------------|---------------------|------|
| atax      | ~12%          | ~115%               | +103pp |
| bicg      | ~8%           | ~150%               | +142pp |

These benchmarks perform streaming accesses that overwhelm the L1 and
generate many capacity misses. The 80-cycle round-trip penalty is applied to
**every** miss, compounding across thousands of cache lines. Real MI300A
hardware uses a high-bandwidth on-die interconnect (Infinity Fabric) where the
L1→L2 hop is effectively hidden by pipelining and deep buffering—something
our simple latency model cannot capture.

## Why Hotspot Needs a Longer Timeout

The hotspot benchmark at large grid sizes (≥2048) performs many small-kernel
iterations, each with moderate L1/L2 traffic. Even without the L1→L2 latency,
the MSHR bank-latency fix increases per-iteration time, pushing the largest
sizes past the previous 600s CI timeout. Increasing to 1800s gives adequate
headroom.

## Decision

- **Keep**: MSHR bank latency fix (structurally correct, improves most benchmarks)
- **Revert**: L1→L2 latency to 0 (directconnection) — too coarse a model for
  the on-die interconnect; needs bandwidth-limited modeling before re-enabling
- **Increase**: hotspot CI timeout from 600s → 1800s

## Future Work

To properly model L1→L2 interconnect delay, consider:
1. Bandwidth-limited connection (bytes/cycle cap) instead of fixed latency
2. Per-bank L2 arbitration queues with back-pressure
3. Separate read/write latency paths
