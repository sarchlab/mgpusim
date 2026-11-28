"""Reduce an MI300X dispatch collection to the calibration reference.

The collection directory holds collection.json, collection-current.log (one
JSON line per repetition) and, for every successful repetition,
dispatch/<benchmark>/<case>/<rep>/trace/measured-dispatches.csv. The plan is
the JSON list of cases the collection ran; only cases with simulator `args`
enter the reference.
"""

import argparse
import csv
import gzip
import hashlib
import json
from collections import defaultdict
from datetime import UTC, datetime
from pathlib import Path
from statistics import median

SIZES = [f"{kind}_Size_{axis}" for kind in ("Grid", "Workgroup") for axis in "XYZ"]


def fixed_parameters(label):
    def parse(value):
        try:
            return json.loads(value)
        except ValueError:
            return value

    if label in {"默认参数", "Default parameters", ""}:
        return {}
    return {
        key: parse(value)
        for key, value in (pair.split("=", 1) for pair in label.split(", "))
    }


def log(directory):
    with (directory / "collection-current.log").open() as file:
        return [json.loads(line) for line in file]


def repetitions(directory, cases, keep=lambda name: True):
    """Launches of each successful repetition of the cases, by case and repetition.

    A launch is the kernel name, grid and workgroup sizes and duration in ns,
    in start order. `keep` selects the measured rows by kernel name.
    """
    runs = defaultdict(dict)
    for row in log(directory):
        case = cases.get(row["case_id"])
        if row["status"] != "ok" or case is None:
            continue
        if row["benchmark"] != case["benchmark"]:
            raise ValueError(f"Wrong benchmark for case {row['case_id']}: {row['benchmark']}")
        path = (
            directory / "dispatch" / case["benchmark"] / str(row["case_id"]) / str(row["rep"])
            / "trace" / "measured-dispatches.csv"
        )
        with path.open(newline="") as file:
            rows = sorted(csv.DictReader(file), key=lambda r: int(r["Start_Timestamp"]))
        if len(rows) != row["measured_dispatches"]:
            raise ValueError(f"{path} does not have the logged {row['measured_dispatches']} rows")
        launches = [
            [r["Kernel_Name"], *(int(r[size]) for size in SIZES), int(r["Duration_ns"])]
            for r in rows
            if keep(r["Kernel_Name"])
        ]
        if not all(launch[-1] > 0 for launch in launches):
            raise ValueError(f"Non-positive duration in {path}")
        runs[row["case_id"]][row["rep"]] = launches
    return runs


def medians(cases, runs):
    """Median launches of each case and the cases that have none, by reason."""
    measured = {}
    dropped = {"no_successful_repetitions": [], "inconsistent_launches": []}
    for case_id in cases:
        reps = list(runs.get(case_id, {}).values())
        if not reps or not reps[0]:
            dropped["no_successful_repetitions"].append(case_id)
            continue
        signature = [launch[:-1] for launch in reps[0]]
        if any([launch[:-1] for launch in rep] != signature for rep in reps):
            dropped["inconsistent_launches"].append(case_id)
            continue
        measured[case_id] = {
            "repetitions": len(reps),
            "launches": [
                [*launch, median(rep[index][-1] for rep in reps)]
                for index, launch in enumerate(signature)
            ],
        }
    return measured, dropped


def provenance(plan_path, directory):
    """What the collection records about itself; checks that it ran this plan."""
    info = json.loads((directory / "collection.json").read_text())
    if hashlib.sha256(plan_path.read_bytes()).hexdigest() != info["plan_sha256"]:
        raise ValueError(f"{directory} did not run the plan {plan_path}")
    rows = [row for row in log(directory) if "started_at" in row]

    def utc(seconds):
        return datetime.fromtimestamp(seconds, UTC).isoformat(timespec="seconds")

    return {
        "benchmark_revision": info["benchmarks_revision"],
        "collection_started_at": utc(min(row["started_at"] for row in rows)),
        "collection_finished_at": utc(max(row["started_at"] + row["seconds"] for row in rows)),
        "core_clock_limit_mhz": info["core_clock_limit_mhz"],
        "max_sampled_core_clock_mhz": max(row.get("core_clock_mhz_max") or 0 for row in rows),
        "timer": info["timer"],
        "plan_sha256": info["plan_sha256"],
    }


def build(plan, directory):
    cases = {
        p["case"]: p
        for p in sorted(plan, key=lambda p: (p["benchmark"], p["series"], p["x"]))
        if "args" in p
    }
    measured, dropped = medians(cases, repetitions(directory, cases))
    groups = defaultdict(list)
    for case_id, result in measured.items():
        case = cases[case_id]
        if [launch[:-1] for launch in result["launches"]] != case["expected_launches"]:
            raise ValueError(f"Case {case_id} measured other launches than the plan expects")
        groups[(case["benchmark"], case["series"])].append(
            {"case": case_id, "x": case["x"], "args": case["args"], **result}
        )
    series = [
        {
            "benchmark": name,
            "sample": cases[rows[0]["case"]]["sample"],
            "axis": cases[rows[0]["case"]]["axis"],
            "parameters": fixed_parameters(label),
            "cases": rows,
        }
        for (name, label), rows in groups.items()
    ]
    return series, dropped


def write(path, result):
    path.write_bytes(
        gzip.compress(
            json.dumps(result, separators=(",", ":"), allow_nan=False).encode(), mtime=0
        )
    )
    series = result["series"]
    print(
        f"{len(series)} series, {sum(len(g['cases']) for g in series)} cases, "
        f"{sum(len(c['launches']) for g in series for c in g['cases'])} median launch timings."
    )


def main():
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("plan", type=Path)
    parser.add_argument("collection", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    series, dropped = build(json.loads(args.plan.read_text()), args.collection)
    for reason, cases in dropped.items():
        print(f"Dropped for {reason.replace('_', ' ')}: {cases}")
    write(
        args.output,
        {**provenance(args.plan, args.collection), "dropped_cases": dropped, "series": series},
    )


if __name__ == "__main__":
    main()
