#!/usr/bin/env python3
"""Line-of-Code Reuse Rate for the BeyondV NVIDIA simulator.

Implements the "Line-of-Code Reuse Rate" metric from the paper's Section
V-B ("Code Reuse Validation"). Every non-test .go line reachable from the
NVIDIA simulator is put into exactly one of three buckets:

  (2) unchanged - the akita toolkit code the simulator depends on, reused
                   verbatim (MODE below controls how "the akita code" is
                   scoped: the whole module, or only the packages actually
                   imported).
  (3) adapted    - core logic in nvidia/sm and nvidia/smsp, adapted from
                   AMD's ShaderArray/ComputeUnit (SA -> SM, CU -> SMSP),
                   excluding the glue code that wires those components into
                   the rest of the system (see GLUE_FILENAMES).
  (4) new        - everything else under the code folders below, plus the
                   sm/smsp glue code excluded from bucket (3).

(1) total = (2) + (3) + (4).

Usage (run from anywhere inside the repo, on a checkout where the nvidia/
code folders exist, e.g. the nvidia_instruction branch):

    python3 scripts/code_reuse/count_loc.py
    python3 scripts/code_reuse/count_loc.py --mode used_akita
    python3 scripts/code_reuse/count_loc.py --repo-root /path/to/mgpusim

Requires the `go` toolchain on PATH: it is used to resolve where the akita
module lives on disk (both modes) and which akita packages the nvidia/ code
actually imports (used_akita mode only).

Blank/whitespace-only lines are excluded from every count; everything else
(including comments) counts as a line, per physical-line LOC convention.
"""

import argparse
import csv
import json
import subprocess
import sys
from pathlib import Path

# ----------------------------------------------------------------------------
# Configuration -- edit these to change what gets counted / how it's classified
# ----------------------------------------------------------------------------

# "all_akita":  every .go file in the entire akita module counts as reused/
#               unchanged code, regardless of whether the nvidia/ simulator
#               actually uses it.
# "used_akita": only the akita packages actually imported (directly or
#               transitively) by the nvidia/ code folders below count as
#               reused/unchanged. Harder to justify precisely (it's still a
#               whole-package granularity, not per-symbol), but it excludes
#               akita packages the NVIDIA simulator never touches.
MODE = "used_akita"  # "all_akita" or "used_akita"

# Set to False to also count *_test.go files in every bucket below.
EXCLUDE_TEST_FILES = True

# The nvidia simulator's code, per the paper's Section V-B scope. Only these
# folders count toward the simulator's own line totals (buckets 3 and 4).
CODE_FOLDERS = [
    "nvidia/driver",
    "nvidia/gpu",
    "nvidia/message",
    "nvidia/platform",
    "nvidia/runner",
    "nvidia/sm",
    "nvidia/smsp",
    "nvidia/trace",
]

# Folders whose *core* logic (i.e. every file except GLUE_FILENAMES) is
# treated as "adapted" (bucket 3) rather than "new" (bucket 4), because it
# mirrors an AMD structural counterpart: AMD's ShaderArray -> NVIDIA's SM
# (nvidia/sm), and AMD's ComputeUnit -> NVIDIA's SMSP (nvidia/smsp).
ADAPTED_FOLDERS = [
    "nvidia/sm",
    "nvidia/smsp",
]

# Filenames within ADAPTED_FOLDERS that are pure "connect this component to
# the rest of the system" glue (component construction/port wiring) rather
# than adapted architectural logic, so they count as "new" (bucket 4) even
# though they live under an ADAPTED_FOLDERS directory.
GLUE_FILENAMES = {
    "builder.go",
    "doc.go",
}

# Manual overrides, keyed by path relative to the repo root (e.g.
# "nvidia/sm/smcontroller.go"). Use these to force a specific file into a
# bucket regardless of the folder/filename rule above.
FORCE_ADAPTED = set()
FORCE_NEW = set()

# Go import-path prefix used to recognize akita packages among dependencies.
# Matches any major version (v4, v5, ...) via `go list -m all`.
AKITA_IMPORT_PREFIX = "github.com/sarchlab/akita/"

OUTPUT_DIR = Path(__file__).resolve().parent / "output"

# ----------------------------------------------------------------------------


def count_nonblank_lines(path: Path) -> int:
    try:
        text = path.read_text(encoding="utf-8", errors="replace")
    except OSError as exc:
        print(f"warning: could not read {path}: {exc}", file=sys.stderr)
        return 0
    return sum(1 for line in text.splitlines() if line.strip())


