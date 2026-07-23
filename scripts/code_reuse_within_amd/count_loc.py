#!/usr/bin/env python3
"""Line-of-Code Reuse Rate for AMD's own codebase evolution: main vs v4.

A same-vendor counterpart to scripts/code_reuse/count_loc.py (which
measures how much of the NVIDIA simulator is unchanged/adapted/new
relative to AMD's own toolkit). This script asks the analogous question
about AMD's own amd/ folder across two branches of the same repo: how much
of `main`'s amd/ (mgpusim's current AMD target, MI300X) is unchanged from
`v4`'s amd/ (mgpusim's original AMD target, R9 Nano) versus newly written,
with the same "(1) unchanged also includes the toolkit code actually used"
convention.

Scope is deliberately narrow: only AMD_CODE_SUBDIRS below (driver, bitops,
emu, insts, kernels, protocol, sampling, server, timing) -- not amd/arch,
amd/samples, amd/benchmarks, amd/tests. Test files (*_test.go), doc.go
files (package-level doc stubs, low information content), and non-.go
files are all excluded from every count.

Categories, mirroring count_loc.py's structure:
  (2) unchanged - amd/ lines on MAIN_REF that are textually identical to a
                   line already present in the same file on OLD_REF (a
                   difflib-based per-file line match; a file with no
                   OLD_REF counterpart contributes 0 unchanged lines),
                   PLUS the akita packages actually imported by MAIN_REF's
                   amd/ code (used_akita methodology -- same "only
                   packages actually reachable, whole-package granularity"
                   caveat as count_loc.py).
  (3) adapted    - always 0: "adapted" captures a different vendor's
                   structure being retrofitted (SA -> SM, CU -> SMSP for
                   the NVIDIA side); there is no such concept for AMD's
                   own code evolving against itself.
  (4) new        - amd/ lines on MAIN_REF with no line-level match in the
                   corresponding OLD_REF file, or in a file that didn't
                   exist on OLD_REF at all.
(1) total = (2) + (3) + (4).

Usage:

    python3 scripts/code_reuse_within_amd/count_loc.py
    python3 scripts/code_reuse_within_amd/count_loc.py --repo-root /path/to/mgpusim

Requires the `go` toolchain on PATH and network access: MAIN_REF's go.mod
(mgpusim/v5, requiring akita/v5) has a `replace ... => ../../../../akita`
directive pointing at a local checkout that does not exist in an isolated
materialized copy of MAIN_REF, so this script drops that replace line and
lets `go mod download` fetch akita/v5 from the module proxy instead --
this is the one network dependency here (OLD_REF's akita/v4 dependency is
already resolvable from the local module cache with no such issue).

Blank/whitespace-only lines AND comment-only lines are excluded from every
count; a line with trailing code plus a comment still counts (only the
comment text is stripped, via strip_comments.go / go/scanner, which -
unlike a naive regex - correctly leaves "//" or "/*" inside string/rune
literals alone).
"""

import argparse
import csv
import difflib
import json
import re
import subprocess
import sys
from pathlib import Path

# ----------------------------------------------------------------------------
# Configuration -- edit these to change what gets counted / how it's classified
# ----------------------------------------------------------------------------

# The "new" reference (current AMD target, MI300X) and the "old" reference
# (original AMD target, R9 Nano) -- both refs of *this* repo's own git
# history (this repo IS a clone of sarchlab/mgpusim), so the amd/ content
# itself needs no network access; only MAIN_REF's akita/v5 dependency does
# (see module docstring).
MAIN_REF = "origin/main"
OLD_REF = "origin/v4"

AMD_ROOT = "amd"

# Only these amd/ subdirectories count toward any bucket below (also used
# to scope the `go list -deps` used_akita computation). Everything else
# under amd/ (arch, samples, benchmarks, tests, ...) is out of scope.
AMD_CODE_SUBDIRS = [
    "driver",
    "bitops",
    "emu",
    "insts",
    "kernels",
    "protocol",
    "sampling",
    "server",
    "timing",
]

