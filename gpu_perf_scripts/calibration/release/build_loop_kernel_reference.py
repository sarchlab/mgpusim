"""Reduce instruction_cost and loop_wait collections to a supplementary reference.

Each input is PLAN:COLLECTION in the format build_reference.py reads. Only the
measured ic_* or lw_* kernel of a repetition is kept. A configuration (series
label and iterations) measured by a later input replaces the earlier one.

A series is one benchmark, instruction and launch shape, which keeps the
release workflow within its 256 jobs. The dependency or layout and
ops_per_iter of each configuration are case parameters.
"""

import argparse
import json
from pathlib import Path

from build_reference import fixed_parameters, medians, provenance, repetitions, write

HARDWARE = "MI300X VF kernel time (median, supplementary collection)"
# Per benchmark, the series parameters other than the launch shape and the
# case parameters. Each becomes the sample flag of the same name.
PARAMETERS = {
    "instruction_cost": (("instruction",), ("dependency", "ops_per_iter")),
    "loop_wait": (("instruction",), ("layout", "ops_per_iter")),
}
SHAPE = ("num_blocks", "threads_per_block")


def kernel(benchmark, p):
    """The kernel the sample launches, as instruction_cost.hip and loop_wait.hip select it."""
    if benchmark == "loop_wait":
        return f"lw_{p['layout']}_{p['instruction']}_k{p['ops_per_iter']}"
    if p["ops_per_iter"] == 0:
        return "ic_empty"
    if p["instruction"].startswith("s_"):
        return f"ic_{p['instruction']}_k{p['ops_per_iter']}"
    return f"ic_{p['instruction']}_{p['dependency']}_k{p['ops_per_iter']}"


def configurations(plan, directory):
    """Measured configurations, by benchmark, series label and iterations."""
    cases = {case["case"]: case for case in plan if case["benchmark"] in PARAMETERS}
    runs = repetitions(directory, cases, keep=lambda name: name.startswith(("ic_", "lw_")))
    measured, dropped = medians(cases, runs)
    found = {}
    for case_id, result in measured.items():
        case = cases[case_id]
        p = fixed_parameters(case["series"])
        series_keys, case_keys = PARAMETERS[case["benchmark"]]
        if case["axis"] != "iterations" or set(p) != {*series_keys, *case_keys, *SHAPE}:
            raise ValueError(f"Case {case_id} has unexpected parameters: {case['series']}")
        threads = p["threads_per_block"]
        launch = [kernel(case["benchmark"], p), p["num_blocks"] * threads, 1, 1, threads, 1, 1]
        launches = result["launches"]
        if len(launches) != 1 or launches[0][0].split("(")[0] != launch[0] \
                or launches[0][1:-1] != launch[1:]:
            raise ValueError(f"Case {case_id} measured {launches}, expected {launch}")
        found[(case["benchmark"], case["series"], case["x"])] = (case_id, p, result)
    return found, dropped


def build(inputs):
    found, dropped, replaced = {}, {}, []
    for plan, directory in inputs:
        measured, lost = configurations(plan, directory)
        replaced += sorted(found[key][0] for key in measured.keys() & found.keys())
        found.update(measured)
        for reason, cases in lost.items():
            dropped.setdefault(reason, []).extend(cases)

    groups = {}
    for (benchmark, _, x), (case_id, p, result) in found.items():
        series_keys, case_keys = PARAMETERS[benchmark]
        fixed = {key: p[key] for key in (*series_keys, *SHAPE)}
        varied = {key: p[key] for key in case_keys}
        args = [f"-{key.replace('_', '-')}={value}" for key, value in {**fixed, **varied}.items()]
        groups.setdefault(
            (benchmark, *fixed.values()),
            {
                "benchmark": benchmark,
                "sample": benchmark,
                "axis": "iterations",
                "parameters": fixed,
                "hardware": HARDWARE,
                "cases": [],
            },
        )["cases"].append(
            {
                "case": case_id,
                "x": x,
                "parameters": varied,
                "args": [*args, f"-iterations={x}"],
                **result,
            }
        )
    series = sorted(
        groups.values(),
        key=lambda s: (s["benchmark"], s["parameters"]["threads_per_block"],
                       s["parameters"]["instruction"]),
    )
    for row in series:
        row["cases"].sort(key=lambda case: (*case["parameters"].values(), case["x"]))
    return series, dropped, replaced


def main():
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument("inputs", nargs="+", metavar="PLAN:COLLECTION")
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    inputs = [tuple(map(Path, pair.split(":", 1))) for pair in args.inputs]
    series, dropped, replaced = build(
        [(json.loads(plan.read_text()), directory) for plan, directory in inputs]
    )
    for reason, cases in dropped.items():
        print(f"Dropped for {reason.replace('_', ' ')}: {cases}")
    print(f"Replaced by a later collection: {len(replaced)} cases")
    write(
        args.output,
        {
            "collections": [provenance(plan, directory) for plan, directory in inputs],
            "dropped_cases": dropped,
            "replaced_cases": replaced,
            "series": series,
        },
    )


if __name__ == "__main__":
    main()
