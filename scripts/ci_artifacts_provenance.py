#!/usr/bin/env python3
"""Generate and verify legacy archive provenance for local CI artifacts.

The manifest captures:
- source workflow run id
- benchmark ref SHA
- context note
- generation timestamp
- deterministic artifact fingerprint
"""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import os
import sys
from pathlib import Path
from typing import Iterable, List, Tuple

SCHEMA_VERSION = 1


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        while True:
            chunk = f.read(1024 * 1024)
            if not chunk:
                break
            h.update(chunk)
    return h.hexdigest()


def iter_artifact_files(artifacts_dir: Path, excluded: Iterable[Path]) -> List[Path]:
    excluded_resolved = {p.resolve() for p in excluded}
    files: List[Path] = []
    for root, _, filenames in os.walk(artifacts_dir):
        root_path = Path(root)
        for filename in filenames:
            path = (root_path / filename).resolve()
            if path in excluded_resolved:
                continue
            files.append(path)
    files.sort(key=lambda p: p.relative_to(artifacts_dir.resolve()).as_posix())
    return files


def compute_fingerprint(artifacts_dir: Path, manifest_path: Path | None = None) -> Tuple[str, int, List[Tuple[str, str]]]:
    excluded = []
    if manifest_path is not None and manifest_path.exists():
        excluded.append(manifest_path)
    files = iter_artifact_files(artifacts_dir, excluded)

    entries: List[Tuple[str, str]] = []
    listing_lines: List[str] = []

    for path in files:
        relpath = path.relative_to(artifacts_dir.resolve()).as_posix()
        digest = sha256_file(path)
        entries.append((relpath, digest))
        listing_lines.append(f"{digest}  {relpath}\n")

    listing_blob = "".join(listing_lines).encode("utf-8")
    aggregate = hashlib.sha256(listing_blob).hexdigest()
    return aggregate, len(files), entries


def build_manifest(
    artifacts_dir: Path,
    run_id: str,
    benchmark_ref_sha: str,
    context: str,
    generated_at_utc: str,
    manifest_path: Path,
):
    fingerprint, file_count, _ = compute_fingerprint(artifacts_dir, manifest_path)
    return {
        "schema_version": SCHEMA_VERSION,
        "run_id": str(run_id),
        "benchmark_ref_sha": benchmark_ref_sha,
        "context": context,
        "generated_at_utc": generated_at_utc,
        "artifacts_dir": artifacts_dir.as_posix(),
        "fingerprint": {
            "algorithm": "sha256",
            "basis": "sha256 of sorted lines: '<file_sha256><two spaces><relative_path>\\n'",
            "file_count": file_count,
            "value": fingerprint,
        },
    }


def load_manifest(manifest_path: Path):
    with manifest_path.open("r", encoding="utf-8") as f:
        return json.load(f)


def validate_manifest_structure(manifest: dict) -> List[str]:
    errors: List[str] = []
    for key in ["schema_version", "run_id", "benchmark_ref_sha", "context", "generated_at_utc", "fingerprint"]:
        if key not in manifest:
            errors.append(f"missing field: {key}")
    fingerprint = manifest.get("fingerprint", {})
    for key in ["algorithm", "basis", "file_count", "value"]:
        if key not in fingerprint:
            errors.append(f"missing field: fingerprint.{key}")
    return errors