# Set to False to also count *_test.go files in every bucket below.
EXCLUDE_TEST_FILES = True

# Set to False to count doc.go files too.
EXCLUDE_DOC_GO = True

# Go import-path prefix used to recognize akita packages among dependencies.
# Matches any major version (v4, v5, ...) via `go list -m all`.
AKITA_IMPORT_PREFIX = "github.com/sarchlab/akita/"

SCRIPT_DIR = Path(__file__).resolve().parent
CACHE_DIR = SCRIPT_DIR / "cache"
OUTPUT_DIR = SCRIPT_DIR / "output"

# ----------------------------------------------------------------------------


def nonblank_lines_from_text(text: str):
    return [line for line in text.splitlines() if line.strip()]


def build_strip_comments_binary() -> Path:
    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    binary = CACHE_DIR / "strip_comments_bin"
    if not binary.exists():
        src = SCRIPT_DIR / "strip_comments.go"
        result = subprocess.run(["go", "build", "-o", str(binary), str(src)], capture_output=True, text=True)
        if result.returncode != 0:
            raise RuntimeError(f"failed to build strip_comments.go: {result.stderr}")
    return binary


def strip_comments_batch(binary: Path, paths):
    """Returns {resolved Path: comment-stripped source text} for every
    path in `paths` (deduplicated). Comments are blanked via go/scanner
    (see strip_comments.go's docstring) -- safe against "//"/"/*" inside
    string or rune literals, unlike a regex-based stripper."""
    unique_paths = sorted({Path(p).resolve() for p in paths})
    if not unique_paths:
        return {}
    result = subprocess.run(
        [str(binary), *[str(p) for p in unique_paths]], capture_output=True, text=True
    )
    if result.returncode != 0:
        raise RuntimeError(f"strip_comments failed: {result.stderr}")
    data = json.loads(result.stdout)
    return {Path(item["path"]): item["source"] for item in data}


def _is_doc_go(path: Path) -> bool:
    return EXCLUDE_DOC_GO and path.name == "doc.go"


def iter_go_files(root: Path, exclude_tests: bool, recursive: bool = True):
    paths = root.rglob("*.go") if recursive else root.glob("*.go")
    for path in sorted(paths):
        if exclude_tests and path.name.endswith("_test.go"):
            continue
        if _is_doc_go(path):
            continue
        yield path


def iter_amd_code_files(amd_dir: Path, exclude_tests: bool):
    """Only files under one of AMD_CODE_SUBDIRS, applying the same
    test/doc.go exclusions as iter_go_files."""
    for subdir in AMD_CODE_SUBDIRS:
        d = amd_dir / subdir
        if not d.is_dir():
            print(f"warning: {amd_dir}/{subdir} not found -- skipping", file=sys.stderr)
            continue
        yield from iter_go_files(d, exclude_tests, recursive=True)


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


def pct(part: int, total: int) -> float:
    return 100.0 * part / total if total else 0.0


# ---- Corpus materialization ---------------------------------------------


def _ref_sha(repo_root: Path, ref: str) -> str:
    return subprocess.run(
        ["git", "rev-parse", ref], cwd=repo_root, capture_output=True, text=True, check=True
    ).stdout.strip()


def _git_archive_extract(repo_root: Path, sha: str, paths, dest: Path):
    dest.mkdir(parents=True, exist_ok=True)
    archive = subprocess.Popen(
        ["git", "archive", sha, "--", *paths], cwd=repo_root, stdout=subprocess.PIPE
    )
    subprocess.run(["tar", "-x", "-C", str(dest)], stdin=archive.stdout, check=True)
    archive.wait()


