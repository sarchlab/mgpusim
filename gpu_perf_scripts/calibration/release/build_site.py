"""Build a release index and bounded plots using the standard Calibration Report."""

import argparse
import json
import re
import shutil
from collections import defaultdict
from html import escape
from pathlib import Path

from workflow import atomic_json, read_reference, report

LAUNCHES_PER_PAGE = 48


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


def render_site(data, template, destination, *, launch_counts=None):
    destination.mkdir(parents=True, exist_ok=True)
    for name in ("report.css", "summary.js", "report.js"):
        shutil.copy2(template / name, destination / name)
    html = (template / "index.html").read_text()
    grouped = defaultdict(list)
    for point in data["data"]:
        grouped[point["benchmark"]].append(point)

    links = []
    for benchmark in launch_counts if launch_counts is not None else grouped:
        points = grouped[benchmark]
        if not re.fullmatch(r"[a-z][a-z0-9_]*", benchmark):
            raise ValueError(f"Invalid benchmark name: {benchmark}")
        benchmark_dir = destination / benchmark
        benchmark_dir.mkdir(exist_ok=True)
        title = benchmark.replace("_", " ")
        links.append((f"{benchmark}/", title))
        pages = defaultdict(list)
        for point in points:
            ordinal = int(point["non_scaling_params"]["launch"].split(" ", 1)[0][1:])
            pages[(ordinal - 1) // LAUNCHES_PER_PAGE].append(point)
        if launch_counts is not None:
            page_count = (
                launch_counts[benchmark] + LAUNCHES_PER_PAGE - 1
            ) // LAUNCHES_PER_PAGE
            for page in range(page_count):
                pages.setdefault(page, [])
        page_links = []
        for page, items in sorted(pages.items()):
            paginated = len(pages) > 1
            page_dir = (
                benchmark_dir / f"part-{page + 1}" if paginated else benchmark_dir
            )
            page_dir.mkdir(exist_ok=True)
            relative = "../../" if paginated else "../"
            back = (
                f'<p><a href="../">All launches for {escape(title)}</a></p>'
                if paginated
                else '<p><a href="../">All benchmarks</a></p>'
            )
            screen = html.replace(
                '    <header class="report-header">',
                f'    {back}\n    <header class="report-header">',
            )
            for asset in ("report.css", "summary.js", "report.js"):
                screen = screen.replace(f'"./{asset}"', f'"{relative}{asset}"')
            (page_dir / "index.html").write_text(screen)
            first = page * LAUNCHES_PER_PAGE + 1
            if launch_counts is None:
                last = max(
                    int(item["non_scaling_params"]["launch"].split(" ", 1)[0][1:])
                    for item in items
                )
            else:
                last = min(first + LAUNCHES_PER_PAGE - 1, launch_counts[benchmark])
            page_title = f"{title} · launches {first}–{last}" if paginated else title
            atomic_json(
                page_dir / "data.json",
                {
                    "report": {**data["report"], "title": page_title},
                    "benchmarks": {benchmark: data["benchmarks"][benchmark]}
                    if benchmark in data["benchmarks"]
                    else {},
                    "data": items,
                },
            )
            page_links.append((f"part-{page + 1}/", f"Launches {first}–{last}"))
        if len(pages) > 1:
            link_list(
                benchmark_dir / "index.html",
                title,
                "Each page compares a range of kernel launch positions.",
                page_links,
                "../report.css",
                parent="../",
            )
    render_overview(data, html, destination, links)


def render_overview(data, html, destination, links):
    # Keep the landing page responsive even when a benchmark has thousands
    # of launches. The linked benchmark pages retain every result.
    charts = defaultdict(list)
    for point in data["data"]:
        fixed = tuple(sorted(point["non_scaling_params"].items()))
        charts[(point["benchmark"], fixed)].append(point)
    shown = list(charts.values())[:LAUNCHES_PER_PAGE]
    points = [point for chart in shown for point in chart]
    names = {point["benchmark"] for point in points}
    header = dict(data["report"])
    if len(shown) < len(charts):
        header["description"] += (
            f" The overview uses {len(shown):,} of {len(charts):,} available charts. "
            "Open a benchmark below for its complete results."
        )
    atomic_json(
        destination / "data.json",
        {
            **data,
            "report": header,
            "benchmarks": {
                name: meta for name, meta in data["benchmarks"].items() if name in names
            },
            "data": points,
        },
    )
    items = "".join(f'<a href="{href}">{escape(label)}</a>' for href, label in links)
    navigation = (
        f'<nav class="benchmark-links" aria-label="Benchmark reports">{items}</nav>'
    )
    style = """<style>
.benchmark-links { display: flex; flex-wrap: wrap; gap: .65rem 1.2rem; margin-block: 1rem; }
.benchmark-links a { overflow-wrap: anywhere; }
</style>"""
    html = html.replace("</head>", style + "\n</head>")
    html = html.replace(
        '<section class="charts"', navigation + '\n    <section class="charts"'
    )
    (destination / "index.html").write_text(html)


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
    )


if __name__ == "__main__":
    main()
