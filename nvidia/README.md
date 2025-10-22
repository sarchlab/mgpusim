# NVIDIA GPU Simulator

Simulation framework for NVIDIA GPUs based on trace execution.

## Quick Start

```bash
go build
./nvidia --help
./nvidia --trace-dir data/simple-trace-example
```

## Common Issues

### Slice Bounds Out of Range Error

If you encounter a panic like:
```
panic: runtime error: slice bounds out of range [:512] with capacity 128
```

This indicates that your cache block size is too small for the memory transactions in your trace. 

**Quick Fix**: Increase the cache block size in your platform configuration:
- Change `WithLog2BlockSize(7)` to `WithLog2BlockSize(9)` or higher
- This increases block size from 128 bytes to 512+ bytes

**Detailed Solutions:**
- [Step-by-step fix guide](docs/FIXING_SLICE_BOUNDS_ERROR.md) - Complete walkthrough with examples
- [Cache configuration guide](docs/CACHE_CONFIGURATION.md) - Detailed cache setup and troubleshooting

## Documentation

- [Fixing Slice Bounds Error](docs/FIXING_SLICE_BOUNDS_ERROR.md) - Complete fix for the cache block size issue
- [Cache Configuration Guide](docs/CACHE_CONFIGURATION.md) - Detailed cache setup and troubleshooting

## Directory Structure

- `benchmark/` - Benchmark implementations
- `driver/` - GPU driver simulation
- `gpu/` - GPU device model
- `platform/` - Platform configurations (A100, H100, etc.)
- `sm/` - Streaming Multiprocessor simulation
- `subcore/` - Subcore (processing unit) simulation
- `tracereader/` - Trace file reader and parser
- `data/` - Example trace files
- `docs/` - Documentation
