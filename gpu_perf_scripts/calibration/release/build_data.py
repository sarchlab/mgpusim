"""Export one complete calibration dataset for the standard report viewer."""

import argparse
import json
from pathlib import Path

from workflow import atomic_json, read_reference, report


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("artifacts", type=Path)
    p.add_argument("output", type=Path)
    p.add_argument(
        "--reference",
        type=Path,
        default=Path(__file__).with_name("mi300x-v1.1.json.gz"),
    )
    p.add_argument("--version", required=True)
    p.add_argument("--revision", required=True)
    p.add_argument("--benchmark")
    args = p.parse_args()
    reference = read_reference(args.reference)
    results = {
        int(path.parent.name.removeprefix("series-")): json.loads(path.read_text())
        for path in args.artifacts.glob("series-*/results.json")
    }
    if args.benchmark:
        for series in reference["series"]:
            series["selection_excluded"] = series["benchmark"] != args.benchmark
    args.output.parent.mkdir(parents=True, exist_ok=True)
    atomic_json(args.output, report(reference, results, args.version, args.revision))


if __name__ == "__main__":
    main()