def materialize_amd_only(repo_root: Path, ref: str) -> Path:
    """Archives just amd/ from `ref` into CACHE_DIR/amd_only_<sha>/.
    Everything outside AMD_CODE_SUBDIRS is still present on disk (git
    archive pulls the whole amd/ tree) -- scope filtering happens later,
    at line-counting time, via iter_amd_code_files()."""
    sha = _ref_sha(repo_root, ref)
    dest = CACHE_DIR / f"amd_only_{sha[:12]}"
    if not dest.exists():
        _git_archive_extract(repo_root, sha, [AMD_ROOT], dest)
    return dest


def materialize_full_repo(repo_root: Path, ref: str) -> Path:
    """Archives the *entire* tree at `ref` into CACHE_DIR/full_<sha>/ --
    needed (not just amd/) so `go list -deps` can resolve the module:
    e.g. amd/samples/*/main.go imports amd/benchmarks/..., so the full
    tree is needed even though amd/samples and amd/benchmarks are outside
    AMD_CODE_SUBDIRS and don't contribute to any count."""
    sha = _ref_sha(repo_root, ref)
    dest = CACHE_DIR / f"full_{sha[:12]}"
    if not dest.exists():
        _git_archive_extract(repo_root, sha, ["."], dest)
    return dest


_LOCAL_REPLACE_RE = re.compile(r"^\s*replace\s+\S+\s*=>\s*(\.\.?/\S+)\s*$")


def strip_local_replace_directives(go_mod_path: Path):
    """Removes `replace X => <relative path>` lines: in an isolated
    materialized copy, any replace pointing at a relative path (as opposed
    to a real module path) refers to a sibling checkout that doesn't exist
    here, and would otherwise make the whole module unresolvable."""
    text = go_mod_path.read_text()
    kept, dropped = [], []
    for line in text.splitlines():
        if _LOCAL_REPLACE_RE.match(line):
            dropped.append(line.strip())
            continue
        kept.append(line)
    if dropped:
        go_mod_path.write_text("\n".join(kept) + "\n")
        for line in dropped:
            print(f"  dropped local replace directive: {line}", file=sys.stderr)


def ensure_module_resolvable(full_dir: Path):
    """Fixes up go.mod (see strip_local_replace_directives) and downloads
    whatever's missing from the module proxy."""
    go_mod = full_dir / "go.mod"
    strip_local_replace_directives(go_mod)
    print(f"  running `go mod download` in {full_dir} ...", file=sys.stderr)
    result = subprocess.run(["go", "mod", "download"], cwd=full_dir, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"`go mod download` failed in {full_dir}:\n{result.stderr}")


# ---- (2)/(4): unchanged vs. new, via per-file line diff -------------------


def build_rename_map(repo_root: Path, old_ref: str, new_ref: str):
    """Returns {new_relpath_under_amd: old_relpath_under_amd} for every
    amd/ file that git's content-based rename/move detection (`git diff
    -M`) can match between old_ref and new_ref -- e.g. v4's
    amd/emu/aluvop2.go became main's amd/emu/gcn3/aluvop2.go (moved into a
    new gcn3/ subpackage to make room for a sibling cdna3/ package); exact
    relative-path matching alone would wrongly call every line of the
    moved file "new". Files git diff doesn't mention at all are unchanged
    at an identical path -- those don't need an entry here, since plain
    same-path lookup already finds them.

    Only covers files git considers "changed" (added/modified/renamed) --
    truly identical files never appear in `git diff` output in the first
    place, so they're handled separately (see classify_amd_diff)."""
    old_sha = _ref_sha(repo_root, old_ref)
    new_sha = _ref_sha(repo_root, new_ref)
    # A low threshold is deliberate: the subsequent line-level diff (see
    # classify_amd_diff) does the precise unchanged/new accounting anyway,
    # so a weak-but-real rename match costs nothing (worst case it
    # contributes ~0 unchanged lines, same as finding no match at all),
    # while git's 50% default missed a real move here (v4's
    # amd/emu/aluvop2.go -> main's amd/emu/gcn3/aluvop2.go, only ~33%
    # textually similar after the surrounding akita v4->v5 migration, but
    # still the correct file to diff against).
    out = subprocess.run(
        ["git", "diff", "--name-status", "-M5%", old_sha, new_sha, "--", AMD_ROOT],
        cwd=repo_root, capture_output=True, text=True, check=True,
    ).stdout

    prefix = f"{AMD_ROOT}/"
    mapping = {}
    for line in out.splitlines():
        parts = line.split("\t")
        status = parts[0]
        if status.startswith("R") and len(parts) == 3:
            old_path, new_path = parts[1], parts[2]
            if old_path.startswith(prefix) and new_path.startswith(prefix):
                mapping[new_path[len(prefix):]] = old_path[len(prefix):]
        elif status == "M" and len(parts) == 2:
            path = parts[1]
            if path.startswith(prefix):
                mapping[path[len(prefix):]] = path[len(prefix):]
        # 'A' (added, no old counterpart) and 'D' (deleted, not part of
        # the new tree) need no entry.
    return mapping


