"""Check the eight shared_mem_bandwidth slopes against MI300X v1.1."""

import argparse
from pathlib import Path
from statistics import linear_regression
from tempfile import TemporaryDirectory

from workflow import kernel_names, paired_timings, read_reference, run_case


def slopes(series, binary, scratch):
    cases = {case["x"]: case for case in series["cases"]}
    xs = (64, 128, 256)
    hardware, simulated = [], []
    for x in xs:
        case = cases[x]
        directory = scratch / str(case["case"])
        result = run_case(binary, case, directory, remaining=600)
        if result["status"] != "complete":
            raise RuntimeError(f"{series['parameters']}, inner_iters={x}: {result}")
        real, sim = case["launches"], result["launches"]
        paired = paired_timings(series["benchmark"], real, sim, kernel_names(sim))
        if len(paired) != 1 or paired[0] is None:
            raise ValueError(f"Unmatched dispatch for {series['parameters']}, inner_iters={x}")
        hardware.append(real[0][-1] / 1000)
        simulated.append(paired[0] / 1000)
    return linear_regression(xs, hardware).slope, linear_regression(xs, simulated).slope


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("binary", type=Path, help="built shared_mem_bandwidth timing sample")
    args = parser.parse_args()
    binary = args.binary.resolve(strict=True)
    reference = read_reference(Path(__file__).with_name("mi300x-v1.1.json.gz"))
    series = [s for s in reference["series"] if s["benchmark"] == "shared_mem_bandwidth"]
    configs = [(s["parameters"]["block_size"], s["parameters"]["access_pattern"],
                s["parameters"]["num_blocks"]) for s in series]
    expected = {(b, p, 1) for b in (32, 64, 256, 1024) for p in ("conflict", "no_conflict")}
    if len(series) != len(expected) or set(configs) != expected:
        raise ValueError(f"Unexpected LDS reference configurations: {configs}")

    errors = []
    print("threads pattern       HW us/iter  Sim us/iter  relative error", flush=True)
    with TemporaryDirectory() as folder:
        for (threads, pattern, _), row in sorted(zip(configs, series, strict=True)):
            hw, sim = slopes(row, binary, Path(folder))
            error = sim / hw - 1
            print(f"{threads:7d} {pattern:11s} {hw:10.4f} {sim:12.4f} {error:+13.2%}", flush=True)
            errors.append(abs(error))
    if max(errors) > 0.05:
        raise SystemExit("At least one LDS slope exceeds 5% relative error.")


if __name__ == "__main__":
    main()
