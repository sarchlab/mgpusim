"""Write the accuracy report to ``GITHUB_STEP_SUMMARY`` with embedded charts.

This script replaces the GitHub Pages workflow.  Instead of rendering
``accuracy_report.md`` to a separate HTML page, it produces Markdown that is
appended to the per-step summary file referenced by the
``GITHUB_STEP_SUMMARY`` environment variable.  GitHub Actions renders that
file directly on the workflow run page and supports inline HTML, so each
PNG chart referenced from the report is embedded as a ``data:image/png``
URI inside a collapsible ``<details>`` block.

Usage::

    python3 scripts/generate_accuracy_summary.py
    python3 scripts/generate_accuracy_summary.py \
        --input accuracy_report.md \
        --figures-dir docs/figures \
        --summary-file "$GITHUB_STEP_SUMMARY"

If ``accuracy_report.md`` is missing the script exits 0 with a warning so a
benchmark workflow that did not produce the report (for example, when the
prior steps were skipped) does not fail the job.
"""

from __future__ import annotations

import argparse
import base64
import html
import os
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, List, Sequence


REPO_ROOT = Path(__file__).resolve().parents[1]
DEFAULT_INPUT = REPO_ROOT / "accuracy_report.md"
DEFAULT_FIGURES_DIR = REPO_ROOT / "docs" / "figures"

# GitHub Actions caps the step summary at 1 MiB per step.  We keep a small
# safety margin for the trailing scaffolding GitHub injects around the
# rendered Markdown.
GITHUB_STEP_SUMMARY_LIMIT_BYTES = 1024 * 1024
DEFAULT_MAX_SUMMARY_BYTES = GITHUB_STEP_SUMMARY_LIMIT_BYTES - 16 * 1024

_PNG_LINK_RE = re.compile(r"\[([^\]]+)\]\(([^)\s]+\.png)\)")


# ---------------------------------------------------------------------------
# Image embedding


def encode_png_data_uri(path: Path) -> str:
    """Return ``data:image/png;base64,...`` for the bytes at ``path``."""

    encoded = base64.b64encode(path.read_bytes()).decode("ascii")
    return f"data:image/png;base64,{encoded}"


def find_image_references(markdown_text: str) -> List[str]:
    """Return PNG references appearing as Markdown links, deduplicated."""

    refs = _PNG_LINK_RE.findall(markdown_text)
    return list(dict.fromkeys(url for _label, url in refs))


def build_image_uris(
    refs: Iterable[str],
    *,
    repo_root: Path,
    figures_dir: Path,
) -> dict[str, str]:
    """Resolve each reference and encode the PNG as a base64 data URI.

    References are first resolved relative to ``repo_root``; if missing,
    the basename is looked up in ``figures_dir``.  References that cannot
    be resolved are silently omitted so callers can detect them by
    diffing against the input.
    """

    out: dict[str, str] = {}
    for ref in refs:
        for candidate in (
            (repo_root / ref).resolve(),
            (figures_dir / Path(ref).name).resolve(),
        ):
            if candidate.is_file():
                out[ref] = encode_png_data_uri(candidate)
                break
    return out


# ---------------------------------------------------------------------------
# Summary rendering


def _embed_block(label: str, data_uri: str) -> str:
    """Return a ``<details>`` block embedding ``data_uri`` as an image."""

    safe_label = html.escape(label)
    safe_uri = html.escape(data_uri, quote=True)
    return (
        f"<details><summary>{safe_label}</summary>"
        f'<p><img src="{safe_uri}" alt="{safe_label}" '
        f'style="max-width:100%;"/></p>'
        f"</details>"
    )


def _replace_png_links(
    markdown_text: str,
    image_uris: dict[str, str],
    missing: list[str],
) -> str:
    """Rewrite ``[label](*.png)`` to inline data-URI ``<details>`` blocks."""

    def repl(match: re.Match[str]) -> str:
        label = match.group(1)
        url = match.group(2)
        data_uri = image_uris.get(url)
        if data_uri is None:
            missing.append(url)
            return match.group(0)
        return _embed_block(label, data_uri)

    return _PNG_LINK_RE.sub(repl, markdown_text)


def render_summary_markdown(
    markdown_text: str,
    *,
    repo_root: Path,
    figures_dir: Path,
) -> tuple[str, "RenderStats"]:
    """Return ``(summary_markdown, stats)`` for ``markdown_text``."""

    refs = find_image_references(markdown_text)
    image_uris = build_image_uris(
        refs, repo_root=repo_root, figures_dir=figures_dir
    )
    missing: list[str] = []
    rewritten = _replace_png_links(markdown_text, image_uris, missing)
    stats = RenderStats(
        embedded_images=len(image_uris),
        referenced_images=len(refs),
        missing_image_refs=list(dict.fromkeys(missing)),
    )
    return rewritten, stats


@dataclass
class RenderStats:
    embedded_images: int
    referenced_images: int
    missing_image_refs: list[str]


# ---------------------------------------------------------------------------
# Top-level orchestration


@dataclass
class GenerateResult:
    summary_path: Path | None
    embedded_images: int
    referenced_images: int
    missing_image_refs: list[str]
    bytes_written: int
    truncated: bool
    skipped: bool = False
    skip_reason: str = ""