def iter_go_files(root: Path, exclude_tests: bool, recursive: bool):
    pattern = "*.go"
    paths = root.rglob(pattern) if recursive else root.glob(pattern)
    for path in sorted(paths):
        if exclude_tests and path.name.endswith("_test.go"):
            continue
        yield path


def run_go(args, cwd: Path) -> str:
    result = subprocess.run(
        ["go", *args], cwd=cwd, capture_output=True, text=True, check=False
    )
    if result.returncode != 0:
        raise RuntimeError(f"`go {' '.join(args)}` failed in {cwd}:\n{result.stderr}")
    return result.stdout


def find_repo_root(start: Path) -> Path:
    try:
        out = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            cwd=start, capture_output=True, text=True, check=True,
        )
        return Path(out.stdout.strip())
    except (subprocess.CalledProcessError, FileNotFoundError):
        return start


def classify_nvidia_file(rel_path: str) -> str:
    if rel_path in FORCE_ADAPTED:
        return "adapted"
    if rel_path in FORCE_NEW:
        return "new"
    p = Path(rel_path)
    folder = "/".join(p.parts[:2])  # e.g. "nvidia/sm"
    if folder in ADAPTED_FOLDERS and p.name not in GLUE_FILENAMES:
        return "adapted"
    return "new"


def count_nvidia_code(repo_root: Path):
    """Returns (adapted_lines, new_lines, per_file_records)."""
    adapted_total = 0
    new_total = 0
    records = []
    for folder in CODE_FOLDERS:
        folder_path = repo_root / folder
        if not folder_path.is_dir():
            print(f"warning: {folder} not found under {repo_root} -- skipping", file=sys.stderr)
            continue
        for f in iter_go_files(folder_path, EXCLUDE_TEST_FILES, recursive=True):
            rel = str(f.relative_to(repo_root))
            n = count_nonblank_lines(f)
            bucket = classify_nvidia_file(rel)
            if bucket == "adapted":
                adapted_total += n
            else:
                new_total += n
            records.append({"path": rel, "lines": n, "bucket": bucket})
    return adapted_total, new_total, records


def resolve_akita_module(repo_root: Path):
    """Returns (import_path, module_dir) for the akita dependency."""
    out = run_go(["list", "-m", "-f", "{{.Path}} {{.Dir}}", "all"], cwd=repo_root)
    for line in out.splitlines():
        if line.startswith(AKITA_IMPORT_PREFIX):
            path, _, dir_ = line.partition(" ")
            return path, Path(dir_)
    raise RuntimeError(
        f"could not find a module under {AKITA_IMPORT_PREFIX!r} via `go list -m all` "
        f"in {repo_root} -- is `go` on PATH and is this repo's go.mod resolvable?"
    )


def count_all_akita(module_dir: Path):
    total = 0
    records = []
    for f in iter_go_files(module_dir, EXCLUDE_TEST_FILES, recursive=True):
        n = count_nonblank_lines(f)
        total += n
        records.append({"path": str(f.relative_to(module_dir)), "package": "", "lines": n})
    return total, records


def count_used_akita(repo_root: Path, akita_import_path: str):
    """Only the akita packages actually imported (directly/transitively) by
    the nvidia/ code folders, via `go list -deps`."""
    deps_args = ["-deps"]
    if not EXCLUDE_TEST_FILES:
        deps_args.append("-test")
    pkg_args = [f"./{folder}/..." for folder in CODE_FOLDERS]
    out = run_go(["list", *deps_args, *pkg_args], cwd=repo_root)
    deps = sorted({line.strip() for line in out.splitlines() if line.strip()})
    akita_pkgs = [d for d in deps if d.startswith(akita_import_path)]
    if not akita_pkgs:
        raise RuntimeError(
            "no akita packages found among nvidia/ dependencies -- "
            "check CODE_FOLDERS and that `go build ./nvidia/...` succeeds"
        )

    dir_out = run_go(["list", "-f", "{{.ImportPath}} {{.Dir}}", *akita_pkgs], cwd=repo_root)
    total = 0
    records = []
    for line in dir_out.splitlines():
        import_path, _, dir_ = line.partition(" ")
        d = Path(dir_)
        for f in iter_go_files(d, EXCLUDE_TEST_FILES, recursive=False):
            n = count_nonblank_lines(f)
            total += n
            records.append({"path": str(f), "package": import_path, "lines": n})
    return total, records, akita_pkgs