def check_manifest(
    manifest_path: Path,
    artifacts_dir: Path,
    expected_run_id: str | None,
    expected_benchmark_ref_sha: str | None,
) -> int:
    if not manifest_path.exists():
        print(f"FAIL: Manifest not found: {manifest_path}", file=sys.stderr)
        return 1
    if not artifacts_dir.exists():
        print(f"FAIL: Artifacts directory not found: {artifacts_dir}", file=sys.stderr)
        return 1

    manifest = load_manifest(manifest_path)
    errors = validate_manifest_structure(manifest)

    if expected_run_id and str(manifest.get("run_id")) != str(expected_run_id):
        errors.append(
            f"run_id mismatch: manifest={manifest.get('run_id')} expected={expected_run_id}"
        )
    if expected_benchmark_ref_sha and manifest.get("benchmark_ref_sha") != expected_benchmark_ref_sha:
        errors.append(
            "benchmark_ref_sha mismatch: "
            f"manifest={manifest.get('benchmark_ref_sha')} expected={expected_benchmark_ref_sha}"
        )

    actual_fingerprint, actual_count, _ = compute_fingerprint(artifacts_dir, manifest_path)
    manifest_fingerprint = manifest.get("fingerprint", {}).get("value")
    manifest_count = manifest.get("fingerprint", {}).get("file_count")

    if manifest_fingerprint != actual_fingerprint:
        errors.append(
            "fingerprint mismatch: "
            f"manifest={manifest_fingerprint} actual={actual_fingerprint}"
        )
    if manifest_count != actual_count:
        errors.append(
            f"file_count mismatch: manifest={manifest_count} actual={actual_count}"
        )

    if errors:
        print("FAIL: CI artifacts provenance guardrail rejected local artifacts.", file=sys.stderr)
        for err in errors:
            print(f"  - {err}", file=sys.stderr)
        return 1

    print("PASS: CI artifacts provenance guardrail check succeeded.")
    print(f"  manifest: {manifest_path}")
    print(f"  run_id: {manifest['run_id']}")
    print(f"  benchmark_ref_sha: {manifest['benchmark_ref_sha']}")
    print(f"  fingerprint: {actual_fingerprint}")
    print(f"  file_count: {actual_count}")
    return 0


def parse_args():
    parser = argparse.ArgumentParser(
        description="Manage legacy ci_artifacts provenance manifest"
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    write_cmd = subparsers.add_parser("write", help="Write/update provenance manifest")
    write_cmd.add_argument("--artifacts-dir", required=True)
    write_cmd.add_argument("--manifest", required=True)
    write_cmd.add_argument("--run-id", required=True)
    write_cmd.add_argument("--benchmark-ref-sha", required=True)
    write_cmd.add_argument("--context", required=True)
    write_cmd.add_argument(
        "--generated-at-utc",
        help="Override timestamp (RFC3339). Defaults to current UTC time.",
    )

    check_cmd = subparsers.add_parser("check", help="Verify provenance manifest matches local artifacts")
    check_cmd.add_argument("--artifacts-dir", required=True)
    check_cmd.add_argument("--manifest", required=True)
    check_cmd.add_argument("--expected-run-id")
    check_cmd.add_argument("--expected-benchmark-ref-sha")

    return parser.parse_args()


def main() -> int:
    args = parse_args()
    if args.command == "write":
        artifacts_dir = Path(args.artifacts_dir)
        manifest_path = Path(args.manifest)
        if not artifacts_dir.exists():
            print(f"ERROR: Artifacts directory not found: {artifacts_dir}", file=sys.stderr)
            return 1

        generated_at_utc = args.generated_at_utc
        if not generated_at_utc:
            generated_at_utc = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")

        manifest_path.parent.mkdir(parents=True, exist_ok=True)
        manifest = build_manifest(
            artifacts_dir=artifacts_dir,
            run_id=args.run_id,
            benchmark_ref_sha=args.benchmark_ref_sha,
            context=args.context,
            generated_at_utc=generated_at_utc,
            manifest_path=manifest_path,
        )
        with manifest_path.open("w", encoding="utf-8") as f:
            json.dump(manifest, f, indent=2)
            f.write("\n")

        print(f"Wrote provenance manifest: {manifest_path}")
        print(f"  run_id: {manifest['run_id']}")
        print(f"  benchmark_ref_sha: {manifest['benchmark_ref_sha']}")
        print(f"  fingerprint: {manifest['fingerprint']['value']}")
        print(f"  file_count: {manifest['fingerprint']['file_count']}")
        return 0

    if args.command == "check":
        return check_manifest(
            manifest_path=Path(args.manifest),
            artifacts_dir=Path(args.artifacts_dir),
            expected_run_id=args.expected_run_id,
            expected_benchmark_ref_sha=args.expected_benchmark_ref_sha,
        )

    print("ERROR: Unknown command", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