def classify_amd_diff(old_amd_dir: Path, new_amd_dir: Path, exclude_tests: bool,
                       rename_map: dict, stripped: dict):
    """Returns (unchanged_lines, new_lines, per_file_records) for every
    .go file under new_amd_dir's AMD_CODE_SUBDIRS, classifying each of its
    non-blank, non-comment lines as unchanged (matched, via difflib, to a
    line in its counterpart file under old_amd_dir -- same relative path,
    or the rename_map's mapped path if git detected a move) or new (no
    counterpart at all, or no line-level match within one).

    `stripped` is {resolved Path: comment-stripped source}, pre-populated
    by strip_comments_batch() for every file this function will touch."""
    unchanged_total = 0
    new_total = 0
    records = []
    for f in iter_amd_code_files(new_amd_dir, exclude_tests):
        rel = str(f.relative_to(new_amd_dir))
        new_lines_list = nonblank_lines_from_text(stripped[f.resolve()])
        old_rel = rename_map.get(rel, rel)
        old_f = old_amd_dir / old_rel
        if old_f.is_file():
            old_lines_list = nonblank_lines_from_text(stripped[old_f.resolve()])
            matcher = difflib.SequenceMatcher(None, old_lines_list, new_lines_list, autojunk=False)
            unchanged_here = sum(block.size for block in matcher.get_matching_blocks())
        else:
            unchanged_here = 0
        new_here = len(new_lines_list) - unchanged_here
        unchanged_total += unchanged_here
        new_total += new_here
        records.append({
            "path": f"{AMD_ROOT}/{rel}",
            "matched_old_path": f"{AMD_ROOT}/{old_rel}" if old_f.is_file() else "",
            "unchanged_lines": unchanged_here,
            "new_lines": new_here,
            "existed_in_old": old_f.is_file(),
        })
    return unchanged_total, new_total, records


# ---- (2)'s toolkit component: akita actually used by MAIN_REF's amd/ -----


def resolve_akita_module(full_dir: Path):
    out = run_go(["list", "-m", "-f", "{{.Path}} {{.Dir}}", "all"], cwd=full_dir)
    for line in out.splitlines():
        if line.startswith(AKITA_IMPORT_PREFIX):
            path, _, dir_ = line.partition(" ")
            return path, Path(dir_)
    raise RuntimeError(
        f"could not find a module under {AKITA_IMPORT_PREFIX!r} via `go list -m all` in {full_dir}"
    )


