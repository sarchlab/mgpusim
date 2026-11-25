"""Watch a GitHub Actions calibration run and serve its completed series as static JSON."""

import argparse
import io
import json
import re
import shutil
import subprocess
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from zipfile import ZipFile

from workflow import atomic_json, matrix_rows, read_reference, report


def github(*args):
    return subprocess.check_output(["gh", "api", *args], timeout=60)


def artifact_json(repo, artifact_id, filename):
    archive = github(f"repos/{repo}/actions/artifacts/{artifact_id}/zip")
    with ZipFile(io.BytesIO(archive)) as bundle:
        return json.loads(bundle.read(filename))


def artifact_ids(endpoint):
    pages = json.loads(
        github(f"{endpoint}/artifacts?per_page=100", "--paginate", "--slurp")
    )
    latest = {}
    for page in pages:
        for item in page["artifacts"]:
            name = item["name"]
            if item["expired"] or not (
                name == "calibration-plan" or re.fullmatch(r"series-\d+", name)
            ):
                continue
            latest[name] = max(item["id"], latest.get(name, 0))
    return latest


def select_reference(reference, plan):
    expected = {row["id"]: row for row in matrix_rows(reference, None)}
    selection = {row["id"]: row for row in plan["series"]}
    if (
        not selection
        or len(selection) != len(plan["series"])
        or any(expected.get(i) != row for i, row in selection.items())
    ):
        raise ValueError("Run plan does not match this calibration reference")

    for i, series in enumerate(reference["series"]):
        series["selection_excluded"] = i not in selection
    return selection.keys()


def render_site(data, template, output):
    shutil.copytree(template, output, dirs_exist_ok=True)
    atomic_json(output / "data.json", data)


def watch(reference, template, output, repo, run_id, interval, destination):
    endpoint = f"repos/{repo}/actions/runs/{run_id}"
    plan_id = None
    plan = None
    last_published = None
    cached = {}
    with ThreadPoolExecutor(max_workers=6) as pool:
        while True:
            try:
                run = json.loads(github(endpoint))
                artifacts = artifact_ids(endpoint)
                live = run["status"] != "completed"
                current_plan_id = artifacts.pop("calibration-plan", None)
                if current_plan_id is None and not live:
                    raise ValueError("Completed run has no calibration plan artifact")
                if current_plan_id is not None and current_plan_id != plan_id:
                    plan = artifact_json(repo, current_plan_id, "calibration-plan.json")
                    if plan["revision"] != run["head_sha"]:
                        raise ValueError(
                            "Calibration plan revision does not match the run"
                        )
                    selected = select_reference(reference, plan)
                    plan_id = current_plan_id

                if plan is not None:
                    results = {
                        int(name.removeprefix("series-")): artifact_id
                        for name, artifact_id in artifacts.items()
                        if int(name.removeprefix("series-")) in selected
                    }
                    signature = (plan_id, live, tuple(sorted(results.items())))
                    if signature != last_published:
                        missing = sorted(set(results.values()) - cached.keys())
                        cached.update(
                            zip(
                                missing,
                                pool.map(
                                    lambda item: artifact_json(
                                        repo, item, "results.json"
                                    ),
                                    missing,
                                ),
                                strict=True,
                            )
                        )
                        data = report(
                            reference,
                            {i: cached[item] for i, item in results.items()},
                            plan["version"],
                            plan["revision"],
                            live=live,
                        )
                        if live:
                            data["report"]["description"] += (
                                f" Results published for {len(results)} of {len(selected)} series. "
                                "Configurations in active series are pending until their artifacts arrive."
                            )
                        render_site(data, template, output)
                        if destination:
                            subprocess.run(
                                [
                                    "rsync",
                                    "-rz",
                                    "--checksum",
                                    "--delay-updates",
                                    "--",
                                    f"{output}/",
                                    destination,
                                ],
                                timeout=120,
                                check=True,
                            )
                        print(data["progress"], flush=True)
                        last_published = signature
                    if not live:
                        return
            except subprocess.SubprocessError as error:
                print(f"Temporary preview error: {error}. Retrying.", flush=True)
            time.sleep(interval)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("run_id", type=int)
    parser.add_argument(
        "template", type=Path, help="Calibration Report's site directory"
    )
    parser.add_argument(
        "output", type=Path, help="dedicated directory for this run's preview"
    )
    parser.add_argument("--repo", default="sarchlab/mgpusim")
    parser.add_argument(
        "--reference",
        type=Path,
        default=Path(__file__).with_name("mi300x-v1.1.json.gz"),
    )
    parser.add_argument("--interval", type=float, default=60)
    parser.add_argument(
        "--publish", help="optional rsync destination, e.g. marin-5:~/preview/123/"
    )
    args = parser.parse_args()
    if args.interval <= 0:
        parser.error("--interval must be positive")
    watch(
        read_reference(args.reference),
        args.template,
        args.output,
        args.repo,
        args.run_id,
        args.interval,
        args.publish,
    )


if __name__ == "__main__":
    main()
