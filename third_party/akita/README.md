# Vendored Akita code (cache set-index fix)

This directory contains a **verbatim copy** of three Akita cache packages
(`github.com/sarchlab/akita/v5@v5.0.0-beta.5`):

- `mem/cache` — directory / MSHR helpers
- `mem/cache/writeback` — write-back cache (used here as the MI300A L2)
- `mem/cache/writethroughcache` — write-through cache (used here as the MI300A MALL)

The only functional change is in `mem/cache/directory_ops.go`: the cache
set index is computed with an XOR-fold hash (`directorySetID`) instead of
`(addr / blockSize) % numSets`.

## Why

Akita's caches index sets on contiguous low-order address bits. When a cache is
placed behind a bank/channel interleaver (as the MI300A L2 and MALL are — 16
per-channel slices selected by a 128 B interleave), the interleaver's
select bits fall *inside* the set-index bit range. Those bits are constant
within a slice, so only a fraction of the sets are reachable and the slice's
**effective capacity collapses to ~1/16 of nominal** (a 4 MB L2 behaves like
~256 KB). This made the `cache_latency` microbenchmark mis-predict the 1–4 MB
region badly (a 1 MB pointer chase never stayed resident).

The XOR-fold mixes higher-order address bits into the index, so every set is
reachable regardless of which low bits the interleaver consumed. The stored tag
is still the full address, so lookups remain correct (`-verify` passes); only
the set distribution changes.

## Upstream

This is a local fix to be reported back to Akita. Once Akita's
`mem/cache` carries an equivalent set-index hash, delete this directory and
re-point `amd/samples/runner/timingconfig/mi300a/builder.go` at the upstream
`writeback` / `writethroughcache` packages.

Only the MI300A L2 and MALL use this vendored copy; all other caches (L1, the
r9nano config) still use upstream Akita unchanged.
