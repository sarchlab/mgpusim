#!/usr/bin/env python3
"""MI300A terminal-discovery workflow helper.

This helper keeps the branch workflow YAML small while preserving the
per-benchmark terminal-discovery shard contract:

* six checked-in shard matrix files;
* 81 visible workflow benchmark rows;
* 1416 nested per-size attempts;
* one benchmark-level artifact containing one outcome record per attempt.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import shlex
import shutil
import signal
import sqlite3
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from typing import Any, Mapping, Optional, Sequence

REPO_ROOT = Path(__file__).resolve().parents[1]
GENERATED_DIR = REPO_ROOT / "benchmark-comparison" / "generated"
SHARD_MANIFEST = GENERATED_DIR / "mi300a_terminal_discovery_shard_manifest.json"
SHARD_TEMPLATE = "mi300a_terminal_discovery_shard_{shard_index:02d}_matrix.json"
EXPECTED_BRANCH = "e26-m18-launch-per-benchmark-terminal-discovery-replacem"
EXPECTED_ATTEMPTS = 1416
EXPECTED_ROWS = 81
EXPECTED_SHARDS = 6
EXPECTED_TIMEOUT_SEC = 3600
ARTIFACT_PREFIX = "m18-terminal-discovery-outcomes"
OUTCOME_SCHEMA = "mgpusim.mi300a_terminal_discovery_outcome"
BENCHMARK_OUTCOME_SCHEMA = "mgpusim.mi300a_terminal_discovery_benchmark_outcomes"

BUS_SPEED_BENCHMARKS = frozenset({"bus_speed_download", "bus_speed_readback"})

CSV_KERNEL_NAMES = {
    "fft": "fft1D_512",
    "spmv": "spmv_csr_scalar",
    "2dconvolution": "2DConvolution",
    "3dconvolution": "3DConvolution",
    "backprop": "layerforward",
    "streamcluster": "streamcluster_pgain",
    "parboil_cutcp": "cutoff_potential",
    "parboil_histo": "histogram",
    "parboil_lbm": "lbm_stream_collide",
    "parboil_mri_gridding": "gridding_kernel",
    "parboil_mri_q": "ComputeQ",
    "parboil_sad": "ComputeSAD",
    "parboil_sgemm": "sgemm_tiled",
    "parboil_spmv": "spmv_csr",
    "parboil_stencil": "stencil_kernel",
    "parboil_tpacf": "tpacf_DD",
    "bus_speed_download": "BusSpeedDownload",
    "bus_speed_readback": "BusSpeedReadback",
    "device_memory_read": "DeviceMemory_Read",
    "device_memory_write": "DeviceMemory_Write",
    "max_flops": "MaxFlops",
    "md_lj": "md",
    "md5hash": "md5hash",
    "qtc": "qtc",
    "sssp": "sssp_relax_kernel",
    "bh": "compute_forces_kernel",
    "dmr": "identify_bad_triangles",
}


class OperatorError(Exception):
    """Raised when the branch-only operator contract cannot be satisfied."""


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def repo_relative(path: Path) -> str:
    try:
        return path.resolve().relative_to(REPO_ROOT).as_posix()
    except ValueError:
        return path.as_posix()


def load_json_object(path: Path) -> Mapping[str, Any]:
    try:
        with path.open("r", encoding="utf-8") as f:
            data = json.load(f)
    except FileNotFoundError as exc:
        raise OperatorError(f"{repo_relative(path)}: file not found") from exc
    except json.JSONDecodeError as exc:
        raise OperatorError(f"{repo_relative(path)}: invalid JSON: {exc}") from exc
    if not isinstance(data, Mapping):
        raise OperatorError(f"{repo_relative(path)}: JSON root must be an object")
    return data


def write_json(path: Path, data: Mapping[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def append_github_output(key: str, value: str) -> None:
    output = os.environ.get("GITHUB_OUTPUT")
    if output:
        with open(output, "a", encoding="utf-8") as f:
            if "\n" in value:
                marker = f"EOF_{key}"
                f.write(f"{key}<<{marker}\n{value}\n{marker}\n")
            else:
                f.write(f"{key}={value}\n")
    else:
        print(f"{key}={value}")


def shard_matrix_path(shard_index: int) -> Path:
    return GENERATED_DIR / SHARD_TEMPLATE.format(shard_index=shard_index)


def load_shard_rows() -> list[dict[str, Any]]:
    manifest = load_json_object(SHARD_MANIFEST)
    if int(manifest.get("entry_count", -1)) != EXPECTED_ATTEMPTS:
        raise OperatorError(f"shard manifest entry_count must be {EXPECTED_ATTEMPTS}")
    if int(manifest.get("visible_matrix_row_count", -1)) != EXPECTED_ROWS:
        raise OperatorError(f"shard manifest visible_matrix_row_count must be {EXPECTED_ROWS}")
    if int(manifest.get("shard_count", -1)) != EXPECTED_SHARDS:
        raise OperatorError(f"shard manifest shard_count must be {EXPECTED_SHARDS}")
    hotspot = manifest.get("hotspot_48")
    if not isinstance(hotspot, Mapping):
        raise OperatorError("shard manifest hotspot_48 must be an object")
    if (
        int(hotspot.get("plan_index", -1)) != 474
        or int(hotspot.get("shard_index", -1)) != 2
        or str(hotspot.get("problem_size")) != "48"
    ):
        raise OperatorError("hotspot problem_size 48 must remain at plan_index 474 in shard 2")

    rows: list[dict[str, Any]] = []
    seen_row_ids: set[str] = set()
    seen_plan_indices: set[int] = set()
    for shard_index in range(1, EXPECTED_SHARDS + 1):
        matrix_path = shard_matrix_path(shard_index)
        matrix = load_json_object(matrix_path)
        if int(matrix.get("shard_index", -1)) != shard_index:
            raise OperatorError(f"{repo_relative(matrix_path)} shard_index mismatch")
        include = matrix.get("include")
        if not isinstance(include, list) or not include:
            raise OperatorError(f"{repo_relative(matrix_path)} include must be a non-empty list")
        for row in include:
            if not isinstance(row, Mapping):
                raise OperatorError(f"{repo_relative(matrix_path)} include row must be an object")
            row_id = str(row.get("matrix_row_id", ""))
            if not row_id:
                raise OperatorError(f"{repo_relative(matrix_path)} include row missing matrix_row_id")
            if row_id in seen_row_ids:
                raise OperatorError(f"duplicate matrix_row_id {row_id}")
            seen_row_ids.add(row_id)
            attempts = row.get("attempts")
            if not isinstance(attempts, list) or not attempts:
                raise OperatorError(f"{row_id} attempts must be a non-empty list")
            if int(row.get("attempt_count", -1)) != len(attempts):
                raise OperatorError(f"{row_id} attempt_count must match attempts length")
            for attempt in attempts:
                if not isinstance(attempt, Mapping):
                    raise OperatorError(f"{row_id} attempt entries must be objects")
                plan_index = int(attempt.get("plan_index", -1))
                if plan_index in seen_plan_indices:
                    raise OperatorError(f"duplicate plan_index {plan_index}")
                seen_plan_indices.add(plan_index)
                if int(attempt.get("timeout_sec", -1)) != EXPECTED_TIMEOUT_SEC:
                    raise OperatorError(f"plan_index {plan_index} timeout_sec must be {EXPECTED_TIMEOUT_SEC}")
                if int(attempt.get("shard_index", -1)) != shard_index:
                    raise OperatorError(f"plan_index {plan_index} shard_index mismatch")
            rows.append(
                {
                    "shard_index": shard_index,
                    "shard_id": row.get("shard_id"),
                    "matrix_row_id": row_id,
                    "job_name": row.get("job_name") or f"Benchmark Discovery: {row.get('benchmark')}",
                    "benchmark": row.get("benchmark"),
                    "workflow_benchmark": row.get("workflow_benchmark") or row.get("benchmark"),
                    "attempt_count": len(attempts),
                    "plan_index_start": row.get("plan_index_start"),
                    "plan_index_end": row.get("plan_index_end"),
                    "timeout_sec": EXPECTED_TIMEOUT_SEC,
                }
            )

    expected = set(range(1, EXPECTED_ATTEMPTS + 1))
    if len(rows) != EXPECTED_ROWS or seen_plan_indices != expected:
        missing = sorted(expected - seen_plan_indices)
        extra = sorted(seen_plan_indices - expected)
        raise OperatorError(
            f"terminal matrix coverage mismatch: rows={len(rows)} missing={missing[:10]} extra={extra[:10]}"
        )
    return rows


def emit_matrix(_args: argparse.Namespace) -> int:
    rows = load_shard_rows()
    matrix = {"include": rows}
    matrix_text = json.dumps(matrix, separators=(",", ":"), sort_keys=True)
    append_github_output("terminal_matrix", matrix_text)
    append_github_output("benchmark_row_count", str(len(rows)))
    append_github_output("attempt_count", str(sum(int(row["attempt_count"]) for row in rows)))

    lines = [
        "# MI300A terminal-discovery matrix validation",
        "",
        f"- Shards: {EXPECTED_SHARDS}",
        f"- Visible benchmark rows: {len(rows)}",
        f"- Nested per-size attempts: {sum(int(row['attempt_count']) for row in rows)}",
        f"- Per-attempt timeout_sec: {EXPECTED_TIMEOUT_SEC}",
        "- Hotspot problem_size 48: plan_index 474 in shard 2",
        "",
        "| Shard | Benchmark | Attempts | Plan range | Matrix row id |",
        "|---:|---|---:|---|---|",
    ]
    for row in rows:
        lines.append(
            f"| {row['shard_index']} | `{row['benchmark']}` | {row['attempt_count']} | "
            f"{row['plan_index_start']}-{row['plan_index_end']} | `{row['matrix_row_id']}` |"
        )
    summary = "\n".join(lines) + "\n"
    Path("m18_terminal_discovery_matrix_summary.md").write_text(summary, encoding="utf-8")
    print(summary)
    return 0


def find_matrix_row(shard_index: int, matrix_row_id: str) -> Mapping[str, Any]:
    matrix_path = shard_matrix_path(shard_index)
    matrix = load_json_object(matrix_path)
    for row in matrix.get("include", []):
        if isinstance(row, Mapping) and row.get("matrix_row_id") == matrix_row_id:
            return row
    raise OperatorError(f"{repo_relative(matrix_path)} does not contain matrix_row_id {matrix_row_id!r}")


def expand_flags(flag_template: str, sizes: str) -> tuple[list[str], str]:
    size_for_flags = sizes
    label = sizes
    if "{p1}" in flag_template:
        parts = sizes.split("_")
        replacements = {f"{{p{idx}}}": part for idx, part in enumerate(parts, start=1)}
        expanded = flag_template
        for key, value in replacements.items():
            expanded = expanded.replace(key, value)
        label = "_".join(parts)
    elif ":" in sizes:
        actual, label = sizes.split(":", 1)
        expanded = flag_template.replace("{size}", actual)
    else:
        expanded = flag_template.replace("{size}", size_for_flags)
    return shlex.split(expanded), label


def set_memory_limit() -> None:
    try:
        import resource

        limit = 16 * 1024 * 1024 * 1024
        resource.setrlimit(resource.RLIMIT_AS, (limit, limit))
    except Exception:
        return


def run_process(command: Sequence[str], *, cwd: Path, timeout_sec: Optional[int] = None) -> tuple[int, str, bool]:
    start_new_session = hasattr(os, "setsid")
    proc = subprocess.Popen(
        list(command),
        cwd=str(cwd),
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        errors="replace",
        start_new_session=start_new_session,
    )
    try:
        stdout, _ = proc.communicate(timeout=timeout_sec)
        return proc.returncode, stdout or "", False
    except subprocess.TimeoutExpired:
        if start_new_session:
            try:
                os.killpg(proc.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
        else:
            proc.terminate()
        try:
            stdout, _ = proc.communicate(timeout=30)
        except subprocess.TimeoutExpired:
            if start_new_session:
                try:
                    os.killpg(proc.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
            else:
                proc.kill()
            stdout, _ = proc.communicate()
        return 124, stdout or "", True


def find_sqlite_file(work_dir: Path) -> Optional[Path]:
    candidates = sorted(work_dir.glob("akita_sim_*.sqlite3"))
    if not candidates:
        candidates = sorted(work_dir.glob("akita_data_recording_*.sqlite3"))
    return candidates[0] if candidates else None


def extract_kernel_time_ms(db_path: Path) -> Optional[float]:
    try:
        conn = sqlite3.connect(str(db_path))
        try:
            cur = conn.cursor()
            cur.execute("SELECT Value FROM mgpusim_metrics WHERE What='kernel_time' AND Location='Driver' LIMIT 1")
            row = cur.fetchone()
            if row is None:
                cur.execute("SELECT Value FROM mgpusim_metrics WHERE What='kernel_time' LIMIT 1")
                row = cur.fetchone()
        finally:
            conn.close()
    except sqlite3.Error:
        return None
    if row is None:
        return None
    try:
        value = float(row[0])
    except (TypeError, ValueError):
        return None
    return value / 1e9 if value > 1e6 else value * 1000.0


def extract_d2h_time_ms(db_path: Path) -> Optional[float]:
    try:
        conn = sqlite3.connect(str(db_path))
        try:
            cur = conn.cursor()
            cur.execute("SELECT Value FROM mgpusim_metrics WHERE What='d2h_time' AND Location='Driver' LIMIT 1")
            row = cur.fetchone()
        finally:
            conn.close()
    except sqlite3.Error:
        return None
    if row is None:
        return None
    try:
        value = float(row[0])
    except (TypeError, ValueError):
        return None
    return value / 1e9 if value > 1e6 else value * 1000.0


def outcome_base(row: Mapping[str, Any], attempt: Mapping[str, Any], output_dir: Path) -> dict[str, Any]:
    now = utc_now()
    run_id = os.environ.get("GITHUB_RUN_ID")
    repository = os.environ.get("GITHUB_REPOSITORY", "sarchlab/mgpusim-dev")
    run_url = f"https://github.com/{repository}/actions/runs/{run_id}" if run_id else None
    artifact_name = f"{ARTIFACT_PREFIX}-{row['matrix_row_id']}"
    plan_index = int(attempt["plan_index"])
    return {
        "schema_name": OUTCOME_SCHEMA,
        "schema_version": 1,
        "terminal_discovery": True,
        "terminal": True,
        "milestone": "",
        "collection_run_id": "",
        "issue": 239,
        "branch": os.environ.get("GITHUB_REF_NAME") or EXPECTED_BRANCH,
        "workflow": ".github/workflows/benchmark.yml",
        "run_id": int(run_id) if run_id and run_id.isdigit() else run_id,
        "run_attempt": os.environ.get("GITHUB_RUN_ATTEMPT"),
        "job_url": run_url,
        "job_database_id": int(run_id) if run_id and run_id.isdigit() else None,
        "job_conclusion": "success",
        "artifact_name": artifact_name,
        "artifact_path": repo_relative(output_dir),
        "shard_index": int(attempt["shard_index"]),
        "shard_id": attempt["shard_id"],
        "shard_count": int(attempt.get("shard_count", row.get("shard_count", EXPECTED_SHARDS))),
        "matrix_row_id": row["matrix_row_id"],
        "benchmark_attempt_index": int(attempt["benchmark_attempt_index"]),
        "shard_attempt_index": int(attempt["shard_attempt_index"]),
        "plan_index": plan_index,
        "runnable_unit_id": attempt["runnable_unit_id"],
        "attempt_name": attempt.get("attempt_name"),
        "benchmark": attempt.get("workflow_benchmark_name") or attempt.get("workflow_benchmark") or attempt.get("benchmark"),
        "plan_benchmark": attempt.get("plan_benchmark") or attempt.get("benchmark"),
        "workflow_benchmark": attempt.get("workflow_benchmark"),
        "workflow_benchmark_name": attempt.get("workflow_benchmark_name"),
        "hardware_kernel_name": attempt.get("hardware_kernel_name"),
        "problem_size": str(attempt.get("problem_size")),
        "hardware_problem_size": str(attempt.get("hardware_problem_size")),
        "sizes": str(attempt.get("sizes")),
        "size_token": str(attempt.get("size_token")),
        "size_label": str(attempt.get("size_label")),
        "flag_template": attempt.get("flag_template"),
        "size_label_template": attempt.get("size_label_template"),
        "timeout_sec": int(attempt.get("timeout_sec", EXPECTED_TIMEOUT_SEC)),
        "started_at": now,
        "finished_at": now,
    }


def finalize_outcome(outcome: dict[str, Any], *, status: str, detail: str, exit_code: Optional[int], started_at: str, kernel_time_ms: Optional[float] = None) -> dict[str, Any]:
    outcome["status"] = status
    outcome["detail"] = detail
    outcome["exit_code"] = exit_code
    outcome["started_at"] = started_at
    outcome["finished_at"] = utc_now()
    if kernel_time_ms is not None:
        outcome["kernel_time_ms"] = round(kernel_time_ms, 6)
    return outcome


def write_outcome_files(output_dir: Path, outcomes: Sequence[Mapping[str, Any]]) -> None:
    outcomes_dir = output_dir / "outcomes"
    outcomes_dir.mkdir(parents=True, exist_ok=True)
    for outcome in outcomes:
        plan_index = int(outcome["plan_index"])
        write_json(outcomes_dir / f"outcome_mi300a_terminal_discovery_plan_{plan_index:04d}.json", outcome)


def build_failure_outcomes(row: Mapping[str, Any], output_dir: Path, detail: str, exit_code: int) -> list[dict[str, Any]]:
    outcomes: list[dict[str, Any]] = []
    for attempt in row["attempts"]:
        started_at = utc_now()
        outcome = outcome_base(row, attempt, output_dir)
        outcomes.append(
            finalize_outcome(
                outcome,
                status="build_failed",
                detail=detail[:1000],
                exit_code=exit_code,
                started_at=started_at,
            )
        )
    return outcomes


def run_attempt(row: Mapping[str, Any], attempt: Mapping[str, Any], binary: Path, output_dir: Path) -> dict[str, Any]:
    benchmark = str(row["benchmark"])
    sizes = str(attempt["sizes"])
    flag_template = str(attempt["flag_template"])
    timeout_sec = int(attempt.get("timeout_sec", EXPECTED_TIMEOUT_SEC))
    started_at = utc_now()
    outcome = outcome_base(row, attempt, output_dir)

    flags, computed_label = expand_flags(flag_template, sizes)
    label = str(attempt.get("size_label") or computed_label)
    common = ["-timing", "-arch", "cdna3", "-gpu", "mi300a", "-disable-rtm"]
    extra_flags = shlex.split(str(row.get("extra_flags", "") or ""))
    command = [str(binary.resolve()), *common, *extra_flags, *flags]
    print(f"[RUN] plan_index={attempt['plan_index']} benchmark={benchmark} label={label} command={' '.join(shlex.quote(part) for part in command)}", flush=True)

    with tempfile.TemporaryDirectory(prefix=f"m18-terminal-{benchmark}-") as tmp:
        work_dir = Path(tmp)
        exit_code, stdout, timed_out = run_process(command, cwd=work_dir, timeout_sec=timeout_sec)
        log_path = output_dir / "logs" / f"stdout_plan_{int(attempt['plan_index']):04d}.log"
        log_path.parent.mkdir(parents=True, exist_ok=True)
        log_path.write_text(stdout, encoding="utf-8", errors="replace")
        outcome["stdout_log"] = repo_relative(log_path)
        outcome["command"] = command

        if timed_out or exit_code == 124:
            print(f"[TIMEOUT] plan_index={attempt['plan_index']} timeout_sec={timeout_sec}", flush=True)
            return finalize_outcome(
                outcome,
                status="timeout",
                detail=f"timed out after {timeout_sec}s",
                exit_code=124,
                started_at=started_at,
            )
        if exit_code != 0:
            print(f"[FAILED] plan_index={attempt['plan_index']} exit_code={exit_code}", flush=True)
            return finalize_outcome(
                outcome,
                status="failed",
                detail=f"benchmark exited with code {exit_code}",
                exit_code=exit_code,
                started_at=started_at,
            )

        db_path = find_sqlite_file(work_dir)
        if db_path is None:
            print(f"[MISSING_SQLITE] plan_index={attempt['plan_index']}", flush=True)
            return finalize_outcome(
                outcome,
                status="missing_sqlite",
                detail="benchmark completed but produced no akita sqlite metrics file",
                exit_code=exit_code,
                started_at=started_at,
            )
        if benchmark in BUS_SPEED_BENCHMARKS:
            kernel_time_ms = extract_d2h_time_ms(db_path)
            missing_detail = "sqlite metrics file did not contain d2h_time"
        else:
            kernel_time_ms = extract_kernel_time_ms(db_path)
            missing_detail = "sqlite metrics file did not contain kernel_time"
        if kernel_time_ms is None:
            print(f"[MISSING_KERNEL_TIME] plan_index={attempt['plan_index']} db={db_path.name}", flush=True)
            return finalize_outcome(
                outcome,
                status="missing_kernel_time",
                detail=missing_detail,
                exit_code=exit_code,
                started_at=started_at,
            )

    print(f"[OK] plan_index={attempt['plan_index']} kernel_time_ms={kernel_time_ms:.6f}", flush=True)
    outcome["csv_kernel_name"] = CSV_KERNEL_NAMES.get(benchmark, benchmark)
    return finalize_outcome(
        outcome,
        status="success",
        detail="sim_row_emitted",
        exit_code=0,
        started_at=started_at,
        kernel_time_ms=kernel_time_ms,
    )


def summarize_and_write(row: Mapping[str, Any], output_dir: Path, outcomes: Sequence[Mapping[str, Any]]) -> None:
    status_counts: dict[str, int] = {}
    for outcome in outcomes:
        status = str(outcome.get("status"))
        status_counts[status] = status_counts.get(status, 0) + 1

    artifact_name = f"{ARTIFACT_PREFIX}-{row['matrix_row_id']}"
    container = {
        "schema_name": BENCHMARK_OUTCOME_SCHEMA,
        "schema_version": 1,
        "outcome_schema_version": 1,
        "terminal_discovery": True,
        "terminal": True,
        "milestone": "",
        "collection_run_id": "",
        "issue": 239,
        "branch": os.environ.get("GITHUB_REF_NAME") or EXPECTED_BRANCH,
        "workflow": ".github/workflows/benchmark.yml",
        "artifact_name": artifact_name,
        "shard_index": int(row["shard_index"]),
        "shard_id": row.get("shard_id"),
        "shard_count": EXPECTED_SHARDS,
        "matrix_row_id": row["matrix_row_id"],
        "benchmark": row.get("benchmark"),
        "workflow_benchmark": row.get("workflow_benchmark") or row.get("benchmark"),
        "workflow_benchmark_name": row.get("workflow_benchmark") or row.get("benchmark"),
        "attempt_count": len(outcomes),
        "expected_attempt_count": int(row.get("attempt_count", len(outcomes))),
        "timeout_sec": EXPECTED_TIMEOUT_SEC,
        "status_counts": dict(sorted(status_counts.items())),
        "plan_index_start": row.get("plan_index_start"),
        "plan_index_end": row.get("plan_index_end"),
        "created_at": utc_now(),
        "outcomes": list(outcomes),
    }
    write_json(output_dir / "terminal_discovery_benchmark_outcomes.json", container)
    write_outcome_files(output_dir, outcomes)
    write_json(
        output_dir / "row_summary.json",
        {
            "matrix_row_id": row["matrix_row_id"],
            "benchmark": row.get("benchmark"),
            "attempt_count": len(outcomes),
            "status_counts": dict(sorted(status_counts.items())),
            "artifact_name": artifact_name,
        },
    )
    lines = [
        f"# terminal discovery row: {row.get('benchmark')}",
        "",
        f"- Matrix row id: `{row['matrix_row_id']}`",
        f"- Shard: {row['shard_index']}",
        f"- Attempts: {len(outcomes)}",
        f"- Artifact: `{artifact_name}`",
        "",
        "| Status | Count |",
        "|---|---:|",
    ]
    for status, count in sorted(status_counts.items()):
        lines.append(f"| `{status}` | {count} |")
    (output_dir / "row_summary.md").write_text("\n".join(lines) + "\n", encoding="utf-8")


def run_row(args: argparse.Namespace) -> int:
    row = dict(find_matrix_row(args.shard_index, args.matrix_row_id))
    output_dir = Path(args.output_dir)
    if not output_dir.is_absolute():
        output_dir = REPO_ROOT / output_dir
    output_dir.mkdir(parents=True, exist_ok=True)

    attempts = row.get("attempts")
    if not isinstance(attempts, list) or not attempts:
        raise OperatorError(f"{args.matrix_row_id} has no attempts")
    if int(row.get("attempt_count", -1)) != len(attempts):
        raise OperatorError(f"{args.matrix_row_id} attempt_count mismatch")
    for attempt in attempts:
        if int(attempt.get("timeout_sec", -1)) != EXPECTED_TIMEOUT_SEC:
            raise OperatorError(f"plan_index {attempt.get('plan_index')} timeout_sec must be {EXPECTED_TIMEOUT_SEC}")
    row["attempts"] = attempts

    benchmark = str(row["benchmark"])
    sample_dir = REPO_ROOT / "amd" / "samples" / benchmark
    if not sample_dir.is_dir():
        raise OperatorError(f"amd/samples/{benchmark} does not exist")

    binary = REPO_ROOT / f"_bench_{benchmark}"
    build_command = ["go", "build", "-o", str(binary), f"./amd/samples/{benchmark}/"]
    build_started = utc_now()
    print(f"[BUILD] {' '.join(shlex.quote(part) for part in build_command)}", flush=True)
    build_code, build_output, build_timed_out = run_process(build_command, cwd=REPO_ROOT, timeout_sec=args.build_timeout_sec)
    build_log = output_dir / "logs" / "build.log"
    build_log.parent.mkdir(parents=True, exist_ok=True)
    build_log.write_text(build_output, encoding="utf-8", errors="replace")

    if build_timed_out or build_code != 0:
        detail = f"go build {'timed out' if build_timed_out else 'failed'} with exit {build_code}; log={repo_relative(build_log)}"
        print(f"[BUILD_FAILED] {detail}", flush=True)
        outcomes = build_failure_outcomes(row, output_dir, detail, 124 if build_timed_out else build_code)
        summarize_and_write(row, output_dir, outcomes)
        return 0

    set_memory_limit()
    outcomes = []
    for attempt in attempts:
        outcomes.append(run_attempt(row, attempt, binary, output_dir))
        # Persist after each attempt so a runner interruption leaves best-effort evidence.
        write_outcome_files(output_dir, outcomes)
    summarize_and_write(row, output_dir, outcomes)
    print(
        json.dumps(
            {
                "matrix_row_id": row["matrix_row_id"],
                "benchmark": benchmark,
                "attempt_count": len(outcomes),
                "started_at": build_started,
                "finished_at": utc_now(),
            },
            sort_keys=True,
        )
    )
    return 0


def count_outcomes(args: argparse.Namespace) -> int:
    roots = [Path(root) for root in args.outcomes_root]
    sys.path.insert(0, str(REPO_ROOT / "scripts"))
    import collect_mi300a_terminal_discovery_provenance as collector

    outcome_roots = [root if root.is_absolute() else REPO_ROOT / root for root in roots]
    try:
        plan, plan_entries, by_index = collector.load_plan(collector.DEFAULT_PLAN)
        shard_by_plan = collector.load_shard_plan_index_map(collector.DEFAULT_SHARD_MANIFEST, len(plan_entries))
        sources = collector.collect_outcome_sources(outcome_roots)
        deduplicated = collector.deduplicate_outcome_sources(
            sources=sources,
            plan_entries=plan_entries,
            by_index=by_index,
            shard_by_plan=shard_by_plan,
        )
    except collector.CollectionError as exc:
        raise OperatorError(str(exc)) from exc

    dedup_report = collector.outcome_source_deduplication_report(deduplicated)
    selected_indices = set(deduplicated.selected_by_plan)
    expected = set(range(1, EXPECTED_ATTEMPTS + 1))
    missing = sorted(expected - selected_indices)
    extra = sorted(selected_indices - expected)
    hotspot_entry = by_index[474]
    complete = (
        dedup_report["logical_record_count"] == EXPECTED_ATTEMPTS
        and dedup_report["unique_plan_index_count"] == EXPECTED_ATTEMPTS
        and not missing
        and not extra
        and int(plan.get("timeout_sec", EXPECTED_TIMEOUT_SEC)) == EXPECTED_TIMEOUT_SEC
        and str(hotspot_entry.get("workflow_benchmark_name")) == "hotspot"
        and str(hotspot_entry.get("problem_size")) == "48"
        and 474 in selected_indices
    )
    report = {
        "record_count": dedup_report["logical_record_count"],
        "unique_plan_index_count": dedup_report["unique_plan_index_count"],
        "duplicate_plan_index_count": 0,
        "duplicate_plan_indices": [],
        "missing_plan_indices": missing[:20],
        "extra_plan_indices": extra[:20],
        "expected_plan_index_count": EXPECTED_ATTEMPTS,
        "timeout_sec": EXPECTED_TIMEOUT_SEC,
        "plan_metadata_validated": True,
        "hotspot_48": {
            "covered": 474 in selected_indices,
            "plan_index": 474,
            "benchmark": hotspot_entry.get("workflow_benchmark_name"),
            "problem_size": str(hotspot_entry.get("problem_size")),
            "timeout_sec": EXPECTED_TIMEOUT_SEC,
        },
        "raw_source_record_count": dedup_report["raw_source_record_count"],
        "raw_source_file_count": dedup_report["raw_source_file_count"],
        "raw_duplicate_plan_index_count": dedup_report["raw_duplicate_plan_index_count"],
        "raw_duplicate_plan_index_ranges": dedup_report["raw_duplicate_plan_index_ranges"],
        "outcome_source_deduplication": dedup_report,
        "complete": complete,
    }
    print(json.dumps(report, indent=2, sort_keys=True))
    if not report["complete"]:
        return 1
    return 0


def parse_args(argv: Optional[Sequence[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="MI300A terminal-discovery workflow helper")
    sub = parser.add_subparsers(dest="command", required=True)

    matrix = sub.add_parser("matrix", help="Validate shard surface and emit compact Actions matrix")
    matrix.set_defaults(func=emit_matrix)

    run = sub.add_parser("run-row", help="Run one benchmark-level terminal-discovery matrix row")
    run.add_argument("--shard-index", type=int, required=True)
    run.add_argument("--matrix-row-id", required=True)
    run.add_argument("--output-dir", default="terminal-discovery-row-output")
    run.add_argument("--build-timeout-sec", type=int, default=1800)
    run.set_defaults(func=run_row)

    count = sub.add_parser("count-outcomes", help="Validate exact 1416-record outcome presence before provenance collection")
    count.add_argument("--outcomes-root", action="append", required=True)
    count.set_defaults(func=count_outcomes)

    return parser.parse_args(argv)


def main(argv: Optional[Sequence[str]] = None) -> int:
    args = parse_args(argv)
    try:
        return int(args.func(args))
    except OperatorError as exc:
        print(f"FAIL: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
