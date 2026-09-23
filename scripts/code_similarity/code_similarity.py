#!/usr/bin/env python3
"""Semantic Code Similarity for the BeyondV NVIDIA simulator.

Implements the "Semantic Code Similarity" metric from the paper's Section
V-B ("Code Reuse Validation"): embed every function with a pretrained code
model and report the mean pairwise cosine similarity between corpora,
rather than the LOC-textual-identity metric in count_loc.py.

Matching mode is "brute force": we do NOT try to pair a specific function
with a specific counterpart. Every function in corpus A is compared
against every function in corpus B, and we report the aggregate
statistics (mean, median, std, percentiles) over the full N x M pairwise
cosine-similarity matrix.

Four corpora (see CORPUS_* below):
  mgpusim_nvidia      -- our NVIDIA simulator (this repo's nvidia/ folders,
                          nvidia_instruction branch's working tree)
  mgpusim_amd_MI300X  -- mgpusim's amd/ on the "main" branch: the current
                          AMD target (MI300X)
  mgpusim_amd_r9nano  -- mgpusim's amd/ on the "v4" branch: mgpusim's
                          original AMD target (R9 Nano), before MI300X
                          support landed on main
  gpgpusim            -- GPGPU-Sim, an independently developed NVIDIA
                          simulator with no shared code or authorship

We report a ROWS x COLS matrix of pairwise cosine-similarity statistics
(ROWS/COLS below): each row corpus against itself (self-similarity, i<j
pairs only, no trivial i==j pairs) and against each of the reference
corpora in COLS. "itself" tells us what "high" looks like within a single
codebase/language; mgpusim_amd_r9nano is a same-project, different-branch
control (does our NVIDIA code and the current AMD code both still read
close to the AMD code they were each historically derived/diverged from?);
gpgpusim is the unrelated-simulator floor. Transformer embedding cosine
similarity is known to be anisotropic (unrelated snippets in the same
corpus/language routinely score well above 0), so raw numbers should be
read relative to these baselines, not against an absolute 0-1 scale.

Usage (run from anywhere; nvidia_instruction branch, since it's the only
branch where nvidia/ is tracked):

    python3 scripts/code_similarity/code_similarity.py
    python3 scripts/code_similarity/code_similarity.py --method codebert
    python3 scripts/code_similarity/code_similarity.py --mock-embeddings   # dry run, no model download

Requires (for a real run): `pip install transformers torch numpy`, and the
`go` toolchain on PATH (used only for precise Go function extraction via
extract_go_funcs.go). GPGPU-Sim's C++ is extracted with a regex/brace-depth
heuristic, not a real parser -- see extract_cpp_functions() -- expect some
noise (false positives on macros, missed template edge cases); this is a
known, accepted limitation of comparing against a non-Go codebase without
a full C++ frontend.
"""

import argparse
import hashlib
import json
import pickle
import re
import subprocess
import sys
from pathlib import Path

# ----------------------------------------------------------------------------
# Configuration -- edit these to change what gets embedded / compared
# ----------------------------------------------------------------------------

# "codebert" | "graphcodebert" | "unixcoder"
METHOD = "unixcoder"

MODEL_NAMES = {
    "codebert": "microsoft/codebert-base",
    "graphcodebert": "microsoft/graphcodebert-base",
    "unixcoder": "microsoft/unixcoder-base",
}

# Excluded from every count/comparison below.
EXCLUDE_TEST_FILES = True

# Matches count_loc.py's scope: the NVIDIA simulator's own code.
NVIDIA_CODE_FOLDERS = [
    "nvidia/driver",
    "nvidia/gpu",
    "nvidia/message",
    "nvidia/platform",
    "nvidia/runner",
    "nvidia/sm",
    "nvidia/smsp",
    "nvidia/trace",
]

# ---- Corpus names --------------------------------------------------------
CORPUS_NVIDIA = "mgpusim_nvidia"
CORPUS_AMD_MI300X = "mgpusim_amd_MI300X"
CORPUS_AMD_R9NANO = "mgpusim_amd_r9nano"
CORPUS_GPGPUSIM = "gpgpusim"