def pct(part: int, total: int) -> float:
    return 100.0 * part / total if total else 0.0


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--repo-root", type=Path, default=None, help="Repo root to analyze (default: auto-detect from cwd)")
    parser.add_argument("--mode", choices=["all_akita", "used_akita"], default=None, help=f"Override MODE (default: {MODE!r} from the script)")
    parser.add_argument("--verbose", action="store_true", help="Print the full per-file breakdown to stdout")
    args = parser.parse_args()

    mode = args.mode or MODE
    repo_root = args.repo_root or find_repo_root(Path.cwd())
    repo_root = repo_root.resolve()

    print(f"repo root : {repo_root}")
    print(f"mode      : {mode}")
    print(f"exclude _test.go files : {EXCLUDE_TEST_FILES}")
    print()

    adapted_lines, new_lines, nvidia_records = count_nvidia_code(repo_root)

    akita_import_path, akita_module_dir = resolve_akita_module(repo_root)
    print(f"akita module: {akita_import_path} @ {akita_module_dir}")

    akita_pkgs_used = None
    if mode == "all_akita":
        unchanged_lines, akita_records = count_all_akita(akita_module_dir)
    else:
        unchanged_lines, akita_records, akita_pkgs_used = count_used_akita(repo_root, akita_import_path)
        print(f"akita packages actually imported by nvidia/ code: {len(akita_pkgs_used)}")

    total_lines = unchanged_lines + adapted_lines + new_lines

    print()
    print("=" * 72)
    print("Line-of-Code Reuse Rate (paper Section V-B)")
    print("=" * 72)
    print(f"(1) total simulator lines            : {total_lines:>8,}")
    print(f"(2) unchanged (akita, {mode:>10}) : {unchanged_lines:>8,}  ({pct(unchanged_lines, total_lines):5.1f}%)")
    print(f"(3) adapted (nvidia/sm, nvidia/smsp)  : {adapted_lines:>8,}  ({pct(adapted_lines, total_lines):5.1f}%)")
    print(f"(4) new (from scratch)                : {new_lines:>8,}  ({pct(new_lines, total_lines):5.1f}%)")
    print("-" * 72)
    reuse_lines = unchanged_lines + adapted_lines
    print(f"combined reuse rate (2)+(3)           : {reuse_lines:>8,}  ({pct(reuse_lines, total_lines):5.1f}%)")
    print("=" * 72)

    if args.verbose:
        print("\n-- nvidia/ files --")
        for r in nvidia_records:
            print(f"  [{r['bucket']:>7}] {r['lines']:>6,}  {r['path']}")
        print("\n-- akita files --")
        for r in akita_records:
            pkg = f" ({r['package']})" if r.get("package") else ""
            print(f"  {r['lines']:>6,}  {r['path']}{pkg}")

    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    summary = {
        "repo_root": str(repo_root),
        "mode": mode,
        "exclude_test_files": EXCLUDE_TEST_FILES,
        "akita_import_path": akita_import_path,
        "akita_module_dir": str(akita_module_dir),
        "akita_packages_used": akita_pkgs_used,
        "total_lines": total_lines,
        "unchanged_lines": unchanged_lines,
        "adapted_lines": adapted_lines,
        "new_lines": new_lines,
        "unchanged_pct": pct(unchanged_lines, total_lines),
        "adapted_pct": pct(adapted_lines, total_lines),
        "new_pct": pct(new_lines, total_lines),
        "combined_reuse_lines": reuse_lines,
        "combined_reuse_pct": pct(reuse_lines, total_lines),
    }
    summary_path = OUTPUT_DIR / f"summary_{mode}.json"
    summary_path.write_text(json.dumps(summary, indent=2) + "\n")

    files_path = OUTPUT_DIR / f"files_{mode}.csv"
    with files_path.open("w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["source", "bucket", "package", "lines", "path"])
        for r in nvidia_records:
            writer.writerow(["nvidia", r["bucket"], "", r["lines"], r["path"]])
        for r in akita_records:
            writer.writerow(["akita", "unchanged", r.get("package", ""), r["lines"], r["path"]])

    print(f"\nwrote {summary_path.relative_to(repo_root) if repo_root in summary_path.parents else summary_path}")
    print(f"wrote {files_path.relative_to(repo_root) if repo_root in files_path.parents else files_path}")


if __name__ == "__main__":
    main()
