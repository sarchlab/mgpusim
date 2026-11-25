"""Build a release index and bounded plots using the standard Calibration Report."""

import argparse
import json
import re
import shutil
from collections import defaultdict
from html import escape
from pathlib import Path

from workflow import atomic_json, base_kernel_name, read_reference, report

LAUNCHES_PER_DETAIL_PAGE = 48


def link_list(path, title, intro, links, css, parent=None):
    back = f'<p><a href="{parent}">All benchmarks</a></p>' if parent else ""
    items = "".join(
        f'<li><a href="{href}">{escape(label)}</a></li>' for href, label in links
    )
    path.write_text(f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>{escape(title)}</title>
<link rel="stylesheet" href="{css}">
</head>
<body><main><header class="report-header"><h1>{escape(title)}</h1>
<p class="report-description">{escape(intro)}</p>{back}</header>
<nav aria-label="Calibration reports"><ul>{items}</ul></nav>
</main></body></html>
""")


def report_layout(reference):
    launch_counts = {}
    charts = set()
    for series in reference["series"]:
        if series.get("selection_excluded"):
            continue
        benchmark = series["benchmark"]
        cases = series["cases"]
        launch_counts[benchmark] = max(
            launch_counts.get(benchmark, 0),
            max(len(case["launches"]) for case in cases),
        )
        fixed = tuple(sorted(series["parameters"].items()))
        for case in cases:
            if len(charts) > LAUNCHES_PER_DETAIL_PAGE:
                break
            charts.update(
                (benchmark, fixed, ordinal, base_kernel_name(launch[0]))
                for ordinal, launch in enumerate(case["launches"], start=1)
            )
    return launch_counts, len(charts) > LAUNCHES_PER_DETAIL_PAGE


def write_report(data, template, destination):
    shutil.copytree(template, destination, dirs_exist_ok=True)
    atomic_json(destination / "data.json", data)


def render_site(data, template, destination, *, layout):
    launch_counts, paginated = layout
    if not paginated:
        write_report(data, template, destination)
        return

    write_report(data, template, destination / "overview")
    grouped = defaultdict(list)
    for point in data["data"]:
        grouped[point["benchmark"]].append(point)

    links = [("overview/", "Accuracy overview")]
    for benchmark, count in launch_counts.items():
        if not re.fullmatch(r"[a-z][a-z0-9_]*", benchmark):
            raise ValueError(f"Invalid benchmark name: {benchmark}")
        benchmark_dir = destination / benchmark
        title = benchmark.replace("_", " ")
        links.append((f"{benchmark}/", title))

        pages = defaultdict(list)
        for point in grouped[benchmark]:
            ordinal = int(point["non_scaling_params"]["launch"].split(" ", 1)[0][1:])
            pages[(ordinal - 1) // LAUNCHES_PER_DETAIL_PAGE].append(point)

        page_count = (count + LAUNCHES_PER_DETAIL_PAGE - 1) // LAUNCHES_PER_DETAIL_PAGE
        page_links = []
        for page in range(page_count):
            split = page_count > 1
            page_dir = benchmark_dir / f"part-{page + 1}" if split else benchmark_dir
            first = page * LAUNCHES_PER_DETAIL_PAGE + 1
            last = min(first + LAUNCHES_PER_DETAIL_PAGE - 1, count)
            page_label = f"Page {page + 1} of {page_count} · launches {first}–{last}"
            page_title = f"{title} · {page_label}" if split else title
            write_report(
                {
                    "report": {**data["report"], "title": page_title},
                    "benchmarks": {benchmark: data["benchmarks"][benchmark]}
                    if benchmark in data["benchmarks"]
                    else {},
                    "data": pages[page],
                },
                template,
                page_dir,
            )
            page_links.append((f"part-{page + 1}/", page_label))
        if page_count > 1:
            link_list(
                benchmark_dir / "index.html",
                title,
                "Each page compares a range of kernel launch positions.",
                page_links,
                "../overview/report.css",
                parent="../",
            )
    link_list(
        destination / "index.html",
        data["report"]["title"],
        "The overview summarizes all paired dispatches. Benchmark reports contain every launch.",
        links,
        "./overview/report.css",
    )


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("artifacts", type=Path)
    p.add_argument("template", type=Path)
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
    render_site(
        report(reference, results, args.version, args.revision),
        args.template,
        args.output,
        layout=report_layout(reference),
    )


if __name__ == "__main__":
    main()