# Which ref of *this* repo (mgpusim IS a clone of sarchlab/mgpusim, so no
# network access is needed for either of these) each AMD-side corpus's
# amd/ folder is pulled from.
AMD_SNAPSHOT_REFS = {
    CORPUS_AMD_MI300X: "origin/main",
    CORPUS_AMD_R9NANO: "origin/v4",
}
AMD_ROOT = "amd"
AMD_EXCLUDE_PREFIXES = [
    "amd/benchmarks/",
    "amd/tests/",
]
AMD_EXCLUDE_FILES = {
    "amd/run_before_merge.sh",
}

GPGPU_SIM_REPO_URL = "https://github.com/gpgpu-sim/gpgpu-sim_distribution.git"
GPGPU_SIM_EXTENSIONS = {".cc", ".cpp", ".cxx", ".c", ".h", ".hpp"}

SCRIPT_DIR = Path(__file__).resolve().parent
EXTERNAL_DIR = SCRIPT_DIR / "external"
GPGPU_SIM_DIR = EXTERNAL_DIR / "gpgpu-sim_distribution"
CACHE_DIR = SCRIPT_DIR / "cache"
OUTPUT_DIR = SCRIPT_DIR / "output"

# The comparison matrix: for each row corpus, report its similarity to
# itself and to each corpus in COLS (excluding "itself", which always
# means the row corpus's own self-similarity).
ROWS = [CORPUS_NVIDIA, CORPUS_AMD_MI300X]
COLS = ["itself", CORPUS_AMD_R9NANO, CORPUS_GPGPUSIM]

# Dump the K highest-scoring pairs for every (row, col) cell to disk as
# plain-text files, for eyeballing what "most similar" actually looks
# like. Written under SCRIPT_DIR/results_<method>/<row>_vs_<col>_top_<K>/,
# fully refreshed (old files removed) on every run.
DUMP_TOP_PAIRS = True
TOP_K_PAIRS = 200

# ----------------------------------------------------------------------------


# ---- Go extraction (exact, via go/ast) --------------------------------------