def resolve_akita_files(full_dir: Path, akita_import_path: str, exclude_tests: bool):
    """Returns (akita_pkgs, [(file_path, import_path), ...]) for every
    akita package actually reachable from AMD_CODE_SUBDIRS (used_akita
    methodology). Doesn't count lines itself -- that happens after
    comment-stripping, in count_akita_lines()."""
    deps_args = ["-deps"]
    if not exclude_tests:
        deps_args.append("-test")
    pkg_patterns = [f"./{AMD_ROOT}/{d}/..." for d in AMD_CODE_SUBDIRS]
    out = run_go(["list", *deps_args, *pkg_patterns], cwd=full_dir)
    deps = sorted({line.strip() for line in out.splitlines() if line.strip()})
    akita_pkgs = [d for d in deps if d.startswith(akita_import_path)]
    if not akita_pkgs:
        raise RuntimeError(f"no akita packages found among {AMD_ROOT}/ dependencies in {full_dir}")

    dir_out = run_go(["list", "-f", "{{.ImportPath}} {{.Dir}}", *akita_pkgs], cwd=full_dir)
    files = []
    for line in dir_out.splitlines():
        import_path, _, dir_ = line.partition(" ")
        d = Path(dir_)
        for f in iter_go_files(d, exclude_tests, recursive=False):
            files.append((f, import_path))
    return akita_pkgs, files


def count_akita_lines(akita_files, stripped: dict):
    total = 0
    records = []
    for f, import_path in akita_files:
        n = len(nonblank_lines_from_text(stripped[f.resolve()]))
        total += n
        records.append({"path": str(f), "package": import_path, "lines": n})
    return total, records