def _truncate_if_needed(
    text: str, max_bytes: int
) -> tuple[str, bool]:
    """Return ``(possibly_truncated_text, was_truncated)`` under ``max_bytes``."""

    encoded = text.encode("utf-8")
    if len(encoded) <= max_bytes:
        return text, False
    notice = (
        "\n\n> **Note:** Accuracy report truncated to fit the GitHub "
        "Actions step-summary size limit. See the uploaded "
        "`accuracy_report.md` artifact for the full report.\n"
    )
    notice_bytes = notice.encode("utf-8")
    budget = max_bytes - len(notice_bytes)
    if budget <= 0:
        return notice.lstrip(), True
    # Trim on a UTF-8 boundary by decoding with errors='ignore'.
    trimmed = encoded[:budget].decode("utf-8", errors="ignore")
    return trimmed + notice, True


def generate(
    *,
    input_path: Path = DEFAULT_INPUT,
    figures_dir: Path = DEFAULT_FIGURES_DIR,
    summary_path: Path | None,
    repo_root: Path = REPO_ROOT,
    max_bytes: int = DEFAULT_MAX_SUMMARY_BYTES,
) -> GenerateResult:
    """Render ``input_path`` and append the result to ``summary_path``.

    If ``input_path`` does not exist, returns a ``GenerateResult`` with
    ``skipped=True`` and writes nothing.  Callers translate that into a
    zero exit code so CI does not fail when the report was not produced.
    """

    if not input_path.is_file():
        return GenerateResult(
            summary_path=summary_path,
            embedded_images=0,
            referenced_images=0,
            missing_image_refs=[],
            bytes_written=0,
            truncated=False,
            skipped=True,
            skip_reason=f"accuracy report not found: {input_path}",
        )

    markdown_text = input_path.read_text(encoding="utf-8")
    rendered, stats = render_summary_markdown(
        markdown_text, repo_root=repo_root, figures_dir=figures_dir
    )
    payload, truncated = _truncate_if_needed(rendered, max_bytes)

    bytes_written = 0
    if summary_path is not None:
        summary_path.parent.mkdir(parents=True, exist_ok=True)
        with summary_path.open("a", encoding="utf-8") as fh:
            # Always start the appended block on its own line.
            fh.write("\n")
            fh.write(payload)
            if not payload.endswith("\n"):
                fh.write("\n")
        bytes_written = len(payload.encode("utf-8")) + 1

    return GenerateResult(
        summary_path=summary_path,
        embedded_images=stats.embedded_images,
        referenced_images=stats.referenced_images,
        missing_image_refs=stats.missing_image_refs,
        bytes_written=bytes_written,
        truncated=truncated,
    )


def _resolve_summary_path(value: str | None) -> Path | None:
    if value is None or value == "":
        env_value = os.environ.get("GITHUB_STEP_SUMMARY", "")
        if not env_value:
            return None
        return Path(env_value)
    return Path(value)


def _parse_args(argv: Sequence[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Append the accuracy report (with base64-embedded charts) to "
            "GITHUB_STEP_SUMMARY so it renders on the Actions run page."
        )
    )
    parser.add_argument(
        "--input", type=Path, default=DEFAULT_INPUT,
        help="Path to accuracy_report.md (default: repo root).",
    )
    parser.add_argument(
        "--figures-dir", type=Path, default=DEFAULT_FIGURES_DIR,
        help="Fallback directory to search for chart PNGs by basename.",
    )
    parser.add_argument(
        "--summary-file", default=None,
        help=(
            "Destination summary file. Defaults to the value of the "
            "GITHUB_STEP_SUMMARY environment variable. If neither is set "
            "the rendered summary is written to stdout."
        ),
    )
    parser.add_argument(
        "--repo-root", type=Path, default=REPO_ROOT,
        help="Repository root used to resolve relative image references.",
    )
    parser.add_argument(
        "--max-bytes", type=int, default=DEFAULT_MAX_SUMMARY_BYTES,
        help=(
            "Soft cap on the rendered summary size (UTF-8 bytes). The "
            "GitHub Actions hard limit is 1 MiB per step."
        ),
    )
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> int:
    args = _parse_args(argv)
    summary_path = _resolve_summary_path(args.summary_file)

    if summary_path is None and args.input.is_file():
        # Fall back to stdout so local users can preview the rendered
        # summary without setting GITHUB_STEP_SUMMARY.
        markdown_text = args.input.read_text(encoding="utf-8")
        rendered, stats = render_summary_markdown(
            markdown_text,
            repo_root=args.repo_root,
            figures_dir=args.figures_dir,
        )
        payload, truncated = _truncate_if_needed(rendered, args.max_bytes)
        sys.stdout.write(payload)
        if not payload.endswith("\n"):
            sys.stdout.write("\n")
        print(
            f"# rendered to stdout (embedded {stats.embedded_images} of "
            f"{stats.referenced_images} chart images; "
            f"{len(stats.missing_image_refs)} unresolved; "
            f"truncated={truncated})",
            file=sys.stderr,
        )
        return 0

    result = generate(
        input_path=args.input,
        figures_dir=args.figures_dir,
        summary_path=summary_path,
        repo_root=args.repo_root,
        max_bytes=args.max_bytes,
    )

    if result.skipped:
        print(f"warning: {result.skip_reason}; nothing written", file=sys.stderr)
        return 0

    print(
        f"wrote {result.bytes_written} bytes to {result.summary_path} "
        f"(embedded {result.embedded_images} of {result.referenced_images} "
        f"chart images; {len(result.missing_image_refs)} unresolved; "
        f"truncated={result.truncated})"
    )
    if result.missing_image_refs:
        for ref in result.missing_image_refs:
            print(f"  unresolved: {ref}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
