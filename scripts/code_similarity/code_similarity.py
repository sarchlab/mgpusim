#!/usr/bin/env python3
"""Semantic Code Similarity for the BeyondV NVIDIA simulator.

Implements the "Semantic Code Similarity" metric from the paper's Section
V-B ("Code Reuse Validation"): embed every function with a pretrained code
model and report the mean pairwise cosine similarity between corpora,
rather than the LOC-textual-identity metric in count_loc.py.

Matching mode is "brute force": we do NOT try to pair a specific NVIDIA
function with a specific counterpart. Every function in corpus A is
compared against every function in corpus B, and we report the aggregate
statistics (mean, median, std, percentiles) over the full N x M pairwise
cosine-similarity matrix. This sidesteps the need for a curated or
nearest-neighbor matching scheme, at the cost of the "xx matched function
pairs" framing the LOC-style metric uses -- the number here is N x M pairs,
not a curated 1:1 mapping.

Three comparisons:
  (1) nvidia/  vs  amd/    (this repo's origin/main, minus excluded paths)
                            -- "does our retargeted code stay semantically
                            close to the codebase it was retargeted from?"
  (2) nvidia/  vs  GPGPU-Sim
                            -- an unrelated-simulator baseline: two
                            independently designed NVIDIA simulators with
                            no shared code or authorship, for calibrating
                            what a "low" score looks like.
  (3) optional same-corpus self-baselines (nvidia vs nvidia, amd vs amd,
      GPGPU-Sim vs GPGPU-Sim, excluding self-pairs) -- context for what a
      "high" score looks like within one codebase/language, since
      transformer embedding cosine similarity is known to be anisotropic
      (unrelated snippets in the same corpus/language routinely score well
      above 0, so raw cross-corpus numbers should be read relative to
      these baselines, not against an absolute 0-1 scale).

Usage (run from anywhere; nvidia_instruction branch, since it's the only
branch where nvidia/ is tracked):

    python3 scripts/code_similarity/code_similarity.py
    python3 scripts/code_similarity/code_similarity.py --method graphcodebert
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
METHOD = "codebert"

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

# Which ref of *this* repo to pull the original AMD-only MGPUSim source
# from. Uses local git history (this repo IS a clone of sarchlab/mgpusim),
# so no network access is needed for this corpus.
AMD_GIT_REF = "origin/main"
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

# Compute same-corpus self-similarity baselines (see module docstring, item 3).
COMPUTE_SELF_BASELINE = True
# Cap self-baseline matrix size for large corpora (amd/, GPGPU-Sim can have
# thousands of functions; N^2 pairs gets expensive/large fast).
SELF_BASELINE_MAX_FUNCTIONS = 800
SELF_BASELINE_SEED = 0

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


def materialize_amd_corpus(repo_root: Path, ref: str = AMD_GIT_REF) -> Path:
    """Extracts amd/ (minus excludes) from `ref` in this repo's own git
    history into CACHE_DIR/amd_<ref>_<sha>/, without touching the network
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
        n = min(sim.shape)
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


