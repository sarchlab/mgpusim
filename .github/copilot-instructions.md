# MGPUSim - GPU Simulator

MGPUSim is a high-flexibility, high-performance, high-accuracy GPU simulator that models GPUs running AMD GCN3 instruction sets with multi-GPU simulation support.

Always reference these instructions first and fallback to search or bash commands only when you encounter unexpected information that does not match the info here.

## Working Effectively

### Bootstrap, Build, and Test the Repository
- Install Go 1.24+ from golang.org
- `go build ./...` -- builds all packages. Takes ~70 seconds on first run (downloads dependencies), ~16 seconds on subsequent clean builds. NEVER CANCEL. Set timeout to 120+ seconds.
- Install Ginkgo test framework: `go install github.com/onsi/ginkgo/v2/ginkgo`
- Add `$(go env GOPATH)/bin` to PATH: `export PATH=$PATH:$(go env GOPATH)/bin`
- Install golangci-lint: `curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin`
- Run unit tests: `ginkgo -r --skip-package=nvidia` -- takes ~17 seconds. NEVER CANCEL. Set timeout to 60+ seconds.
- Run linting: `golangci-lint run ./amd/... --timeout=10m` -- takes ~3 seconds. Some style issues are expected.

### Run Sample Applications
- Build samples: `cd amd/samples/fir && go build` -- takes ~1 second
- Run simulation: `./fir -timing --report-all -length=64` -- takes <1 second for small datasets
- Check output: generates `akita_sim_*.sqlite3` files with metrics (not CSV)
- Available samples in `amd/samples/`: fir, aes, atax, bicg, bfs, fft, kmeans, matrixmultiplication, and many more

### Run Comprehensive Tests
- Unit tests: `ginkgo -r --skip-package=nvidia` -- 17 seconds, covers all AMD components
- Acceptance tests: `cd amd/tests/acceptance && go build && ./acceptance -num-gpu=1` -- takes ~3.5 minutes. NEVER CANCEL. Set timeout to 10+ minutes.
- Deterministic tests: `cd amd/tests/deterministic && python3 test.py` -- tests simulation reproducibility
- Multi-GPU tests: Use `-num-gpu=2` or `-num-gpu=4` flags (these take longer, 30+ minutes each)

## Validation

### Always Test Your Changes
- ALWAYS run `go build ./...` after making changes to ensure compilation
- ALWAYS run unit tests: `ginkgo -r --skip-package=nvidia` 
- ALWAYS run linting: `golangci-lint run ./amd/... --timeout=10m`
- Run specific benchmark to validate functionality: `cd amd/samples/fir && go build && ./fir -timing --report-all -length=64 -verify`

### Manual Validation Scenarios
- **Basic simulation**: Build and run FIR sample with verification: `./fir -timing --report-all -length=64 -verify` -- should show "Passed!" message
- **Timing simulation**: Run with larger datasets: `./fir -timing --report-all -length=8192 -verify`
- **Multi-benchmark validation**: Run acceptance tests on specific benchmarks: `./acceptance -benchmark="fir|aes" -num-gpu=1`
- **Output verification**: Check that SQLite files are generated: `ls -la *.sqlite3` -- should show `akita_sim_*.sqlite3` files

### Simulation Output Validation
- Check SQLite output: `ls -la *.sqlite3` -- should generate files named `akita_sim_*.sqlite3`
- Simulation should complete without errors and show "Monitoring simulation with http://localhost:XXXXX"
- With `-verify` flag, should print verification results

## Common Tasks

### Building Individual Components
```bash
# Build specific benchmark
cd amd/samples/fir && go build

# Build test utilities
cd amd/tests/acceptance && go build
cd amd/tests/deterministic/empty_kernel && go build
```

### Running Different Simulation Modes
```bash
# Emulation mode (fast, no timing)
./fir -verify --report-all -length=8192

# Timing mode (detailed, slower)
./fir -timing -verify --report-all -length=8192

# Multi-GPU simulation
./fir -timing --report-all -gpus=1,2 -length=8192

# Unified memory mode
./fir -timing --report-all -use-unified-memory -length=8192
```

### CI/CD Workflow Validation
Before committing changes, run the same checks as CI:
1. `go build ./...` -- ensures compilation
2. `golangci-lint run ./amd/... --timeout=10m` -- style checks (some issues expected)
3. `ginkgo -r --skip-package=nvidia` -- unit tests (~17 seconds)
4. `cd amd/tests/acceptance && ./acceptance -num-gpu=1` -- acceptance tests (~3.5 minutes)
5. `cd amd/tests/deterministic && python3 test.py` -- deterministic tests

## Repository Structure

### Key Directories
- `amd/`: AMD GPU simulation (stable, primary focus)
- `nvidia/`: NVIDIA GPU simulation (under development, not ready for use)
- `amd/samples/`: Sample benchmarks and applications
- `amd/benchmarks/`: Benchmark implementations (AMD APP SDK, HeteroMark, Polybench, Rodinia, SHOC, DNN)
- `amd/tests/`: Test suites (acceptance, deterministic)
- `amd/driver/`: GPU driver simulation
- `amd/timing/`: Timing simulation components
- `amd/emu/`: GPU emulation components

### Key Files
- `go.mod`: Go module dependencies (depends on Akita simulation framework)
- `.golangci.yml`: Linting configuration (many checks disabled)
- `.github/workflows/mgpusim_test.yml`: CI/CD pipeline definition
- `amd/run_before_merge.sh`: Script that mimics CI checks

### Sample Applications Available
From `amd/samples/`: atax, aes, bfs, bicg, bitonicsort, fft, fir, floydwarshall, kmeans, matrixmultiplication, matrixtranspose, nbody, pagerank, relu, simpleconvolution, spmv, stencil2d

## Development Patterns

### Working with Akita Framework
- MGPUSim depends on `github.com/sarchlab/akita/v4` simulation framework
- To use local Akita: add `replace github.com/sarchlab/akita/v4 => ../akita` to `go.mod`
- Tests use Ginkgo/Gomega testing framework with extensive mocking

### Configuration and Platform Files
When creating experiments:
- Copy `amd/samples/experiment/` template 
- Modify `main.go` for benchmark selection
- Adjust `runner.go`, `platform.go`, `r9nano.go`, `shaderarray.go` for configuration

### Timing Expectations (NEVER CANCEL)
- Initial build with deps: ~70 seconds (subsequent: ~16 seconds clean rebuild)
- Unit test suite: ~17 seconds  
- Linting: ~3 seconds
- Single-GPU acceptance tests: ~3.5 minutes
- Multi-GPU acceptance tests: 30+ minutes each
- Deterministic tests: ~1 minute for basic tests
- Large simulations: can take hours (document expected time)
- Individual sample simulations: <1 second for small datasets

## Troubleshooting

### Build Issues
- Ensure Go 1.24+ is installed: `go version`
- Clean module cache: `go clean -modcache`
- Update dependencies: `go mod tidy`

### Test Failures
- AMD tests should pass; NVIDIA tests are under development
- Deterministic test failures indicate simulation non-determinism
- Acceptance test failures may indicate core functionality issues

### Performance Issues
- Large datasets significantly increase simulation time
- Use smaller problem sizes for development: `-length=64` instead of `-length=65536`
- Timing mode (`-timing`) is much slower than emulation mode

This is a specialized simulation codebase. Focus on AMD components, avoid NVIDIA code, and always validate with multiple test scenarios.