# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build, Test, and Lint Commands

```bash
# Build all packages (~16s clean, ~70s first run with deps)
go build ./...

# Run unit tests (~17s) - uses Ginkgo framework
ginkgo -r --skip-package=nvidia

# Run specific package tests
go test ./amd/emu/... -v

# Lint AMD code
golangci-lint run ./amd/... --timeout=10m

# Run acceptance tests (~3.5 min for single GPU)
cd amd/tests/acceptance && go build && ./acceptance -num-gpu=1

# Run sample simulation
cd amd/samples/fir && go build && ./fir -timing --report-all -length=64 -verify
```

## Architecture Overview

MGPUSim is a cycle-accurate GPU simulator modeling AMD GCN3 instruction set architecture. It uses the Akita discrete-event simulation framework (`github.com/sarchlab/akita/v4`).

### Two Simulation Modes

1. **Emulation Mode** (`amd/emu/`): Fast functional simulation
   - `ComputeUnit` executes wavefronts instruction-by-instruction
   - `ALUImpl` implements all GCN3 instruction semantics (VALU, SALU, LDS, FLAT, etc.)
   - Used for correctness verification

2. **Timing Mode** (`amd/timing/`): Cycle-accurate performance simulation
   - `cu/` - Compute Unit with pipeline stages, scheduling, register files
   - `cp/` - Command Processor for kernel dispatch
   - `rob/` - Reorder Buffer
   - `rdma/` - Remote DMA for multi-GPU communication
   - `pagemigrationcontroller/` - Unified memory page migration

### Key Components

- **`amd/driver/`**: GPU driver simulation - memory allocation, kernel launch, command queues
- **`amd/insts/`**: GCN3 instruction definitions, decoder, disassembler, HSACO parsing
- **`amd/kernels/`**: Kernel loading from HSACO (ELF) files
- **`amd/benchmarks/`**: Benchmark implementations with embedded HSACO binaries
- **`amd/samples/`**: Runnable simulation examples

### HSACO (Kernel Binary) Format

Kernels are compiled to HSACO format (AMD's GPU binary). Two versions:
- **V2/V3**: 256-byte header per kernel in `.text` section, used by GCN3
- **V5**: 64-byte kernel descriptor in `.rodata`, instructions in `.text`

Multi-kernel ELFs: Each kernel is a symbol pointing to its code object. Loading extracts specific kernel data using `symbol.Value` and `symbol.Size`.

### Simulation Flow

1. Benchmark loads HSACO via `kernels.LoadProgramFromMemory(data, "kernelName")`
2. Driver allocates GPU memory, copies kernel args
3. Driver creates dispatch packet with kernel address
4. Command Processor dispatches work-groups to Compute Units
5. CU executes wavefronts (64 threads) through pipeline

## Important Patterns

- Tests use Ginkgo/Gomega with extensive mocking via `go.uber.org/mock`
- Benchmarks embed HSACO binaries using `//go:embed kernels.hsaco`
- Platform configuration in `samples/*/platform.go`, `r9nano.go`, `shaderarray.go`
- NVIDIA code (`nvidia/`) is under development and should be skipped

## Compiling HIP Kernels

For LLVM/ROCm compiler tasks (compiling HIP code to HSACO), use the Docker image:
```bash
docker run -it rocm/dev-ubuntu-24.04:7.1.1
```

## Timing Expectations

Never cancel long-running commands:
- Initial build: ~70s (downloads deps)
- Unit tests: ~17s
- Acceptance tests (1 GPU): ~3.5 min
- Multi-GPU tests: 30+ min each

## Before Completing Tasks

**IMPORTANT**: Before finishing any code modification task, always run linting to ensure code quality:

```bash
# Install golangci-lint v2.1.5 (if not already installed)
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.5

# Run linting
golangci-lint run ./amd/... --timeout=10m
```

Fix any linting issues before considering the task complete. Common issues include:
- Unnecessary type conversions (`unconvert`)
- Function complexity (`gocognit`, `funlen`) - add `//nolint:gocognit,funlen` if justified
- Line length (`lll`) - keep lines under 120 characters
