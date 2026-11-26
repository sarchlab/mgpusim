"""Reduce the verified MI300X v1.1 dispatch archives to calibration inputs."""

import argparse
import csv
import gzip
import io
import json
import subprocess
import tarfile
from collections import defaultdict
from pathlib import Path
from statistics import median


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


def members(directory, pattern):
    for archive in sorted(directory.glob(pattern)):
        with subprocess.Popen(
            ["zstd", "-dc", str(archive)], stdout=subprocess.PIPE
        ) as decoder:
            with tarfile.open(fileobj=decoder.stdout, mode="r|") as stream:
                for info in stream:
                    if info.isfile():
                        yield Path(info.name), stream, info
            if decoder.wait():
                raise RuntimeError(f"Cannot decompress {archive}")


def successful_repeats(directory):
    for path, stream, info in members(directory, "collection-complete.tar.zst"):
        if path.name == "collection-current.log":
            with stream.extractfile(info) as file:
                for line in file:
                    row = json.loads(line)
                    if row["status"] == "ok":
                        yield (row["case_id"], row["rep"]), row["measured_dispatches"]


def hardware_launches(file):
    rows = sorted(
        csv.DictReader(io.StringIO(file.read().decode())),
        key=lambda row: int(row["Start_Timestamp"]),
    )
    return [
        [
            row["Kernel_Name"],
            *(
                int(row[f"{size}_Size_{axis}"])
                for size in ("Grid", "Workgroup")
                for axis in "XYZ"
            ),
            int(row["Duration_ns"]),
        ]
        for row in rows
    ]


def build(plan, directory):
    cases = {
        p["case"]: p
        for p in sorted(plan, key=lambda p: (p["benchmark"], p["series"], p["x"]))
        if not p.get("unsupported_reason")
    }
    expected = dict(successful_repeats(directory))
    repeats = defaultdict(dict)
    for path, stream, info in members(directory, "dispatch-*.tar.zst"):
        if path.name != "measured-dispatches.csv" or len(path.parts) != 6:
            continue
        _, benchmark, case_id, rep, _, _ = path.parts
        case_id, rep = int(case_id), int(rep)
        if case_id not in cases or (case_id, rep) not in expected:
            continue
        if benchmark != cases[case_id]["benchmark"]:
            raise ValueError(f"Wrong benchmark for case {case_id}: {benchmark}")
        launches = hardware_launches(stream.extractfile(info))
        if len(launches) != expected[(case_id, rep)] or not all(
            row[-1] > 0 for row in launches
        ):
            raise ValueError(f"Invalid hardware dispatches in {path}")
        if rep in repeats[case_id] and launches != repeats[case_id][rep]:
            raise ValueError(f"Conflicting repeated record in {path}")
        repeats[case_id][rep] = launches

    groups = defaultdict(list)
    inconsistent = missing = 0
    for case_id, case in cases.items():
        runs = list(repeats[case_id].values())
        if not runs or not runs[0]:
            missing += 1
            continue
        signature = [row[:-1] for row in runs[0]]
        if any([row[:-1] for row in run] != signature for run in runs):
            inconsistent += 1
            continue
        for launch, index in zip(signature, range(len(signature))):
            launch.append(median(run[index][-1] for run in runs))
        groups[(case["benchmark"], case["series"])].append(
            {
                "case": case_id,
                "x": case["x"],
                "args": case["args"],
                "repetitions": len(runs),
                "launches": signature,
            }
        )

    print(
        f"{missing} cases without kernel timings. {inconsistent} with inconsistent hardware launch sequences."
    )
    return [
        {
            "benchmark": name,
            "sample": cases[rows[0]["case"]]["sample"],
            "axis": cases[rows[0]["case"]]["axis"],
            "parameters": fixed_parameters(series),
            "cases": rows,
        }
        for (name, series), rows in groups.items()
    ]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("plan", type=Path)
    parser.add_argument("archives", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    series = build(json.loads(args.plan.read_text()), args.archives)
    result = {
        "benchmark_revision": "a8ca84f8ce4373b3c091d4dc97a8f067607847a9",
        "collection_finished_at": "2026-11-24T04:59:18+00:00",
        "gpu_clock_mhz": 1680,
        "series": series,
    }
    args.output.write_bytes(
        gzip.compress(
            json.dumps(result, separators=(",", ":"), allow_nan=False).encode(), mtime=0
        )
    )
    print(
        f"{len(series)} series, {sum(len(g['cases']) for g in series)} cases, "
        f"{sum(len(c['launches']) for g in series for c in g['cases'])} median launch timings."
    )


if __name__ == "__main__":
    main()
