"""Reduce per-repetition code_fetch_latency dispatch timings to calibration inputs.

The input is a CSV with one row per repetition of one (body_ops, loops) cell:
the single dispatch of fetch_kernel_<body_ops> in that process.

Required columns: body_ops, loops, rep.
Duration, first match: Start_Timestamp and End_Timestamp (ns), Duration_ns,
dispatch_ns.
Optional columns: Kernel_Name, Grid_Size_{X,Y,Z} and Workgroup_Size_{X,Y,Z};
without the profiler's name and size columns the launch the benchmark makes by
default is assumed. A row is dropped unless each of these columns, when
present, has the value shown: status=ok, rc=0, n_dispatch=1,
n_fetch_dispatches=1, op=fma.
"""

import argparse
import csv
import gzip
import hashlib
import json
from collections import defaultdict
from pathlib import Path
from statistics import median

BENCHMARK = "code_fetch_latency"
HARDWARE = "MI300X VF kernel time (median, supplementary collection)"
TIMING_SOURCE = "rocprofv3 --kernel-trace dispatch duration"
# The v1.1 plan numbers its cases below this.
FIRST_CASE = 100_001
THREADS = 64
SIZES = [f"{kind}_Size_{axis}" for kind in ("Grid", "Workgroup") for axis in "XYZ"]
KEPT = {"status": "ok", "rc": "0", "n_dispatch": "1", "n_fetch_dispatches": "1", "op": "fma"}


def duration(row):
    if row.get("Start_Timestamp") and row.get("End_Timestamp"):
        return int(row["End_Timestamp"]) - int(row["Start_Timestamp"])
    return int(row.get("Duration_ns") or row["dispatch_ns"])


def launch(row):
    body = int(row["body_ops"])
    name = row.get("Kernel_Name") or f"fetch_kernel_{body}(float*, float, float, int)"
    if name.removeprefix("void ").split("(", 1)[0].strip() != f"fetch_kernel_{body}":
        raise ValueError(f"{name} is not the kernel of body_ops={body}")
    shape = [int(row.get(key) or default) for key, default in zip(SIZES, [THREADS, 1, 1] * 2)]
    if shape != [THREADS, 1, 1, THREADS, 1, 1]:
        raise ValueError(f"body_ops={body}: expected one block of {THREADS} threads, got {shape}")
    return [name, *shape]


def build(rows):
    cells = defaultdict(dict)
    for row in rows:
        if any(row.get(column, value) != value for column, value in KEPT.items()):
            continue
        cell, rep = (int(row["loops"]), int(row["body_ops"])), int(row["rep"])
        where = f"loops={cell[0]} body_ops={cell[1]}"
        if rep in cells[cell]:
            raise ValueError(f"{where}: two dispatches in repetition {rep}")
        if duration(row) <= 0:
            raise ValueError(f"{where}: invalid duration")
        cells[cell][rep] = (launch(row), duration(row))

    groups = defaultdict(list)
    for index, ((loops, body), repeats) in enumerate(sorted(cells.items())):
        signatures = {tuple(signature) for signature, _ in repeats.values()}
        if len(signatures) != 1:
            raise ValueError(f"loops={loops} body_ops={body}: repetitions disagree on the launch")
        groups[loops].append(
            {
                "case": FIRST_CASE + index,
                "x": body,
                "args": [f"-body-ops={body}", f"-loops={loops}"],
                "repetitions": len(repeats),
                "launches": [[*signatures.pop(), median(ns for _, ns in repeats.values())]],
            }
        )
    return [
        {
            "benchmark": BENCHMARK,
            "sample": BENCHMARK,
            "axis": "body_ops",
            "parameters": {"loops": loops, "num_blocks": 1, "threads_per_block": THREADS},
            "hardware": HARDWARE,
            "cases": cases,
        }
        for loops, cases in groups.items()
    ]


def main():
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("timings", type=Path)
    parser.add_argument("output", type=Path)
    for name in ("benchmark-revision", "machine", "provider", "rocm-version", "collected-at"):
        parser.add_argument(f"--{name}", required=True)
    parser.add_argument(
        "--field",
        action="append",
        default=[],
        metavar="NAME=VALUE",
        help="further provenance to record, for example driver_version=7.1.3",
    )
    args = parser.parse_args()
    fields = dict(field.split("=", 1) for field in args.field)
    raw = args.timings.read_bytes()
    series = build(csv.DictReader(raw.decode().splitlines()))
    repetitions = sorted({case["repetitions"] for group in series for case in group["cases"]})
    result = {
        "benchmark_revision": args.benchmark_revision,
        "collection_finished_at": args.collected_at,
        "machine": args.machine,
        "provider": args.provider,
        "rocm_version": args.rocm_version,
        "timing_source": TIMING_SOURCE,
        "repetitions": repetitions[0] if len(repetitions) == 1 else repetitions,
        "raw_input": {"name": args.timings.name, "sha256": hashlib.sha256(raw).hexdigest()},
        **fields,
        "series": series,
    }
    args.output.write_bytes(
        gzip.compress(
            json.dumps(result, separators=(",", ":"), allow_nan=False).encode(), mtime=0
        )
    )
    print(f"{len(series)} series, {sum(len(g['cases']) for g in series)} cases.")


if __name__ == "__main__":
    main()
