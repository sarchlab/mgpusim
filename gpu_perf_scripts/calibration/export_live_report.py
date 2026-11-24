"""Export an MGPUSim MI300X live sweep for the shared calibration report."""

import argparse
from collections import Counter
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import time

UNITS = {"array_bytes": "B", "buffer_size_mb": "MiB", "cacheline_bytes": "B",
         "grid_spacing": "Å", "amb_temp": "°C"}
GROUP = "mi300x"
STATES = {"queued", "running", "failed", "verification_failed", "timeout", "memory_limit",
          "unsupported", "skipped_after_timeout"}
SYMBOLS = {"running": "▶️", "verification_failed": "⚠️", "timeout": "⏱️",
           "skipped_after_timeout": "↷"}


def parse_parameters(series):
    """Recover typed fixed parameters from the archived sweep's series label."""
    if not series or series in {"默认参数", "Default parameters"}:
        return {}
    params = {}
    for entry in series.split(", "):
        key, value = entry.split("=", 1)
        if key in params:
            raise ValueError(f"Duplicate parameter {key}")
        try:
            scalar = json.loads(value)
        except json.JSONDecodeError:
            scalar = value
        if not isinstance(scalar, (str, bool, int, float)):
            raise ValueError(f"Invalid scalar parameter {key}={value}")
        params[key] = scalar
    return params


def nanoseconds(value, scale):
    return None if value is None else round(value * scale)


def render_timestamp(timestamp):
    if timestamp is None:
        return None
    return datetime.fromtimestamp(timestamp, timezone.utc).isoformat(timespec="seconds")


def run_progress(snapshot):
    cases = snapshot["cases"]
    counts = Counter(case["status"] for case in cases)
    starts = []
    for case in cases:
        start = case.get("started_at")
        if start is None and case.get("finished_at") is not None and case.get("wall_seconds") is not None:
            start = case["finished_at"] - case["wall_seconds"]
        if start is not None:
            starts.append(start)
    started_at = snapshot.get("started_at")
    if started_at is None:
        started_at = min(starts, default=None)
    return {
        "started_at": render_timestamp(started_at),
        "updated_at": render_timestamp(snapshot.get("updated_at")),
        "total": len(cases),
        "completed": counts["complete"],
        "running": counts["running"],
        "queued": counts["queued"],
        "unsuccessful": len(cases) - counts["complete"] - counts["running"] - counts["queued"],
    }


def convert(snapshot):
    benchmarks = {}
    points = []
    seen = set()
    for case in snapshot["cases"]:
        name, axis = case["benchmark"], case["axis"]
        params = case.get("non_scaling_params")
        if params is None:
            params = parse_parameters(case["series"])
        if axis in params:
            raise ValueError(f"Scaling parameter {axis} also appears in {name}'s fixed parameters")
        hardware_name = (
            "MI300X transfer time" if case["kind"] == "transfer" else "MI300X GPU event time"
        )
        metadata = {
            "scaling_param": axis,
            "params": {key: {"unit": UNITS.get(key, "")} for key in (axis, *params)},
            "values": {
                "mi300x": {
                    "name": hardware_name, "unit": "ns", "role": "hardware_measurement",
                },
                "mgpusim": {
                    "name": "MGPUSim kernel time", "unit": "ns", "role": "simulator_output",
                },
            },
        }
        if benchmarks.setdefault(name, metadata) != metadata:
            raise ValueError(f"Inconsistent metadata for {name}")
        identity = name, case["x"], tuple(sorted(params.items()))
        if identity in seen:
            raise ValueError(f"Duplicate configuration {identity}")
        seen.add(identity)
        point = {
            "group": GROUP,
            "benchmark": name,
            "x": case["x"],
            "non_scaling_params": params,
            "values": {
                "mi300x": nanoseconds(case.get("hardware_ms"), 1_000_000),
                "mgpusim": nanoseconds(case.get("kernel_time_s"), 1_000_000_000),
            },
        }
        state = case.get("failure_kind", case["status"])
        if state in STATES and (point["values"]["mgpusim"] is None or state == "verification_failed"):
            status = {"state": "unsupported" if state == "skipped_after_timeout" else state}
            if state in SYMBOLS:
                status["symbol"] = SYMBOLS[state]
            reason = case.get("failure_reason") or case.get("error") or case.get("unsupported_reason")
            if reason:
                status["reason"] = f"Skipped after timeout. {reason}" if state == "skipped_after_timeout" else reason
            point["statuses"] = {"mgpusim": status}
        points.append(point)

    progress = run_progress(snapshot)
    return {
        "report": {
            "title": "MGPUSim MI300X calibration",
            "description": "Kernel time from the GPU Benchmarks v1.1 MI300X measurements and the matching MGPUSim configurations.",
            "live": snapshot.get("live", progress["running"] > 0 or progress["queued"] > 0),
            "groups": {GROUP: {"name": "AMD MI300X"}},
        },
        "progress": progress,
        "benchmarks": benchmarks,
        "data": points,
    }


def publish(snapshot_paths, output):
    snapshots = [json.loads(path.read_text()) for path in snapshot_paths]
    if len(snapshots) == 1:
        combined = snapshots[0]
    else:
        starts = [item["started_at"] for item in snapshots if item.get("started_at") is not None]
        updates = [item["updated_at"] for item in snapshots if item.get("updated_at") is not None]
        combined = {
            "cases": [case for item in snapshots for case in item["cases"]],
            "started_at": min(starts, default=None),
            "updated_at": max(updates, default=None),
            "live": any(item.get("live", any(case["status"] in {"queued", "running"}
                                             for case in item["cases"])) for item in snapshots),
        }
    data = convert(combined)
    output.mkdir(parents=True, exist_ok=True)
    path = output / "data.json"
    temporary = path.with_suffix(".tmp")
    temporary.write_text(json.dumps(data, separators=(",", ":"), allow_nan=False) + "\n")
    os.replace(temporary, path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("snapshot", type=Path, help="MI300X collector's live snapshot")
    parser.add_argument("output", type=Path, help="directory for the shared report files")
    parser.add_argument("--add-snapshot", type=Path, action="append", default=[],
                        help="merge another benchmark snapshot into the report (repeatable)")
    parser.add_argument("--watch", type=float, metavar="SECONDS", help="republish when the input changes")
    args = parser.parse_args()
    if args.watch is not None and args.watch <= 0:
        parser.error("--watch must be positive")

    snapshots = [args.snapshot, *args.add_snapshot]
    last_modified = None
    while True:
        try:
            modified = tuple(path.stat().st_mtime_ns for path in snapshots)
            if modified != last_modified:
                publish(snapshots, args.output)
                last_modified = modified
        except (OSError, ValueError, KeyError) as error:
            if args.watch is None or last_modified is None:
                raise
            print(f"Calibration export failed: {error}", flush=True)
        if args.watch is None:
            return
        time.sleep(args.watch)


if __name__ == "__main__":
    main()
