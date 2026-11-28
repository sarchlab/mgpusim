"""Reduce per-repetition code_fetch_latency dispatch timings to calibration inputs.

The input is a CSV with one row per repetition of one (body_ops, loops) cell:
the single dispatch of fetch_kernel_<body_ops> in that process.

Required columns: body_ops, loops, rep.
Duration, first match: Start_Timestamp and End_Timestamp (ns), Duration_ns,
dispatch_ns.
Optional columns: Kernel_Name, Grid_Size_{X,Y,Z} and Workgroup_Size_{X,Y,Z}.
When any of them is present all must be, and filled in; without them the
launch the benchmark makes by default is assumed. A row is dropped unless each
of these columns, when present, has the value shown: status=ok, rc=0,
n_dispatch=1, n_fetch_dispatches=1, op=fma.

The collection must be complete: every body size for every loop count, each
with the expected number of repetitions.
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
# The sweep of the benchmark and the limits of its sample.
BODY_SIZES = [2**k for k in range(4, 17)]
LOOPS = [1, 4, 8, 16]
MAX_CHAIN = 1 << 20
THREADS = 64
SIZES = [f"{kind}_Size_{axis}" for kind in ("Grid", "Workgroup") for axis in "XYZ"]
PROFILER = ["Kernel_Name", *SIZES]
KEPT = {"status": "ok", "rc": "0", "n_dispatch": "1", "n_fetch_dispatches": "1", "op": "fma"}


def duration(row):
    if "Start_Timestamp" in row or "End_Timestamp" in row:
        if not (row.get("Start_Timestamp") and row.get("End_Timestamp")):
            raise ValueError("a start or end timestamp is missing")
        return int(row["End_Timestamp"]) - int(row["Start_Timestamp"])
    for column in ("Duration_ns", "dispatch_ns"):
        if row.get(column):
            return int(row[column])
    raise ValueError("no duration")


def launch(row, body):
    name, shape = f"fetch_kernel_{body}(float*, float, float, int)", [THREADS, 1, 1] * 2
    if any(column in row for column in PROFILER):
        if not all(row.get(column) for column in PROFILER):
            raise ValueError("the profiler's kernel name or launch sizes are incomplete")
        name, shape = row["Kernel_Name"], [int(row[column]) for column in SIZES]
    if name.removeprefix("void ").split("(", 1)[0].strip() != f"fetch_kernel_{body}":
        raise ValueError(f"{name} is not the kernel of this body size")
    if shape != [THREADS, 1, 1, THREADS, 1, 1]:
        raise ValueError(f"expected one block of {THREADS} threads, got {shape}")
    return [name, *shape]


def repetition(row):
    loops, body, rep = (int(row[column]) for column in ("loops", "body_ops", "rep"))
    where = f"loops={loops} body_ops={body} rep={rep}"
    try:
        if body not in BODY_SIZES:
            raise ValueError("unsupported body size")
        if loops < 1 or loops > MAX_CHAIN // body:
            raise ValueError(f"loops must be 1 to {MAX_CHAIN // body}")
        nanoseconds = duration(row)
        if nanoseconds <= 0:
            raise ValueError("the duration is not positive")
        return (loops, body), rep, (launch(row, body), nanoseconds)
    except ValueError as error:
        raise ValueError(f"{where}: {error}") from None


def build(rows, repetitions):
    cells = defaultdict(dict)
    for row in rows:
        if any(row.get(column, value) != value for column, value in KEPT.items()):
            continue
        cell, rep, measured = repetition(row)
        if rep in cells[cell]:
            raise ValueError(f"loops={cell[0]} body_ops={cell[1]}: two dispatches in rep={rep}")
        cells[cell][rep] = measured

    expected = {(loops, body) for loops in LOOPS for body in BODY_SIZES}
    if set(cells) != expected:
        missing, extra = sorted(expected - set(cells)), sorted(set(cells) - expected)
        raise ValueError(
            f"incomplete collection: {len(missing)} (loops, body_ops) cells missing "
            f"{missing[:3]}, {len(extra)} unexpected {extra[:3]}"
        )
    uneven = {cell: len(repeats) for cell, repeats in cells.items() if len(repeats) != repetitions}
    if uneven:
        raise ValueError(
            f"expected {repetitions} repetitions per cell; "
            f"{len(uneven)} cells differ, for example {sorted(uneven.items())[:3]}"
        )

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
        "--repetitions",
        type=int,
        default=20,
        help="successful repetitions every configuration must have (default: 20)",
    )
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
    series = build(csv.DictReader(raw.decode().splitlines()), args.repetitions)
    result = {
        "benchmark_revision": args.benchmark_revision,
        "collection_finished_at": args.collected_at,
        "machine": args.machine,
        "provider": args.provider,
        "rocm_version": args.rocm_version,
        "timing_source": TIMING_SOURCE,
        "repetitions": args.repetitions,
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