def extract_go_functions(roots, repo_root: Path, exclude_tests: bool = EXCLUDE_TEST_FILES):
    """Runs extract_go_funcs.go over `roots` (paths relative to repo_root)
    and returns a list of {path, name, receiver, start_line, end_line, source}."""
    extractor = SCRIPT_DIR / "extract_go_funcs.go"
    args = ["go", "run", str(extractor)]
    if not exclude_tests:
        args.append("--include-tests")
    args.extend(roots)
    result = subprocess.run(args, cwd=repo_root, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        raise RuntimeError(f"extract_go_funcs.go failed: {result.stderr}")
    records = json.loads(result.stdout)
    for r in records:
        r["language"] = "go"
    return records


# ---- C++ extraction (heuristic, regex + brace-depth) -------------------------

_CPP_CONTROL_KEYWORDS = {
    "if", "for", "while", "switch", "catch", "else", "do", "return",
    "sizeof", "using", "namespace", "typedef", "template", "struct",
    "class", "union", "enum", "public", "private", "protected", "new",
    "delete", "throw", "try", "static_assert", "extern", "friend",
    "operator",
}

_CPP_STRING_OR_COMMENT = re.compile(
    r"//[^\n]*"                      # line comment
    r"|/\*.*?\*/"                    # block comment
    r"|\"(?:\\.|[^\"\\])*\""         # string literal
    r"|'(?:\\.|[^'\\])*'",           # char literal
    re.DOTALL,
)

_CPP_IDENT_PAREN = re.compile(r"([A-Za-z_~]\w*)\s*\(")
_CPP_TRAILING_QUALIFIER = re.compile(r"\s*(const|noexcept|override|final)\b")


def _cpp_blank_comments_and_strings(text: str) -> str:
    def repl(m):
        return "".join("\n" if c == "\n" else " " for c in m.group(0))

    return _CPP_STRING_OR_COMMENT.sub(repl, text)


def _cpp_find_matching_paren(text: str, open_pos: int) -> int:
    """text[open_pos] must be '('. Returns index of the matching ')'."""
    depth = 0
    for i in range(open_pos, len(text)):
        if text[i] == "(":
            depth += 1
        elif text[i] == ")":
            depth -= 1
            if depth == 0:
                return i
    return -1


def _cpp_find_matching_brace(text: str, open_pos: int) -> int:
    depth = 0
    for i in range(open_pos, len(text)):
        if text[i] == "{":
            depth += 1
        elif text[i] == "}":
            depth -= 1
            if depth == 0:
                return i
    return -1


def extract_cpp_functions_from_text(text: str, path_label: str):
    """Heuristic extraction: not a real parser. For every `identifier(`,
    balance-matches the argument list, skips optional trailing
    const/noexcept/override qualifiers and a constructor initializer list
    (`: a(1), b(2)`, itself paren-balanced), and accepts it as a function
    definition only if what remains is a `{` (not `;`, which would make it
    a call or a declaration). Rejects control-flow keywords. Expect some
    noise (macros with unusual bodies, lambdas, heavily macro-generated
    code) -- a known, accepted limitation of comparing against a non-Go
    codebase without a full C++ frontend.
    """
    clean = _cpp_blank_comments_and_strings(text)
    candidates = []
    for m in _CPP_IDENT_PAREN.finditer(clean):
        name = m.group(1)
        if name in _CPP_CONTROL_KEYWORDS:
            continue
        open_paren = m.end() - 1
        close_paren = _cpp_find_matching_paren(clean, open_paren)
        if close_paren == -1:
            continue

        pos = close_paren + 1
        while True:
            qm = _CPP_TRAILING_QUALIFIER.match(clean, pos)
            if not qm:
                break
            pos = qm.end()

        ws = pos
        while ws < len(clean) and clean[ws].isspace():
            ws += 1

        if ws < len(clean) and clean[ws] == ":":
            i = ws + 1
            depth = 0
            while i < len(clean):
                c = clean[i]
                if c == "(":
                    depth += 1
                elif c == ")":
                    depth -= 1
                elif c in "{;" and depth == 0:
                    break
                i += 1
            pos = i
        else:
            pos = ws

        if pos >= len(clean) or clean[pos] != "{":
            continue

        end_pos = _cpp_find_matching_brace(clean, pos)
        if end_pos == -1:
            continue

        candidates.append((m.start(), end_pos, name))

    # Constructor initializer lists (`: a(1), b(2) {`) and, less often,
    # nested lambdas/calls that happen to be followed by a brace, produce
    # spurious matches fully nested inside a real match's span (finditer
    # keeps scanning past the outer match's regex-matched prefix, into
    # territory our own manual brace-matching already claimed). Keep only
    # the outermost match at each nesting level.
    candidates.sort(key=lambda c: (c[0], -c[1]))
    out = []
    last_end = -1
    for start, end_pos, name in candidates:
        if start < last_end:
            continue  # nested inside the previous (larger) match
        last_end = end_pos
        start_line = clean.count("\n", 0, start) + 1
        end_line = clean.count("\n", 0, end_pos) + 1
        out.append({
            "path": path_label,
            "name": name,
            "receiver": "",
            "start_line": start_line,
            "end_line": end_line,
            "source": text[start:end_pos + 1],
            "language": "cpp",
        })
    return out


def extract_cpp_functions(root: Path, extensions=GPGPU_SIM_EXTENSIONS):
    records = []
    for path in sorted(root.rglob("*")):
        if not path.is_file() or path.suffix not in extensions:
            continue
        try:
            text = path.read_text(encoding="utf-8", errors="replace")
        except OSError as exc:
            print(f"warning: could not read {path}: {exc}", file=sys.stderr)
            continue
        rel = str(path.relative_to(root))
        records.extend(extract_cpp_functions_from_text(text, rel))
    return records


# ---- Corpus assembly ---------------------------------------------------------


def materialize_amd_corpus(repo_root: Path, ref: str) -> Path:
    """Extracts amd/ (minus excludes) from `ref` in this repo's own git
    history into CACHE_DIR/amd_src_<sha>/, without touching the network
    (this repo already is a clone of sarchlab/mgpusim)."""
    sha = subprocess.run(
        ["git", "rev-parse", ref], cwd=repo_root, capture_output=True, text=True, check=True
    ).stdout.strip()
    dest = CACHE_DIR / f"amd_src_{sha[:12]}"
    if dest.exists():
        return dest
    dest.mkdir(parents=True, exist_ok=True)
    archive = subprocess.Popen(
        ["git", "archive", sha, "--", AMD_ROOT], cwd=repo_root, stdout=subprocess.PIPE
    )
    subprocess.run(["tar", "-x", "-C", str(dest)], stdin=archive.stdout, check=True)
    archive.wait()

    for excluded in ("benchmarks", "tests"):
        p = dest / AMD_ROOT / excluded
        if p.exists():
            _rmtree(p)
    for f in AMD_EXCLUDE_FILES:
        p = dest / f
        if p.exists():
            p.unlink()
    return dest


def _rmtree(path: Path):
    import shutil
    shutil.rmtree(path)


def ensure_gpgpu_sim_clone() -> Path:
    if (GPGPU_SIM_DIR / ".git").exists():
        return GPGPU_SIM_DIR
    EXTERNAL_DIR.mkdir(parents=True, exist_ok=True)
    print(f"cloning {GPGPU_SIM_REPO_URL} -> {GPGPU_SIM_DIR} ...", file=sys.stderr)
    subprocess.run(
        ["git", "clone", "--depth", "1", GPGPU_SIM_REPO_URL, str(GPGPU_SIM_DIR)], check=True
    )
    return GPGPU_SIM_DIR


def load_all_corpora(repo_root: Path):
    """Returns {corpus_name: [function records]} for every corpus in
    CORPUS_* (all four are always loaded, regardless of ROWS/COLS, since
    extraction is cheap relative to embedding and it keeps the cache keys
    stable)."""
    records = {}

    print(f"extracting {CORPUS_NVIDIA} functions...", file=sys.stderr)
    records[CORPUS_NVIDIA] = extract_go_functions(NVIDIA_CODE_FOLDERS, repo_root)
    print(f"  {len(records[CORPUS_NVIDIA])} functions", file=sys.stderr)

    for corpus_name in (CORPUS_AMD_MI300X, CORPUS_AMD_R9NANO):
        ref = AMD_SNAPSHOT_REFS[corpus_name]
        print(f"materializing {corpus_name} (amd/ @ {ref}) functions...", file=sys.stderr)
        src_dir = materialize_amd_corpus(repo_root, ref)
        amd_records = extract_go_functions([AMD_ROOT], src_dir)
        amd_records = [
            r for r in amd_records
            if not any(r["path"].startswith(p) for p in AMD_EXCLUDE_PREFIXES)
            and r["path"] not in AMD_EXCLUDE_FILES
        ]
        records[corpus_name] = amd_records
        print(f"  {len(amd_records)} functions", file=sys.stderr)

    print(f"preparing {CORPUS_GPGPUSIM} functions...", file=sys.stderr)
    gpgpu_dir = ensure_gpgpu_sim_clone()
    records[CORPUS_GPGPUSIM] = extract_cpp_functions(gpgpu_dir)
    print(f"  {len(records[CORPUS_GPGPUSIM])} functions (heuristic C++ extraction)", file=sys.stderr)

    return records


# ---- Embedding ----------------------------------------------------------------


class Embedder:
    """Wraps a HuggingFace encoder; embeddings are mean-pooled, L2-normalized
    last-hidden-states. `mock=True` swaps in a deterministic hash-based
    pseudo-embedding so the rest of the pipeline (extraction, caching,
    matrix math) can be exercised without downloading model weights."""

    def __init__(self, method: str, mock: bool = False, dim: int = 768, max_length: int = 512):
        self.method = method
        self.mock = mock
        self.dim = dim
        self.max_length = max_length
        self._tokenizer = None
        self._model = None
        if not mock:
            import torch  # noqa: F401  (fail fast if missing)
            from transformers import AutoModel, AutoTokenizer

            model_name = MODEL_NAMES[method]
            self._tokenizer = AutoTokenizer.from_pretrained(model_name)
            self._model = AutoModel.from_pretrained(model_name)
            self._model.eval()
            self.dim = self._model.config.hidden_size

    def embed_batch(self, texts):
        import numpy as np

        if self.mock:
            return np.stack([self._mock_vector(t) for t in texts])

        import torch

        with torch.no_grad():
            enc = self._tokenizer(
                texts, padding=True, truncation=True, max_length=self.max_length, return_tensors="pt"
            )
            out = self._model(**enc)
            hidden = out.last_hidden_state
            mask = enc["attention_mask"].unsqueeze(-1).float()
            summed = (hidden * mask).sum(dim=1)
            counts = mask.sum(dim=1).clamp(min=1e-9)
            pooled = (summed / counts).cpu().numpy()

        norms = (pooled ** 2).sum(axis=1, keepdims=True) ** 0.5
        norms[norms == 0] = 1.0
        return pooled / norms

    def _mock_vector(self, text: str):
        import numpy as np

        h = hashlib.sha256(text.encode("utf-8", errors="replace")).digest()
        rng = np.random.default_rng(int.from_bytes(h[:8], "little"))
        v = rng.normal(size=self.dim)
        return v / (np.linalg.norm(v) or 1.0)


def corpus_cache_key(corpus_name: str, method: str, records) -> str:
    h = hashlib.sha256()
    h.update(f"{corpus_name}:{method}:{len(records)}".encode())
    for r in records[:50]:
        h.update(r["source"][:200].encode("utf-8", errors="replace"))
    return h.hexdigest()[:16]


def embed_corpus(corpus_name: str, records, embedder: Embedder, batch_size: int = 16, refresh: bool = False):
    import numpy as np

    tag = "mock" if embedder.mock else embedder.method
    cache_key = corpus_cache_key(corpus_name, tag, records)
    cache_path = CACHE_DIR / f"embeddings_{corpus_name}_{tag}_{cache_key}.pkl"
    if cache_path.exists() and not refresh:
        with cache_path.open("rb") as f:
            cached = pickle.load(f)
        print(f"  [{corpus_name}] loaded {len(records)} embeddings from cache: {cache_path.name}", file=sys.stderr)
        return cached["embeddings"]

    embeddings = np.zeros((len(records), embedder.dim), dtype=np.float32)
    for i in range(0, len(records), batch_size):
        batch = records[i:i + batch_size]
        vecs = embedder.embed_batch([r["source"] for r in batch])
        embeddings[i:i + len(batch)] = vecs
        if (i // batch_size) % 20 == 0:
            print(f"  [{corpus_name}] embedded {i + len(batch)}/{len(records)}", file=sys.stderr)

    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    with cache_path.open("wb") as f:
        pickle.dump({"embeddings": embeddings}, f)
    return embeddings


# ---- Similarity stats ----------------------------------------------------------


def pairwise_stats(embA, embB, exclude_diagonal: bool = False):
    import numpy as np

    sim = embA @ embB.T
    if exclude_diagonal:
        mask = ~np.eye(sim.shape[0], sim.shape[1], dtype=bool)
        vals = sim[mask]
    else:
        vals = sim.reshape(-1)
    return {
        "pairs": int(vals.size),
        "mean": float(np.mean(vals)),
        "median": float(np.median(vals)),
        "std": float(np.std(vals)),
        "min": float(np.min(vals)),
        "max": float(np.max(vals)),
        "p10": float(np.percentile(vals, 10)),
        "p90": float(np.percentile(vals, 90)),
    }


def top_k_pairs(embA, embB, k: int, upper_triangle_only: bool = False):
    """Returns the k highest-scoring (score, i, j) triples from embA @ embB.T.

    upper_triangle_only=True is for a corpus compared against itself: it
    keeps only i < j, which simultaneously excludes the trivial i == j
    self-pairs and the symmetric mirror of every pair (since the matrix is
    symmetric in that case, (i, j) and (j, i) carry the same score)."""
    import numpy as np

    sim = embA @ embB.T
    if upper_triangle_only:
        sim = np.where(np.triu(np.ones(sim.shape, dtype=bool), k=1), sim, -np.inf)

    flat = sim.reshape(-1)
    k = min(k, int(np.isfinite(flat).sum()))
    top_idx = np.argpartition(flat, -k)[-k:]
    top_idx = top_idx[np.argsort(flat[top_idx])[::-1]]

    pairs = []
    for idx in top_idx:
        i, j = np.unravel_index(int(idx), sim.shape)
        pairs.append((float(sim[i, j]), int(i), int(j)))
    return pairs


def format_function_ref(label: str, rec: dict) -> str:
    qualified_name = f"{rec['receiver']}.{rec['name']}" if rec.get("receiver") else rec["name"]
    return f"{label} :: {rec['path']} :: {qualified_name} (lines {rec['start_line']}-{rec['end_line']})"


def dump_top_pairs(results_root: Path, row_name: str, col_label: str,
                    records_row, records_col, pairs, k: int):
    folder = results_root / f"{row_name}_vs_{col_label}_top_{k}"
    if folder.exists():
        existing = list(folder.glob("*.txt"))
        print(f"removing {len(existing)} results in {folder}/", file=sys.stderr)
        for f in existing:
            f.unlink()
    folder.mkdir(parents=True, exist_ok=True)

    sep = "-" * 60
    for rank, (score, i, j) in enumerate(pairs, start=1):
        rec_a, rec_b = records_row[i], records_col[j]
        content = (
            f"{sep}\n"
            f"function from {format_function_ref(row_name, rec_a)}\n"
            f"{sep}\n"
            f"{rec_a['source']}\n"
            f"\n"
            f"{sep}\n"
            f"function from {format_function_ref(col_label, rec_b)}\n"
            f"{sep}\n"
            f"{rec_b['source']}\n"
        )
        (folder / f"{rank}_{score:.6f}.txt").write_text(content, encoding="utf-8")

    print(f"wrote {len(pairs)} pair files to {folder}/", file=sys.stderr)


# ---- Main ------------------------------------------------------------------


def find_repo_root(start: Path) -> Path:
    out = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"], cwd=start, capture_output=True, text=True, check=True
    )
    return Path(out.stdout.strip())


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--repo-root", type=Path, default=None)
    parser.add_argument("--method", choices=list(MODEL_NAMES), default=None, help=f"Override METHOD (default: {METHOD!r})")
    parser.add_argument("--mock-embeddings", action="store_true", help="Use deterministic pseudo-embeddings instead of downloading a model (pipeline dry run)")
    parser.add_argument("--refresh", action="store_true", help="Ignore cached embeddings and recompute")
    args = parser.parse_args()

    method = args.method or METHOD
    repo_root = (args.repo_root or find_repo_root(Path.cwd())).resolve()
    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    print(f"repo root : {repo_root}")
    print(f"method    : {method}{' (mock)' if args.mock_embeddings else ''}")
    print()

    all_records = load_all_corpora(repo_root)

    embedder = Embedder(method, mock=args.mock_embeddings)

    print("embedding corpora...", file=sys.stderr)
    all_embeddings = {
        name: embed_corpus(name, recs, embedder, refresh=args.refresh)
        for name, recs in all_records.items()
    }

    matrix = {}
    for row in ROWS:
        matrix[row] = {}
        for col in COLS:
            if col == "itself":
                stats = pairwise_stats(all_embeddings[row], all_embeddings[row], exclude_diagonal=True)
            else:
                stats = pairwise_stats(all_embeddings[row], all_embeddings[col])
            matrix[row][col] = stats

    results = {
        "method": method,
        "mock_embeddings": args.mock_embeddings,
        "corpus_sizes": {name: len(recs) for name, recs in all_records.items()},
        "matrix": matrix,
    }

    print()
    print("=" * 96)
    print(f"Semantic Code Similarity ({method}{'  [MOCK]' if args.mock_embeddings else ''})")
    print("=" * 96)
    print("corpus sizes: " + "  ".join(f"{name}={n}" for name, n in results["corpus_sizes"].items()))
    print()
    col_width = 26
    header = " " * 22 + "".join(f"{col:<{col_width}}" for col in COLS)
    print(header)
    for row in ROWS:
        cells = []
        for col in COLS:
            s = matrix[row][col]
            cells.append(f"{s['mean']:.3f} +/- {s['std']:.3f}".ljust(col_width))
        print(f"{row:<22}" + "".join(cells))
    print("=" * 96)

    out_path = OUTPUT_DIR / f"similarity_{method}{'_mock' if args.mock_embeddings else ''}.json"
    out_path.write_text(json.dumps(results, indent=2) + "\n")
    print(f"\nwrote {out_path}")

    if DUMP_TOP_PAIRS:
        results_root = SCRIPT_DIR / f"results_{method}"
        print(f"\ndumping top-{TOP_K_PAIRS} pairs to {results_root}/ ...", file=sys.stderr)
        for row in ROWS:
            for col in COLS:
                if col == "itself":
                    pairs = top_k_pairs(all_embeddings[row], all_embeddings[row], TOP_K_PAIRS, upper_triangle_only=True)
                    dump_top_pairs(results_root, row, "itself", all_records[row], all_records[row], pairs, TOP_K_PAIRS)
                else:
                    pairs = top_k_pairs(all_embeddings[row], all_embeddings[col], TOP_K_PAIRS)
                    dump_top_pairs(results_root, row, col, all_records[row], all_records[col], pairs, TOP_K_PAIRS)


if __name__ == "__main__":
    main()
