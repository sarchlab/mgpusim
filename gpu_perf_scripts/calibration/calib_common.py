#!/usr/bin/env python3
"""Shared helpers for the benchmark-agnostic MI300A calibration pipeline.

A run is identified by (benchmark, scaling point, non-scaling combo). The
non-scaling combo is carried verbatim as a JSON string (`non_scaling_json`) in
both the ground-truth CSV and the sweep CSVs; these helpers turn it into the
human label / id-safe slug used by the report, the Firestore points, and the
PNG filenames -- so all of them agree.
"""
import json
import re


def parse_ns(s):
    """Parse a non_scaling_json string into a dict (tolerant of blanks/garbage)."""
    if not s:
        return {}
    try:
        d = json.loads(s)
        return d if isinstance(d, dict) else {}
    except (ValueError, TypeError):
        return {}


def ns_label(ns):
    """Human-readable non-scaling combo, e.g. 'num_blocks=1, threads_per_block=1024'."""
    if not ns:
        return "(default)"
    return ", ".join(f"{k}={ns[k]}" for k in sorted(ns))


def ns_slug(ns):
    """Stable id-safe slug for a non-scaling combo (PNG name / Firestore doc id)."""
    if not ns:
        return "default"
    return re.sub(r"[^A-Za-z0-9_.-]", "_", "_".join(f"{k}-{ns[k]}" for k in sorted(ns)))


def xparse(v):
    """Parse a scaling value to a number (int when integral) for axis + matching."""
    f = float(v)
    return int(f) if f.is_integer() else f
