#!/usr/bin/env python3
"""
Extract H100_raw.csv from a "global ranking" JSON export (the raw list of
per-config profile/sim records backing the H100 calibration dashboard).

Input is a JSON array of records shaped like:
    {
      "suite": "rodinia", "benchmark": "cfd",
      "param": {"size": "64", "block": "128"},
      "profile_type": "ncu",
      "profile_cycle": 162447,   # real hardware, in cycles
      "predict_cycle": 169504,   # simulator prediction, in cycles
      ... (trace/rank/frequency bookkeeping fields, not needed here)
    }

Output columns:
    profile_type, suite, benchmark, param_json, ground_truth_cycle, simulation_cycle

- param_json: the `param` dict, serialized to a JSON string (sorted keys,
  so identical param sets always produce identical text).
- ground_truth_cycle <- profile_cycle (real hardware)
- simulation_cycle   <- predict_cycle (simulator)
- suite "simtune" is renamed to "microbench" everywhere it appears (see
  SUITE_RENAME -- add more old->new mappings there if other suites get
  renamed later).
- Rows are sorted by (profile_type, suite, benchmark, param_json).

Usage:
    python3 extract_raw_data_from_web_json.py --json global-ranking.json --out H100_raw.csv
"""

import argparse
import csv
import json

# old suite name -> new suite name. Applied to every record's `suite` field.
SUITE_RENAME = {"simtune": "microbench"}


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--json", required=True, help="Path to the global-ranking JSON export")
    parser.add_argument("--out", default="H100_raw.csv", help="Output CSV path")
    args = parser.parse_args()

    with open(args.json) as f:
        records = json.load(f)

    rows = []
    for rec in records:
        suite = SUITE_RENAME.get(rec["suite"], rec["suite"])
        param_json = json.dumps(rec["param"], sort_keys=True, separators=(",", ":"))
        rows.append(
            (
                rec["profile_type"],
                suite,
                rec["benchmark"],
                param_json,
                rec["profile_cycle"],
                rec["predict_cycle"],
            )
        )

    rows.sort(key=lambda r: (r[0], r[1], r[2], r[3]))

    with open(args.out, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(
            ["profile_type", "suite", "benchmark", "param_json", "ground_truth_cycle", "simulation_cycle"]
        )
        writer.writerows(rows)

    print(f"Wrote {len(rows)} rows to {args.out}")


if __name__ == "__main__":
    main()