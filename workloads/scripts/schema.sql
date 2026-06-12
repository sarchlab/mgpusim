-- schema.sql -- SQLite schema for GPU benchmark results
--
-- Usage: sqlite3 results.sqlite3 < schema.sql

-- Run metadata: one row per benchmark execution session
CREATE TABLE IF NOT EXISTS runs (
    run_id          TEXT PRIMARY KEY,  -- timestamp-based ID (e.g., 20260314-123000-hostname)
    run_timestamp   TEXT NOT NULL,     -- ISO 8601 (e.g., 2026-03-14T12:30:00Z)
    hostname        TEXT,

    -- System info
    os_name         TEXT,             -- e.g., "Ubuntu 22.04.3 LTS"
    kernel_version  TEXT,             -- e.g., "6.5.0-44-generic"
    cpu_model       TEXT,             -- e.g., "AMD EPYC 9554 64-Core"
    cpu_cores       INTEGER,
    ram_gb          REAL,

    -- GPU info
    gpu_model       TEXT,             -- e.g., "AMD Instinct MI300A"
    gpu_count       INTEGER,
    gpu_vram_gb     REAL,
    gpu_driver      TEXT,             -- e.g., "6.3.6" (amdgpu) or "550.54.14" (nvidia)

    -- Toolchain info
    compiler        TEXT,             -- "hipcc" or "nvcc"
    compiler_version TEXT,            -- e.g., "17.0.0" or "12.4"
    runtime_version TEXT,             -- ROCm version or CUDA version
    gpu_arch        TEXT,             -- e.g., "gfx942" or "sm_90"

    -- Run config
    platform        TEXT NOT NULL,    -- "hip" or "cuda"
    build_flags     TEXT,             -- e.g., "-O2 --offload-arch=gfx942"
    notes           TEXT              -- free-form annotations
);

-- Benchmark results: one row per (benchmark, problem_size) measurement
CREATE TABLE IF NOT EXISTS results (
    result_id       INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id          TEXT NOT NULL REFERENCES runs(run_id),

    -- Benchmark identity
    suite           TEXT NOT NULL,    -- e.g., "polybench", "shoc", "rodinia"
    benchmark       TEXT NOT NULL,    -- e.g., "atax", "bfs", "nw"
    platform        TEXT NOT NULL,    -- "hip" or "cuda"

    -- Execution parameters
    problem_size    TEXT NOT NULL,    -- e.g., "1024", "1024x1024"
    iterations      INTEGER NOT NULL, -- number of timed iterations

    -- Whole-program timing (wall clock, includes all kernels + transfers)
    total_avg_ms    REAL,
    total_min_ms    REAL,
    total_max_ms    REAL,
    total_stddev_ms REAL,

    -- Status
    status          TEXT NOT NULL DEFAULT 'success',  -- success, fail, timeout, skip
    error_message   TEXT,

    UNIQUE(run_id, suite, benchmark, platform, problem_size)
);

-- Per-kernel timing: one row per individual kernel invocation measurement
CREATE TABLE IF NOT EXISTS kernel_timings (
    timing_id       INTEGER PRIMARY KEY AUTOINCREMENT,
    result_id       INTEGER NOT NULL REFERENCES results(result_id),

    -- Kernel identity
    kernel_name     TEXT NOT NULL,    -- e.g., "atax_kernel1", "atax_kernel2"
    kernel_index    INTEGER,          -- sequential index if same kernel called multiple times

    -- Launch configuration
    grid_x          INTEGER,
    grid_y          INTEGER,
    grid_z          INTEGER,
    block_x         INTEGER,
    block_y         INTEGER,
    block_z         INTEGER,
    shared_mem_bytes INTEGER,

    -- Timing (milliseconds)
    avg_ms          REAL NOT NULL,
    min_ms          REAL NOT NULL,
    max_ms          REAL NOT NULL,
    stddev_ms       REAL,

    -- Optional: register/occupancy info
    registers_per_thread INTEGER,
    occupancy_pct   REAL
);

-- Indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_results_run ON results(run_id);
CREATE INDEX IF NOT EXISTS idx_results_bench ON results(suite, benchmark);
CREATE INDEX IF NOT EXISTS idx_kernel_result ON kernel_timings(result_id);
CREATE INDEX IF NOT EXISTS idx_results_status ON results(status);