# ---- Main ------------------------------------------------------------------


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--repo-root", type=Path, default=None, help="Repo root to analyze (default: auto-detect from cwd)")
    parser.add_argument("--verbose", action="store_true", help="Print the full per-file breakdown to stdout")
    args = parser.parse_args()

    repo_root = (args.repo_root or find_repo_root(Path.cwd())).resolve()

    print(f"repo root : {repo_root}")
    print(f"main ref  : {MAIN_REF}")
    print(f"old ref   : {OLD_REF}")
    print(f"exclude _test.go files : {EXCLUDE_TEST_FILES}")
    print()

    print("building strip_comments.go ...", file=sys.stderr)
    strip_comments_bin = build_strip_comments_binary()

    print(f"materializing {OLD_REF}'s amd/ ...", file=sys.stderr)
    old_amd_dir = materialize_amd_only(repo_root, OLD_REF) / AMD_ROOT

    print(f"materializing {MAIN_REF}'s full tree (needed for module resolution) ...", file=sys.stderr)
    main_full_dir = materialize_full_repo(repo_root, MAIN_REF)
    main_amd_dir = main_full_dir / AMD_ROOT

    print(f"detecting renames/moves between {OLD_REF} and {MAIN_REF} (git diff -M) ...", file=sys.stderr)
    rename_map = build_rename_map(repo_root, OLD_REF, MAIN_REF)
    print(f"  {len(rename_map)} changed/renamed files mapped", file=sys.stderr)

    new_side_files = list(iter_amd_code_files(main_amd_dir, EXCLUDE_TEST_FILES))
    old_side_files = []
    for f in new_side_files:
        rel = str(f.relative_to(main_amd_dir))
        old_f = old_amd_dir / rename_map.get(rel, rel)
        if old_f.is_file():
            old_side_files.append(old_f)

    print(f"stripping comments from {len(new_side_files) + len(old_side_files)} amd/ files ...", file=sys.stderr)
    diff_stripped = strip_comments_batch(strip_comments_bin, new_side_files + old_side_files)

    unchanged_diff_lines, new_lines, diff_records = classify_amd_diff(
        old_amd_dir, main_amd_dir, EXCLUDE_TEST_FILES, rename_map, diff_stripped
    )
    print(f"  amd/ (unchanged vs. {OLD_REF}): {unchanged_diff_lines:,} lines", file=sys.stderr)
    print(f"  amd/ (new relative to {OLD_REF}): {new_lines:,} lines", file=sys.stderr)

    print(f"resolving {MAIN_REF}'s module (fixing local replace + `go mod download`) ...", file=sys.stderr)
    ensure_module_resolvable(main_full_dir)

    akita_import_path, akita_module_dir = resolve_akita_module(main_full_dir)
    print(f"akita module: {akita_import_path} @ {akita_module_dir}")

    akita_pkgs_used, akita_files = resolve_akita_files(main_full_dir, akita_import_path, EXCLUDE_TEST_FILES)
    print(f"akita packages actually imported by {MAIN_REF}'s {AMD_ROOT}/ code: {len(akita_pkgs_used)}")

    print(f"stripping comments from {len(akita_files)} akita files ...", file=sys.stderr)
    akita_stripped = strip_comments_batch(strip_comments_bin, [f for f, _ in akita_files])
    akita_lines, akita_records = count_akita_lines(akita_files, akita_stripped)

    unchanged_lines = unchanged_diff_lines + akita_lines
    adapted_lines = 0
    total_lines = unchanged_lines + adapted_lines + new_lines

    print()
    print("=" * 72)
    print(f"Line-of-Code Reuse Rate within AMD ({MAIN_REF} vs. {OLD_REF})")
    print("=" * 72)
    print(f"(1) total amd/ (on {MAIN_REF}) + akita lines : {total_lines:>8,}")
    print(f"(2) unchanged (identical to {OLD_REF} + akita): {unchanged_lines:>8,}  ({pct(unchanged_lines, total_lines):5.1f}%)")
    print(f"      of which: identical to {OLD_REF}          : {unchanged_diff_lines:>8,}")
    print(f"                akita (used_akita)              : {akita_lines:>8,}")
    print(f"(3) adapted (n/a for same-vendor evolution)   : {adapted_lines:>8,}  ({pct(adapted_lines, total_lines):5.1f}%)")
    print(f"(4) new (relative to {OLD_REF})                : {new_lines:>8,}  ({pct(new_lines, total_lines):5.1f}%)")
    print("-" * 72)
    reuse_lines = unchanged_lines + adapted_lines
    print(f"combined reuse rate (2)+(3)                   : {reuse_lines:>8,}  ({pct(reuse_lines, total_lines):5.1f}%)")
    print("=" * 72)

    if args.verbose:
        print("\n-- amd/ files (diff vs. old ref) --")
        for r in diff_records:
            print(f"  unchanged={r['unchanged_lines']:>6,} new={r['new_lines']:>6,}  {r['path']}"
                  f"{'' if r['existed_in_old'] else '  [new file]'}")
        print("\n-- akita files --")
        for r in akita_records:
            print(f"  {r['lines']:>6,}  {r['path']} ({r['package']})")

    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    summary = {
        "repo_root": str(repo_root),
        "main_ref": MAIN_REF,
        "old_ref": OLD_REF,
        "exclude_test_files": EXCLUDE_TEST_FILES,
        "akita_import_path": akita_import_path,
        "akita_module_dir": str(akita_module_dir),
        "akita_packages_used": akita_pkgs_used,
        "total_lines": total_lines,
        "unchanged_lines": unchanged_lines,
        "unchanged_diff_lines": unchanged_diff_lines,
        "unchanged_akita_lines": akita_lines,
        "adapted_lines": adapted_lines,
        "new_lines": new_lines,
        "unchanged_pct": pct(unchanged_lines, total_lines),
        "adapted_pct": pct(adapted_lines, total_lines),
        "new_pct": pct(new_lines, total_lines),
        "combined_reuse_lines": reuse_lines,
        "combined_reuse_pct": pct(reuse_lines, total_lines),
    }
    summary_path = OUTPUT_DIR / "summary.json"
    summary_path.write_text(json.dumps(summary, indent=2) + "\n")

    files_path = OUTPUT_DIR / "files.csv"
    with files_path.open("w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["source", "bucket", "package", "unchanged_lines", "new_lines", "path", "matched_old_path"])
        for r in diff_records:
            writer.writerow(["amd", "diff", "", r["unchanged_lines"], r["new_lines"], r["path"], r["matched_old_path"]])
        for r in akita_records:
            writer.writerow(["akita", "unchanged", r["package"], r["lines"], 0, r["path"], ""])

    print(f"\nwrote {summary_path}")
    print(f"wrote {files_path}")


if __name__ == "__main__":
    main()
