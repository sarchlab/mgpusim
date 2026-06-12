#!/usr/bin/env python3
"""Legacy CI artifact re-merge workflow with fail-fast provenance guardrail."""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parent.parent


def run_step(name: str, cmd: list[str]) -> None:
    print(f"\n==> {name}", flush=True)
    print("$", " ".join(cmd), flush=True)
    result = subprocess.run(cmd, cwd=REPO_ROOT)
    if result.returncode != 0:
        print(f"FAIL: {name} failed with exit code {result.returncode}", file=sys.stderr)
        raise SystemExit(result.returncode)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Re-merge comparison_ci.csv from legacy CI artifacts with "
            "provenance guardrail"
        )
    )
    parser.add_argument("--artifacts-dir", default="ci_artifacts")
    parser.add_argument("--manifest", default="ci_artifacts/provenance_manifest.json")
    parser.add_argument("--comparison", default="benchmark-comparison/comparison_ci.csv")
    parser.add_argument("--output", default="benchmark-comparison/comparison_ci.csv")
    parser.add_argument(
        "--clear",
        action="store_true",
        help="Clear existing sim columns before merging (recommended for single-run reproducibility)",
    )
    parser.add_argument("--expected-run-id")
    parser.add_argument("--expected-benchmark-ref-sha")
    parser.add_argument(
        "--skip-guardrail",
        action="store_true",
        help="Skip provenance check (not recommended)",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()

    python = sys.executable

    if not args.skip_guardrail:
        guardrail_cmd = [
            python,
            "scripts/ci_artifacts_provenance.py",
            "check",
            "--artifacts-dir",
            args.artifacts_dir,
            "--manifest",
            args.manifest,
        ]
        if args.expected_run_id:
            guardrail_cmd.extend(["--expected-run-id", args.expected_run_id])
        if args.expected_benchmark_ref_sha:
            guardrail_cmd.extend(
                ["--expected-benchmark-ref-sha", args.expected_benchmark_ref_sha]
            )
        run_step("Guardrail provenance check", guardrail_cmd)

    merge_cmd = [
        python,
        "benchmark-comparison/merge_ci_results.py",
        "--artifacts-dir",
        args.artifacts_dir,
        "--comparison",
        args.comparison,
        "--output",
        args.output,
    ]
    if args.clear:
        merge_cmd.append("--clear")
    run_step("Merge CI results", merge_cmd)

    run_step(
        "Regenerate validation report",
        [python, "scripts/generate_validation_report.py"],
    )

    print("\nDONE: deterministic re-merge workflow completed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
