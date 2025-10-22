# NVIDIA GPU Cache Configuration Guide

## Issue: Slice Bounds Out of Range Error

### Problem Description
When running NVIDIA traces (e.g., Rodinia backprop), you may encounter the following panic:

```
panic: runtime error: slice bounds out of range [:512] with capacity 128
```

This occurs in the Akita cache library's writearound implementation when a memory transaction size exceeds the cache block size.

### Root Cause
The error occurs when:
- **Cache block size**: 128 bytes (log2BlockSize = 7)
- **Memory access size**: 512 bytes
- **Result**: The cache tries to slice data beyond the available capacity

### Solution
Increase the cache block size to accommodate the largest memory transaction size in your trace.

## Cache Block Size Configuration

### Recommended Settings

For NVIDIA GPU traces with large memory transactions (up to 512 bytes):

```go
cache := writearound.MakeBuilder().
    WithEngine(engine).
    WithFreq(freq).
    WithLog2BlockSize(9).  // 2^9 = 512 bytes
    WithTotalByteSize(128 * mem.KB).
    WithWayAssociativity(8).
    WithNumMSHREntry(32).
    WithNumBanks(4).
    Build("L1Cache")
```

### Block Size Options

| log2BlockSize | Block Size | Use Case |
|---------------|------------|----------|
| 6 | 64 bytes | Small transactions, default Akita setting |
| 7 | 128 bytes | Medium transactions |
| 8 | 256 bytes | Large transactions |
| 9 | 512 bytes | Extra large transactions (recommended for NVIDIA traces) |
| 10 | 1024 bytes | Maximum transaction size |

### Important Notes

1. **Performance Impact**: Larger block sizes increase memory bandwidth usage but reduce the number of cache misses for large transactions.

2. **Alignment**: Ensure memory transactions in your traces are properly aligned to cache block boundaries when possible.

3. **MSHR Entries**: With larger block sizes, you may need to increase the number of MSHR (Miss Status Holding Register) entries to handle more concurrent misses.

## Example Cache Builder Configuration

```go
package platform

import (
    "github.com/sarchlab/akita/v4/mem/cache/writearound"
    "github.com/sarchlab/akita/v4/mem/mem"
    "github.com/sarchlab/akita/v4/sim"
)

func BuildL1Cache(engine sim.Engine, freq sim.Freq, name string) *writearound.Comp {
    return writearound.MakeBuilder().
        WithEngine(engine).
        WithFreq(freq).
        WithLog2BlockSize(9).           // 512 bytes - prevents slice bounds error
        WithTotalByteSize(128 * mem.KB). // 128 KB total cache size
        WithWayAssociativity(8).        // 8-way associative
        WithNumMSHREntry(32).           // 32 MSHR entries
        WithNumBanks(4).                // 4 banks for parallelism
        WithDirectoryLatency(2).        // 2 cycles directory access
        WithBankLatency(20).            // 20 cycles bank access
        WithNumReqsPerCycle(4).         // Process 4 requests per cycle
        WithMaxNumConcurrentTrans(64).  // Up to 64 concurrent transactions
        Build(name)
}
```

## Validation

To validate your cache configuration, ensure:

```go
// Maximum memory transaction size in your trace
maxTransactionSize := 512 // bytes

// Required cache block size
requiredLog2BlockSize := uint64(math.Ceil(math.Log2(float64(maxTransactionSize))))

// Configure cache
cache := writearound.MakeBuilder().
    WithLog2BlockSize(requiredLog2BlockSize). // At least 9 for 512 byte transactions
    // ... other settings
    Build("L1Cache")
```

## Troubleshooting

### Still Getting Slice Bounds Error?

1. **Check trace file**: Verify the maximum memory transaction size in your trace
2. **Increase block size**: Try log2BlockSize = 10 (1024 bytes)
3. **Split transactions**: Modify trace generation to split large transactions into cache-block-sized chunks

### Performance Issues with Large Blocks?

1. **Reduce cache size**: Decrease totalByteSize to maintain the same number of cache lines
2. **Increase associativity**: Higher associativity reduces conflict misses
3. **Add more banks**: Increase parallelism with more banks

## Reference

- Akita Cache Documentation: https://github.com/sarchlab/akita
- NVIDIA GPU Architecture: https://docs.nvidia.com/cuda/cuda-c-programming-guide/