def subsample(records, embeddings, max_n, seed):
    import numpy as np

    if len(records) <= max_n:
        return records, embeddings
    rng = np.random.default_rng(seed)
    idx = rng.choice(len(records), size=max_n, replace=False)
    idx.sort()
    return [records[i] for i in idx], embeddings[idx]


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
    parser.add_argument("--skip-self-baseline", action="store_true")
    args = parser.parse_args()

    method = args.method or METHOD
    repo_root = (args.repo_root or find_repo_root(Path.cwd())).resolve()
    CACHE_DIR.mkdir(parents=True, exist_ok=True)
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    print(f"repo root : {repo_root}")
    print(f"method    : {method}{' (mock)' if args.mock_embeddings else ''}")
    print()

    print("extracting nvidia/ functions...", file=sys.stderr)
    nvidia_records = extract_go_functions(NVIDIA_CODE_FOLDERS, repo_root)
    print(f"  {len(nvidia_records)} functions", file=sys.stderr)

    print("materializing amd/ (origin/main) functions...", file=sys.stderr)
    amd_src_dir = materialize_amd_corpus(repo_root)
    amd_records = extract_go_functions([AMD_ROOT], amd_src_dir)
    amd_records = [
        r for r in amd_records
        if not any(r["path"].startswith(p) for p in AMD_EXCLUDE_PREFIXES)
        and r["path"] not in AMD_EXCLUDE_FILES
    ]
    print(f"  {len(amd_records)} functions", file=sys.stderr)

    print("preparing GPGPU-Sim functions...", file=sys.stderr)
    gpgpu_dir = ensure_gpgpu_sim_clone()
    gpgpu_records = extract_cpp_functions(gpgpu_dir)
    print(f"  {len(gpgpu_records)} functions (heuristic C++ extraction)", file=sys.stderr)

    embedder = Embedder(method, mock=args.mock_embeddings)

    print("embedding corpora...", file=sys.stderr)
    nvidia_emb = embed_corpus("nvidia", nvidia_records, embedder, refresh=args.refresh)
    amd_emb = embed_corpus("amd_main", amd_records, embedder, refresh=args.refresh)
    gpgpu_emb = embed_corpus("gpgpu_sim", gpgpu_records, embedder, refresh=args.refresh)

    results = {
        "method": method,
        "mock_embeddings": args.mock_embeddings,
        "corpus_sizes": {
            "nvidia": len(nvidia_records),
            "amd_main": len(amd_records),
            "gpgpu_sim": len(gpgpu_records),
        },
        "nvidia_vs_amd": pairwise_stats(nvidia_emb, amd_emb),
        "nvidia_vs_gpgpu_sim": pairwise_stats(nvidia_emb, gpgpu_emb),
    }

    if COMPUTE_SELF_BASELINE and not args.skip_self_baseline:
        print("computing self-similarity baselines...", file=sys.stderr)
        n_rec, n_emb = subsample(nvidia_records, nvidia_emb, SELF_BASELINE_MAX_FUNCTIONS, SELF_BASELINE_SEED)
        a_rec, a_emb = subsample(amd_records, amd_emb, SELF_BASELINE_MAX_FUNCTIONS, SELF_BASELINE_SEED)
        g_rec, g_emb = subsample(gpgpu_records, gpgpu_emb, SELF_BASELINE_MAX_FUNCTIONS, SELF_BASELINE_SEED)
        results["self_baseline"] = {
            "nvidia_vs_nvidia": pairwise_stats(n_emb, n_emb, exclude_diagonal=True),
            "amd_vs_amd": pairwise_stats(a_emb, a_emb, exclude_diagonal=True),
            "gpgpu_sim_vs_gpgpu_sim": pairwise_stats(g_emb, g_emb, exclude_diagonal=True),
        }

    print()
    print("=" * 72)
    print(f"Semantic Code Similarity ({method}{'  [MOCK]' if args.mock_embeddings else ''})")
    print("=" * 72)
    print(f"corpus sizes: nvidia={results['corpus_sizes']['nvidia']}  "
          f"amd_main={results['corpus_sizes']['amd_main']}  "
          f"gpgpu_sim={results['corpus_sizes']['gpgpu_sim']}")
    print()
    for label, key in [("nvidia vs amd_main", "nvidia_vs_amd"), ("nvidia vs gpgpu_sim", "nvidia_vs_gpgpu_sim")]:
        s = results[key]
        print(f"{label:22s} pairs={s['pairs']:>10,}  mean={s['mean']:.4f}  median={s['median']:.4f}  "
              f"std={s['std']:.4f}  [p10={s['p10']:.4f}, p90={s['p90']:.4f}]")
    if "self_baseline" in results:
        print()
        print("self-similarity baselines (excluding self-pairs, subsampled):")
        for label, key in [("nvidia vs nvidia", "nvidia_vs_nvidia"), ("amd vs amd", "amd_vs_amd"), ("gpgpu_sim vs gpgpu_sim", "gpgpu_sim_vs_gpgpu_sim")]:
            s = results["self_baseline"][key]
            print(f"  {label:24s} pairs={s['pairs']:>8,}  mean={s['mean']:.4f}  median={s['median']:.4f}")
    print("=" * 72)

    out_path = OUTPUT_DIR / f"similarity_{method}{'_mock' if args.mock_embeddings else ''}.json"
    out_path.write_text(json.dumps(results, indent=2) + "\n")
    print(f"\nwrote {out_path}")


if __name__ == "__main__":
    main()
