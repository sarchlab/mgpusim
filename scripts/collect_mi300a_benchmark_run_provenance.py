#!/usr/bin/env python3
"""Collect reproducible MI300A Benchmark run provenance from GitHub Actions.

The collector intentionally separates raw GitHub metadata, raw artifact ZIP
fingerprints, extracted artifact inventories, and a curated selected-run file
hash inventory. It is safe to run while a workflow is still active: the README
and run metadata are marked as a non-terminal snapshot and must not be treated
as clean completion or finishability evidence until a later terminal collection
records ``status=completed``.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import zipfile
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Iterable, Mapping, Sequence

RUN_VIEW_FIELDS = (
    "databaseId,status,conclusion,headBranch,headSha,event,displayTitle,name,"
    "workflowName,createdAt,updatedAt,url,jobs,attempt,startedAt,"
    "workflowDatabaseId,number"
)
JOBS_FIELDS = "jobs"
DEFAULT_CURATED_ARTIFACTS = (
    "validation-summary",
    "benchmark-comparison",
    "comparison-detailed",
    "regression-report",
    "validation-report",
    "accuracy-report",
)


class CollectionError(RuntimeError):
    """Raised when provenance collection cannot complete."""


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace(
        "+00:00", "Z"
    )


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def json_dump(path: Path, data: object) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def run_command(args: Sequence[str], *, binary_stdout_path: Path | None = None) -> str:
    if binary_stdout_path is not None:
        binary_stdout_path.parent.mkdir(parents=True, exist_ok=True)
        with binary_stdout_path.open("wb") as out:
            result = subprocess.run(args, stdout=out, stderr=subprocess.PIPE, check=False)
        stderr = result.stderr.decode("utf-8", errors="replace")
        stdout = ""
    else:
        result = subprocess.run(args, capture_output=True, text=True, check=False)
        stderr = result.stderr
        stdout = result.stdout
    if result.returncode != 0:
        raise CollectionError(
            f"command failed ({result.returncode}): {' '.join(args)}\n{stderr.strip()}"
        )
    return stdout


def gh_json(args: Sequence[str]) -> object:
    text = run_command(args)
    try:
        return json.loads(text)
    except json.JSONDecodeError as exc:
        raise CollectionError(f"invalid JSON from {' '.join(args)}: {exc}") from exc


def collect_run_view(repo: str, run_id: str) -> dict:
    data = gh_json(["gh", "run", "view", run_id, "--repo", repo, "--json", RUN_VIEW_FIELDS])
    if not isinstance(data, dict):
        raise CollectionError("gh run view did not return a JSON object")
    return data


def collect_jobs(repo: str, run_id: str) -> dict:
    data = gh_json(["gh", "run", "view", run_id, "--repo", repo, "--json", JOBS_FIELDS])
    if not isinstance(data, dict) or not isinstance(data.get("jobs"), list):
        raise CollectionError("gh run view --json jobs did not return a jobs list")
    return data


def collect_artifacts(repo: str, run_id: str) -> list[dict]:
    text = run_command(
        [
            "gh",
            "api",
            f"repos/{repo}/actions/runs/{run_id}/artifacts",
            "--paginate",
            "--jq",
            ".artifacts[] | @json",
        ]
    )
    artifacts: list[dict] = []
    for line in text.splitlines():
        if not line.strip():
            continue
        item = json.loads(line)
        if not isinstance(item, dict):
            raise CollectionError("artifact list contained a non-object row")
        artifacts.append(item)
    artifacts.sort(key=lambda item: (str(item.get("created_at", "")), str(item.get("name", "")), int(item.get("id", 0))))
    return artifacts


def duration_seconds(started_at: str | None, completed_at: str | None) -> int | None:
    if not started_at or not completed_at or completed_at.startswith("0001-"):
        return None
    try:
        start = datetime.fromisoformat(started_at.replace("Z", "+00:00"))
        end = datetime.fromisoformat(completed_at.replace("Z", "+00:00"))
    except ValueError:
        return None
    return int((end - start).total_seconds())


def benchmark_name(job_name: str) -> str:
    match = re.match(r"^Tier [12] Bench: (?P<benchmark>.+)$", job_name)
    return match.group("benchmark") if match else ""


def run_step(job: Mapping[str, object]) -> Mapping[str, object]:
    for step in job.get("steps") or []:
        if isinstance(step, Mapping) and step.get("name") == "Run benchmark sizes":
            return step
    return {}


def summarize_jobs(jobs_data: Mapping[str, object]) -> tuple[list[dict], dict]:
    rows: list[dict] = []
    status_counts: Counter[str] = Counter()
    conclusion_counts: Counter[str] = Counter()
    benchmark_counts: Counter[str] = Counter()
    for job in jobs_data.get("jobs") or []:
        if not isinstance(job, Mapping):
            continue
        name = str(job.get("name") or "")
        status = str(job.get("status") or "")
        conclusion = str(job.get("conclusion") or "")
        step = run_step(job)
        bench = benchmark_name(name)
        duration = duration_seconds(
            str(job.get("startedAt") or ""),
            str(job.get("completedAt") or ""),
        )
        rows.append(
            {
                "job_id": job.get("databaseId") or job.get("id") or "",
                "name": name,
                "benchmark": bench,
                "status": status,
                "conclusion": conclusion,
                "started_at": job.get("startedAt") or "",
                "completed_at": job.get("completedAt") or "",
                "duration_sec": "" if duration is None else duration,
                "run_step_status": step.get("status") or "",
                "run_step_conclusion": step.get("conclusion") or "",
                "url": job.get("url") or "",
            }
        )
        status_counts[status or "empty"] += 1
        conclusion_counts[conclusion or "empty"] += 1
        if bench:
            benchmark_counts[conclusion or status or "empty"] += 1
    rows.sort(key=lambda item: (str(item["started_at"]), str(item["name"])))
    summary = {
        "total_jobs": len(rows),
        "status_counts": dict(sorted(status_counts.items())),
        "conclusion_counts": dict(sorted(conclusion_counts.items())),
        "benchmark_jobs": sum(1 for row in rows if row["benchmark"]),
        "benchmark_conclusion_or_status_counts": dict(sorted(benchmark_counts.items())),
    }
    return rows, summary


def write_tsv(path: Path, fieldnames: Sequence[str], rows: Iterable[Mapping[str, object]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames, delimiter="\t", lineterminator="\n")
        writer.writeheader()
        for row in rows:
            writer.writerow({name: row.get(name, "") for name in fieldnames})


def write_artifacts_tsv(path: Path, artifacts: Sequence[Mapping[str, object]]) -> None:
    fields = [
        "id",
        "name",
        "size_in_bytes",
        "digest",
        "expired",
        "created_at",
        "updated_at",
        "expires_at",
        "archive_download_url",
    ]
    write_tsv(path, fields, artifacts)


def safe_name(name: str) -> str:
    text = re.sub(r"[^A-Za-z0-9_.-]+", "-", name.strip())
    return text.strip(".-") or "artifact"


def download_raw_zips(repo: str, artifacts: Sequence[Mapping[str, object]], raw_dir: Path) -> tuple[list[dict], list[dict]]:
    zip_rows: list[dict] = []
    entry_rows: list[dict] = []
    if raw_dir.exists():
        shutil.rmtree(raw_dir)
    raw_dir.mkdir(parents=True, exist_ok=True)
    for artifact in artifacts:
        artifact_id = str(artifact.get("id"))
        name = str(artifact.get("name") or artifact_id)
        zip_path = raw_dir / f"{safe_name(name)}-{artifact_id}.zip"
        run_command(
            ["gh", "api", f"repos/{repo}/actions/artifacts/{artifact_id}/zip"],
            binary_stdout_path=zip_path,
        )
        with zipfile.ZipFile(zip_path) as zf:
            members = [info for info in zf.infolist() if not info.is_dir()]
            for info in sorted(members, key=lambda item: item.filename):
                entry_rows.append(
                    {
                        "artifact_id": artifact_id,
                        "artifact_name": name,
                        "zip_path": zip_path.as_posix(),
                        "entry_path": info.filename,
                        "entry_size": info.file_size,
                    }
                )
        zip_rows.append(
            {
                "sha256": sha256_file(zip_path),
                "artifact_id": artifact_id,
                "artifact_name": name,
                "zip_path": zip_path.as_posix(),
                "entry_count": len(members),
            }
        )
    return zip_rows, entry_rows


def should_extract(name: str, mode: str, curated_names: set[str]) -> bool:
    if mode == "none":
        return False
    if mode == "all":
        return True
    return name in curated_names or name.startswith("benchmark-results-")


def extract_artifacts(
    artifacts: Sequence[Mapping[str, object]],
    raw_dir: Path,
    extracted_dir: Path,
    mode: str,
    curated_names: set[str],
) -> list[dict]:
    inventory: list[dict] = []
    if extracted_dir.exists():
        shutil.rmtree(extracted_dir)
    extracted_dir.mkdir(parents=True, exist_ok=True)
    for artifact in artifacts:
        artifact_id = str(artifact.get("id"))
        name = str(artifact.get("name") or artifact_id)
        if not should_extract(name, mode, curated_names):
            continue
        zip_path = raw_dir / f"{safe_name(name)}-{artifact_id}.zip"
        if not zip_path.exists():
            continue
        dest = extracted_dir / name
        if dest.exists():
            dest = extracted_dir / f"{name}-{artifact_id}"
        dest.mkdir(parents=True, exist_ok=True)
        with zipfile.ZipFile(zip_path) as zf:
            zf.extractall(dest)
        for file_path in sorted(path for path in dest.rglob("*") if path.is_file()):
            inventory.append(
                {
                    "artifact_id": artifact_id,
                    "artifact_name": name,
                    "relative_path": file_path.relative_to(extracted_dir.parent).as_posix(),
                    "size_bytes": file_path.stat().st_size,
                    "sha256": sha256_file(file_path),
                }
            )
    return inventory


def iter_selected_files(output_dir: Path) -> list[Path]:
    excluded_names = {"selected-file-sha256.tsv"}
    files: list[Path] = []
    for path in output_dir.rglob("*"):
        if not path.is_file():
            continue
        if path.name in excluded_names:
            continue
        if "raw-zips" in path.relative_to(output_dir).parts:
            continue
        files.append(path)
    files.sort(key=lambda item: item.relative_to(output_dir).as_posix())
    return files


def write_selected_sha256(path: Path, output_dir: Path) -> None:
    rows = [
        {
            "sha256": sha256_file(file_path),
            "relative_path": file_path.relative_to(output_dir).as_posix(),
        }
        for file_path in iter_selected_files(output_dir)
    ]
    write_tsv(path, ["sha256", "relative_path"], rows)


def terminal_state(run_view: Mapping[str, object]) -> str:
    status = str(run_view.get("status") or "").lower()
    conclusion = str(run_view.get("conclusion") or "").lower()
    if status == "completed":
        return f"terminal_{conclusion or 'no_conclusion'}"
    return "non_terminal_snapshot"


def validation_contract_summary(output_dir: Path) -> list[str]:
    matrix_path = output_dir / "extracted" / "validation-summary" / "regular_matrix_validation.json"
    if not matrix_path.exists():
        return []
    try:
        data = json.loads(matrix_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return []
    jobs = data.get("matrix", {}).get("jobs", {}) if isinstance(data, Mapping) else {}
    tier1 = jobs.get("benchmark-tier1", {}) if isinstance(jobs, Mapping) else {}
    tier2 = jobs.get("benchmark-tier2", {}) if isinstance(jobs, Mapping) else {}
    runtime = data.get("regular_runtime_contract", {}) if isinstance(data, Mapping) else {}
    return [
        f"- Validation summary status: `{data.get('status', '')}`",
        (
            "- Matrix rows / plan entries: "
            f"Tier 1 `{tier1.get('matrix_rows', '')}` / `{tier1.get('plan_entries', '')}`, "
            f"Tier 2 `{tier2.get('matrix_rows', '')}` / `{tier2.get('plan_entries', '')}`"
        ),
        (
            "- Runtime contract: "
            f"per-invocation `{runtime.get('per_simulator_timeout_sec', '')}` seconds, "
            f"benchmark job `{runtime.get('benchmark_job_timeout_sec', '')}` seconds, "
            f"max-parallel `{runtime.get('strategy_max_parallel', '')}`"
        ),
    ]


def render_readme(
    *,
    run_id: str,
    repo: str,
    output_dir: Path,
    run_view: Mapping[str, object],
    artifacts: Sequence[Mapping[str, object]],
    job_summary: Mapping[str, object],
    extracted_count: int,
    raw_zip_count: int,
    generated_at: str,
    command: str,
) -> str:
    status = str(run_view.get("status") or "")
    conclusion = str(run_view.get("conclusion") or "")
    state = terminal_state(run_view)
    is_terminal = status.lower() == "completed"
    lines = [
        f"# MI300A Benchmark run {run_id} provenance collection",
        "",
        "This directory is a reproducible provenance collection for the maintained",
        "`MI300A Benchmark` GitHub Actions workflow. It records the run metadata, job",
        "metadata, artifact inventory, raw artifact ZIP fingerprints, extracted artifact",
        "inventory, per-job summary, and selected-file SHA256 inventory.",
        "",
        "## Source run",
        "",
        f"- Repository: `{repo}`",
        f"- Run ID: `{run_id}`",
        f"- Workflow/event: `{run_view.get('workflowName') or run_view.get('name') or ''}` / `{run_view.get('event') or ''}`",
        f"- Head branch/SHA: `{run_view.get('headBranch') or ''}` / `{run_view.get('headSha') or ''}`",
        f"- Status/conclusion at collection: `{status}` / `{conclusion}`",
        f"- Evidence state: `{state}`",
        f"- Run URL: {run_view.get('url') or ''}",
        f"- Collected at: `{generated_at}`",
        "",
    ]
    if is_terminal:
        lines.extend(
            [
                "This is a terminal run metadata snapshot. Its conclusion must still be read",
                "literally: `success` is clean workflow success/pass, `failure` is terminal",
                "failing evidence, `cancelled` is terminal cancelled evidence, and an empty",
                "or missing conclusion is a terminal no-result rather than a pass. A terminal",
                "failing, cancelled, or no-result run is not a clean finishability pass.",
                "",
            ]
        )
    else:
        lines.extend(
            [
                "## Pending/non-terminal state",
                "",
                "This is **not terminal evidence**. The workflow had not reached",
                "`status=completed` at collection time, so no clean pass, no-result, failure,",
                "cancelled, or complete finishability conclusion is claimed from this snapshot.",
                "",
                "Pending state recorded here:",
                f"- Run status is `{status}` and conclusion is `{conclusion}`.",
                f"- Job status counts: `{job_summary.get('status_counts', {})}`.",
                f"- Job conclusion counts: `{job_summary.get('conclusion_counts', {})}`.",
                "- Artifacts are provisional and may increase until the run is terminal.",
                "- Re-run the collector after the run reaches `completed` before deriving or",
                "  claiming terminal finishability evidence.",
                "",
            ]
        )
    lines.extend(
        [
            "## Inventory files",
            "",
            "- `run-view.json`: raw `gh run view` metadata, annotated with collection state.",
            "- `jobs.json`: raw `gh run view --json jobs` metadata.",
            f"- `jobs-summary.tsv`: `{job_summary.get('total_jobs', 0)}` per-job summary rows.",
            f"- `artifacts.tsv`: `{len(artifacts)}` Actions artifact inventory rows.",
            f"- `raw-zip-sha256.tsv`: `{raw_zip_count}` raw artifact ZIP SHA256 rows.",
            "- `raw-zip-entry-counts.tsv`: raw ZIP member inventory/count rows.",
            f"- `artifact-file-inventory.tsv`: `{extracted_count}` extracted artifact file rows.",
            "- `selected-file-sha256.tsv`: SHA256 inventory for tracked curated files in this directory.",
            "",
        ]
    )
    contract_lines = validation_contract_summary(output_dir)
    if contract_lines:
        lines.extend(["## Maintained workflow contract snapshot", "", *contract_lines, ""])
    lines.extend(
        [
            "## Reproduction command",
            "",
            "Run from the repository root with GitHub CLI authentication:",
            "",
            "```bash",
            command,
            "```",
            "",
            "Raw ZIP payloads are written under `raw-zips/` for hashing/extraction. They",
            "are intentionally excluded from the selected-file SHA256 inventory; retain or",
            "delete them according to local storage policy after committing the TSV",
            "inventories and curated extracted files needed for review.",
            "",
        ]
    )
    return "\n".join(lines)


def parse_args(argv: Iterable[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--repo", default="sarchlab/mgpusim-dev")
    parser.add_argument("--output-dir", default=None)
    parser.add_argument(
        "--extract",
        choices=("all", "curated", "none"),
        default="all",
        help="which downloaded artifact ZIPs to extract for curated inventory",
    )
    parser.add_argument(
        "--curated-artifact",
        action="append",
        default=[],
        help="additional exact artifact name to extract when --extract=curated",
    )
    parser.add_argument("--skip-raw-zips", action="store_true", help="skip raw ZIP download/hash")
    return parser.parse_args(list(argv) if argv is not None else None)


def main(argv: Iterable[str] | None = None) -> int:
    args = parse_args(argv)
    output_dir = Path(args.output_dir or f"benchmark-comparison/selected-run-{args.run_id}")
    output_dir.mkdir(parents=True, exist_ok=True)

    try:
        generated_at = utc_now()
        run_view = collect_run_view(args.repo, args.run_id)
        jobs = collect_jobs(args.repo, args.run_id)
        artifacts = collect_artifacts(args.repo, args.run_id)
        run_view["provenance_collection"] = {
            "schema_name": "mgpusim.mi300a_benchmark_run_provenance_collection",
            "collected_at_utc": generated_at,
            "non_terminal_snapshot": str(run_view.get("status") or "").lower() != "completed",
            "artifact_count_at_collection": len(artifacts),
        }
        json_dump(output_dir / "run-view.json", run_view)
        json_dump(output_dir / "jobs.json", jobs)
        write_artifacts_tsv(output_dir / "artifacts.tsv", artifacts)

        job_rows, job_summary = summarize_jobs(jobs)
        write_tsv(
            output_dir / "jobs-summary.tsv",
            [
                "job_id",
                "name",
                "benchmark",
                "status",
                "conclusion",
                "started_at",
                "completed_at",
                "duration_sec",
                "run_step_status",
                "run_step_conclusion",
                "url",
            ],
            job_rows,
        )
        json_dump(output_dir / "jobs-summary.json", job_summary)

        zip_rows: list[dict] = []
        entry_rows: list[dict] = []
        extracted_rows: list[dict] = []
        raw_dir = output_dir / "raw-zips"
        if not args.skip_raw_zips:
            zip_rows, entry_rows = download_raw_zips(args.repo, artifacts, raw_dir)
            curated = set(DEFAULT_CURATED_ARTIFACTS) | set(args.curated_artifact)
            extracted_rows = extract_artifacts(
                artifacts,
                raw_dir,
                output_dir / "extracted",
                args.extract,
                curated,
            )
        write_tsv(
            output_dir / "raw-zip-sha256.tsv",
            ["sha256", "artifact_id", "artifact_name", "zip_path", "entry_count"],
            zip_rows,
        )
        write_tsv(
            output_dir / "raw-zip-entry-counts.tsv",
            ["artifact_id", "artifact_name", "zip_path", "entry_path", "entry_size"],
            entry_rows,
        )
        write_tsv(
            output_dir / "artifact-file-inventory.tsv",
            ["artifact_id", "artifact_name", "relative_path", "size_bytes", "sha256"],
            extracted_rows,
        )

        command = " ".join(sys.argv if sys.argv else ["scripts/collect_mi300a_benchmark_run_provenance.py"])
        readme = render_readme(
            run_id=args.run_id,
            repo=args.repo,
            output_dir=output_dir,
            run_view=run_view,
            artifacts=artifacts,
            job_summary=job_summary,
            extracted_count=len(extracted_rows),
            raw_zip_count=len(zip_rows),
            generated_at=generated_at,
            command=command,
        )
        (output_dir / "README.md").write_text(readme, encoding="utf-8")
        write_selected_sha256(output_dir / "selected-file-sha256.tsv", output_dir)
    except CollectionError as exc:
        print(f"FAIL: MI300A Benchmark provenance collection failed: {exc}", file=sys.stderr)
        return 2

    state = terminal_state(run_view)
    print(
        "PASS: MI300A Benchmark provenance collection wrote "
        f"{output_dir} ({state}, artifacts={len(artifacts)}, raw_zips={len(zip_rows)})"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
