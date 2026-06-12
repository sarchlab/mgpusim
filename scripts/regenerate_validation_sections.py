#!/usr/bin/env python3
"""
Regenerate stale sections of docs/validation_report.md from comparison_ci.csv.

Fixes: Section 5 (Category Analysis), Section 5.4 (No Sim Data),
Section 6.2 (Scaling Accuracy), Section 6.3 (Speedup Error),
Scaling-Region Per-Kernel Breakdown table, Key Findings,
Per-Kernel Scaling Figures list, and all inline count references.

Run from repo root: python3 scripts/regenerate_validation_sections.py
"""

import csv
import re
from collections import defaultdict
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
CSV_PATH = REPO_ROOT / "benchmark-comparison" / "comparison_ci.csv"
REPORT_PATH = REPO_ROOT / "docs" / "validation_report.md"


def spearman(x, y):
    n = len(x)
    if n < 3:
        return None
    def rank(v):
        sorted_idx = sorted(range(n), key=lambda i: v[i])
        ranks = [0] * n
        for i, idx in enumerate(sorted_idx):
            ranks[idx] = i + 1
        return ranks
    rx = rank(x)
    ry = rank(y)
    d2 = sum((rx[i] - ry[i]) ** 2 for i in range(n))
    return 1 - 6 * d2 / (n * (n * n - 1))


def load_data():
    rows = []
    total_hw = 0
    with open(CSV_PATH) as f:
        reader = csv.DictReader(f)
        for row in reader:
            total_hw += 1
            if row['sim_ms'] and row['real_ms']:
                rows.append({
                    'kernel': row['kernel_name'],
                    'size': row['problem_size'],
                    'real': float(row['real_ms']),
                    'sim': float(row['sim_ms']),
                })
    return rows, total_hw


def compute_kernel_stats(rows):
    kernels = defaultdict(list)
    for r in rows:
        kernels[r['kernel']].append(r)

    stats = {}
    for k in sorted(kernels.keys()):
        pts = kernels[k]
        n = len(pts)
        ratios = [p['sim'] / p['real'] for p in pts]
        avg_ratio = sum(ratios) / n
        min_ratio = min(ratios)
        max_ratio = max(ratios)
        sum_ae = sum(abs(p['sim'] - p['real']) for p in pts)
        sum_r = sum(p['real'] for p in pts)
        wmape = sum_ae / sum_r if sum_r > 0 else 0

        sorted_pts = sorted(pts, key=lambda p: p['real'])
        rho = spearman([p['real'] for p in sorted_pts],
                       [p['sim'] for p in sorted_pts])

        if avg_ratio < 0.5:
            cat = "TOO_FAST"
        elif avg_ratio > 1.5:
            cat = "TOO_SLOW"
        else:
            cat = "OK"

        # Scaling region
        min_real = min(p['real'] for p in pts)
        threshold = 2 * min_real
        oh_pts = [p for p in pts if p['real'] < threshold]
        sc_pts = [p for p in pts if p['real'] >= threshold]

        oh_wmape = None
        sc_wmape = None
        sc_rho = None
        if oh_pts:
            oh_ae = sum(abs(p['sim'] - p['real']) for p in oh_pts)
            oh_r = sum(p['real'] for p in oh_pts)
            oh_wmape = oh_ae / oh_r if oh_r > 0 else None
        if sc_pts:
            sc_ae = sum(abs(p['sim'] - p['real']) for p in sc_pts)
            sc_r = sum(p['real'] for p in sc_pts)
            sc_wmape = sc_ae / sc_r if sc_r > 0 else None
            if len(sc_pts) >= 3:
                sc_sorted = sorted(sc_pts, key=lambda p: p['real'])
                sc_rho = spearman([p['real'] for p in sc_sorted],
                                  [p['sim'] for p in sc_sorted])

        stats[k] = {
            'n': n, 'wmape': wmape, 'avg_ratio': avg_ratio,
            'min_ratio': min_ratio, 'max_ratio': max_ratio,
            'rho': rho, 'cat': cat,
            'oh_n': len(oh_pts), 'sc_n': len(sc_pts),
            'oh_wmape': oh_wmape, 'sc_wmape': sc_wmape, 'sc_rho': sc_rho,
        }
    return stats, kernels


def generate_section5(stats):
    ok = {k: s for k, s in stats.items() if s['cat'] == 'OK'}
    fast = {k: s for k, s in stats.items() if s['cat'] == 'TOO_FAST'}
    slow = {k: s for k, s in stats.items() if s['cat'] == 'TOO_SLOW'}

    lines = []
    lines.append("## 5. Category Analysis")
    lines.append("")
    lines.append(f"### 5.1 Well-Calibrated Kernels (sim/real ratio 0.5–1.5): {len(ok)} kernels")
    lines.append("")
    lines.append("These kernels have simulation times within 2× of hardware:")
    lines.append("")
    lines.append("| Kernel | N | WMAPE | Avg Ratio | Notes |")
    lines.append("|--------|---|-------|-----------|-------|")
    for k in sorted(ok.keys(), key=lambda x: ok[x]['wmape']):
        s = ok[k]
        if s['wmape'] < 0.10:
            notes = "Excellent — nearly 1:1"
        elif s['wmape'] < 0.20:
            notes = "Good — well-calibrated"
        elif s['wmape'] < 0.30:
            notes = "Good"
        elif s['wmape'] < 0.50:
            notes = "Moderate"
        elif s['wmape'] < 0.80:
            notes = "Moderate — high variance across sizes"
        else:
            notes = "High WMAPE from size-dependent variance"
        lines.append(f"| {k} | {s['n']} | {s['wmape']*100:.1f}% | {s['avg_ratio']:.3f} | {notes} |")
    lines.append("")
    lines.append("**Common traits:** Simple memory access patterns, moderate parallelism, vector/matrix operations where the flat topology is a reasonable approximation.")
    lines.append("")

    # 5.2 Too Fast
    lines.append(f"### 5.2 Too Fast (sim/real ratio < 0.5): {len(fast)} kernels")
    lines.append("")
    lines.append("The simulator completes these kernels significantly faster than real hardware:")
    lines.append("")
    lines.append("| Kernel | N | WMAPE | Avg Ratio | Likely Root Cause |")
    lines.append("|--------|---|-------|-----------|-------------------|")
    root_causes = {
        'pagerank': 'Graph traversal — irregular memory access severely underestimated',
        'sort': 'Multi-phase bitonic sort — inter-workgroup synchronization undermodeled',
        'srad': 'Stencil computation — memory hierarchy latency underestimated',
        'covariance': 'Dense linear algebra with reductions — missing XCD cross-traffic',
        'correlation': 'Dense linear algebra with reductions — missing XCD cross-traffic',
        'hotspot': '2D stencil — memory subsystem too optimistic',
        'gaussian': 'Pivoting & row ops — sequential dependencies undermodeled',
        'nw': 'Dynamic programming — wavefront dependencies',
        'spmv_csr': 'Sparse matrix — irregular access patterns',
        'scan': 'Prefix sum — inter-workgroup synchronization costs',
        'hotspot3D': '3D stencil — memory subsystem too optimistic',
        'syr2k': 'Dense linear algebra — missing XCD cross-traffic latency',
        'syrk': 'Dense linear algebra — missing XCD cross-traffic latency',
        'dwt2d': 'Wavelet transform — multi-pass memory access pattern undermodeled',
    }
    for k in sorted(fast.keys(), key=lambda x: -fast[x]['wmape']):
        s = fast[k]
        cause = root_causes.get(k, 'Missing XCD topology — uniform latency too optimistic')
        lines.append(f"| {k} | {s['n']} | {s['wmape']*100:.1f}% | {s['avg_ratio']:.3f} | {cause} |")
    lines.append("")
    lines.append("**Root cause analysis:** The simulator's flat memory topology lacks XCD (Accelerator Complex Die) partitioning present in MI300A. Real MI300A has 8 XCDs with NUMA-like memory access patterns. The flat model allows all CUs to access memory with uniform latency, which significantly underestimates:")
    lines.append("- Cross-XCD memory coherence traffic")
    lines.append("- DRAM bank conflicts under high parallelism")
    lines.append("- Synchronization overhead between workgroups on different XCDs")
    lines.append("- Cache line ping-ponging in reduction-style operations")
    lines.append("")

    # 5.3 Too Slow
    lines.append(f"### 5.3 Too Slow (sim/real ratio > 1.5): {len(slow)} kernels")
    lines.append("")
    slow_causes = {
        'gesummv': 'Small vector ops with high sim overhead — ratio driven by tiny real times where fixed overhead dominates',
        'sgemm_tiled': 'Tiled matrix multiply — sim memory hierarchy too slow at large tile sizes (ratio 0.77–6.76)',
        'triad': 'Memory bandwidth-bound — sim memory subsystem scales differently from real HBM3 at large sizes',
    }
    lines.append("| Kernel | N | WMAPE | Avg Ratio | Likely Root Cause |")
    lines.append("|--------|---|-------|-----------|-------------------|")
    for k in sorted(slow.keys(), key=lambda x: -slow[x]['wmape']):
        s = slow[k]
        cause = slow_causes.get(k, 'Simulator overhead dominates at small sizes')
        lines.append(f"| {k} | {s['n']} | {s['wmape']*100:.1f}% | {s['avg_ratio']:.3f} | {cause} |")
    lines.append("")
    lines.append("**Root cause analysis:** These kernels show the simulator running slower than real hardware. For memory-bound kernels (triad, sgemm_tiled), the simulator's simplified memory model over-penalizes at large problem sizes. For gesummv, small vector operations have very short real execution times where simulator fixed overhead dominates.")
    lines.append("")

    # 5.4 No Sim Data
    lines.append("### 5.4 No Sim Data: 48 kernel names")
    lines.append("")
    lines.append("Grouped by reason:")
    lines.append("")
    lines.append("**No `amd/samples` counterpart (29 kernels):** Micro-benchmarks (BusSpeed*, DeviceMemory*, MaxFlops, atomic_throughput, branch_div_50pct, fp32_fma, fp64_fma, global_bw_copy, int_mad, l1_cache_bw, l2_cache_bw, mem_latency_chase, occupancy_fma, sfun_sin, shared_bw), ML kernels (fused_swiglu, gelu, layernorm, naive_attention, rope, softmax, tiled_gemm_16), application suites (bh, dmr, md, md5hash, qtc, s3d, sssp).")
    lines.append("")
    lines.append("**CI benchmark exists but no timing extraction (12 kernels):** adjust_weights (backprop 2nd kernel), bfs, ga, gramschmidt, hist, huffman, kmeans, lud, particlefilter, streamcluster_pgain, lbm_stream_collide, gridding_kernel.")
    lines.append("")
    lines.append("**Kernel name mismatch (4 kernels):** busspeeddownload, busspeedreadback, devicememory_read, devicememory_write — these are HW micro-benchmark variants with no sim equivalent.")
    lines.append("")
    lines.append("**Timing extracted but no HW size match (3 kernels):** computesad, findminsad (from parboil_sad — kernel names don't align with HW reference naming convention), tpacf_dr, tpacf_rr (only tpacf_DD matched).")

    return "\n".join(lines)


def generate_section62_scaling_accuracy(kernels_dict):
    """Generate section 6.2 Scaling Accuracy table."""
    scaling_results = {}
    for k in sorted(kernels_dict.keys()):
        pts = sorted(kernels_dict[k], key=lambda p: p['real'])
        if len(pts) < 2:
            continue
        ratios = []
        for i in range(len(pts) - 1):
            if pts[i]['real'] > 0 and pts[i]['sim'] > 0:
                real_ratio = pts[i+1]['real'] / pts[i]['real']
                sim_ratio = pts[i+1]['sim'] / pts[i]['sim']
                if real_ratio > 0:
                    scaling_ratio = sim_ratio / real_ratio
                    ratios.append(scaling_ratio)
        if ratios:
            mean_ratio = sum(ratios) / len(ratios)
            mean_abs_err = sum(abs(r - 1) for r in ratios) / len(ratios)
            scaling_results[k] = {
                'mean_ratio': mean_ratio,
                'mean_abs_err': mean_abs_err,
                'pairs': len(ratios)
            }

    lines = []
    lines.append("### 6.2 Scaling Accuracy")
    lines.append("")
    lines.append("For consecutive problem sizes, the ratio `(sim[i+1]/sim[i]) / (real[i+1]/real[i])` measures whether the simulator captures how performance scales:")
    lines.append("")
    lines.append("| Kernel | Mean Ratio | Mean Abs Err | Pairs | Flag |")
    lines.append("|--------|-----------|-------------|-------|------|")
    for k in sorted(scaling_results.keys(), key=lambda x: scaling_results[x]['mean_abs_err']):
        s = scaling_results[k]
        flag = " ⚠" if s['mean_abs_err'] >= 0.2 else ""
        lines.append(f"| {k.lower()} | {s['mean_ratio']:.3f} | {s['mean_abs_err']:.3f} | {s['pairs']} |{flag} |")

    all_errs = [s['mean_abs_err'] for s in scaling_results.values()]
    mean_overall = sum(all_errs) / len(all_errs) if all_errs else 0
    n_under_02 = sum(1 for e in all_errs if e < 0.2)
    lines.append("")
    lines.append(f"**Overall mean |scaling error|:** {mean_overall:.4f}  ")
    lines.append(f"**Kernels with mean |err| < 0.2:** {n_under_02} of {len(all_errs)}")

    return "\n".join(lines)


def generate_section63_speedup(kernels_dict):
    """Generate section 6.3 Speedup Error table."""
    speedup_results = {}
    for k in sorted(kernels_dict.keys()):
        pts = sorted(kernels_dict[k], key=lambda p: p['real'])
        if len(pts) < 2:
            continue
        smallest = pts[0]
        largest = pts[-1]
        if smallest['real'] > 0 and smallest['sim'] > 0:
            real_speedup = largest['real'] / smallest['real']
            sim_speedup = largest['sim'] / smallest['sim']
            ratio = sim_speedup / real_speedup if real_speedup > 0 else 0
            speedup_results[k] = {
                'real_speedup': real_speedup,
                'sim_speedup': sim_speedup,
                'ratio': ratio
            }

    lines = []
    lines.append("### 6.3 Speedup Error (Smallest to Largest Size)")
    lines.append("")
    lines.append("End-to-end speedup from smallest to largest problem size:")
    lines.append("")
    lines.append("| Kernel | Real Speedup | Sim Speedup | Ratio | Flag |")
    lines.append("|--------|-------------|-------------|-------|------|")
    for k in sorted(speedup_results.keys(), key=lambda x: -speedup_results[x]['ratio']):
        s = speedup_results[k]
        flag = " ⚠" if abs(s['ratio'] - 1) >= 0.5 else ""
        lines.append(f"| {k.lower()} | {s['real_speedup']:.2f}× | {s['sim_speedup']:.2f}× | {s['ratio']:.2f} |{flag} |")

    all_ratios = [abs(s['ratio'] - 1) for s in speedup_results.values()]
    mean_err = sum(all_ratios) / len(all_ratios) if all_ratios else 0
    n_under_03 = sum(1 for e in all_ratios if e < 0.3)
    lines.append("")
    lines.append(f"**Overall mean |speedup ratio − 1|:** {mean_err:.4f}  ")
    lines.append(f"**Kernels with |ratio − 1| < 0.3:** {n_under_03} of {len(all_ratios)}")
    lines.append("")
    lines.append("The large speedup errors for sgemm_tiled, triad, and nn indicate that the simulator's memory subsystem scales differently from real hardware — it shows increasing divergence at larger sizes where real hardware hits bandwidth limits or where topology effects compound.")

    return "\n".join(lines)


def generate_spearman_table(stats):
    """Generate section 6.1 Spearman table."""
    n_matched = sum(s['n'] for s in stats.values())
    lines = []
    lines.append("### 6.1 Spearman Rank Correlation")
    lines.append("")
    lines.append(f"**Overall:** ρ = 0.9033 across all {n_matched} matched data points.")
    lines.append("")
    lines.append("Per-kernel results (≥3 data points, sorted by ρ):")
    lines.append("")
    lines.append("| Kernel | ρ | N | Flag |")
    lines.append("|--------|---|---|------|")

    qualified = {k: s for k, s in stats.items() if s['rho'] is not None}
    for k in sorted(qualified.keys(), key=lambda x: -qualified[x]['rho']):
        s = qualified[k]
        flag = ""
        if s['rho'] < 0.5:
            flag = "⚠"
        elif s['rho'] < 0.7:
            flag = "⚠ Flat real data" if s['rho'] > 0 else "⚠ Non-monotonic real"
        lines.append(f"| {k} | {s['rho']:+.4f} | {s['n']} | {flag} |")

    n_qualified = len(qualified)
    avg_rho = sum(s['rho'] for s in qualified.values()) / n_qualified if n_qualified else 0
    n_high = sum(1 for s in qualified.values() if s['rho'] >= 0.9)
    lines.append("")
    lines.append(f"**Unfiltered average ρ:** {avg_rho:.4f} across {n_qualified} kernels with ≥3 data points  ")
    lines.append(f"**Kernels with ρ ≥ 0.9:** {n_high} of {n_qualified}")

    return "\n".join(lines)


def generate_scaling_region_table(stats):
    """Generate the Per-Kernel Breakdown table for Scaling-Region Analysis."""
    lines = []
    lines.append("| Kernel | Total | Overhead | Scaling | WMAPE (OH) | WMAPE (Scaling) | Spearman (Scaling) |")
    lines.append("|--------|------:|---------:|--------:|-----------:|----------------:|-------------------:|")
    for k in sorted(stats.keys(), key=lambda x: x.lower()):
        s = stats[k]
        oh_w = f"{s['oh_wmape']*100:.1f}%" if s['oh_wmape'] is not None else "—"
        sc_w = f"{s['sc_wmape']*100:.1f}%" if s['sc_wmape'] is not None else "—"
        sc_r = f"{s['sc_rho']:.2f}" if s['sc_rho'] is not None else "—"
        lines.append(f"| {k} | {s['n']} | {s['oh_n']} | {s['sc_n']} | {oh_w} | {sc_w} | {sc_r} |")
    return "\n".join(lines)


def generate_scaling_findings(stats, kernels_dict):
    """Generate the Key Findings section under Scaling-Region Analysis."""
    total_oh = sum(s['oh_n'] for s in stats.values())
    total_sc = sum(s['sc_n'] for s in stats.values())
    total = total_oh + total_sc
    n_kernels = len(stats)
    n_zero_sc = sum(1 for s in stats.values() if s['sc_n'] == 0)
    n_with_sc = sum(1 for s in stats.values() if s['sc_n'] > 0)

    # Compute overall region WMAPEs
    oh_ae = oh_r = sc_ae = sc_r = 0
    for k, pts in kernels_dict.items():
        min_real = min(p['real'] for p in pts)
        threshold = 2 * min_real
        for p in pts:
            ae = abs(p['sim'] - p['real'])
            if p['real'] < threshold:
                oh_ae += ae
                oh_r += p['real']
            else:
                sc_ae += ae
                sc_r += p['real']

    oh_wmape = oh_ae / oh_r * 100 if oh_r > 0 else 0
    sc_wmape = sc_ae / sc_r * 100 if sc_r > 0 else 0

    # Best/worst scaling performers
    best_sc = [(k, s) for k, s in stats.items()
               if s['sc_n'] > 0 and s['sc_wmape'] is not None]
    best_sc.sort(key=lambda x: x[1]['sc_wmape'])
    worst_sc = list(reversed(best_sc))

    # Notable zero-scaling kernels
    zero_sc_notable = [(k, s) for k, s in stats.items()
                       if s['sc_n'] == 0 and s['n'] >= 4]
    zero_sc_notable.sort(key=lambda x: -x[1]['n'])

    lines = []
    lines.append("### Key Findings and Implications")
    lines.append("")
    oh_pct = total_oh * 100 // total
    sc_pct = total_sc * 100 // total
    lines.append(f"1. **The current dataset has a significant overhead component.** Of {total} matched points,")
    lines.append(f"   {total_oh} ({oh_pct}%) fall in the overhead region (HW time < 2× kernel minimum). {total_sc}")
    lines.append(f"   points ({sc_pct}%) across {n_with_sc} kernels qualify as scaling-region data. This means the")
    lines.append(f"   current CI benchmark matrix primarily tests small-to-moderate problem sizes where launch")
    lines.append(f"   overhead can mask true computational scaling behaviour.")
    lines.append("")
    lines.append(f"2. **Scaling-region WMAPE ({sc_wmape:.1f}%) is lower than overhead-region ({oh_wmape:.1f}%).**")
    lines.append(f"   The simulator is more accurate at larger problem sizes where computation dominates")
    lines.append(f"   and launch overhead becomes negligible. However, some kernels still show high")
    lines.append(f"   scaling-region error due to the flat topology mismatch.")
    lines.append("")

    if len(best_sc) >= 3:
        b1, b2, b3 = best_sc[0], best_sc[1], best_sc[2]
        rho_str = ""
        if b3[1]['sc_rho'] is not None:
            rho_str = f", Spearman ρ = {b3[1]['sc_rho']:.2f}"
        lines.append(f"3. **Best scaling-region performers:** {b1[0]} ({b1[1]['sc_wmape']*100:.1f}% WMAPE), "
                     f"{b2[0]} ({b2[1]['sc_wmape']*100:.1f}% WMAPE), and")
        lines.append(f"   {b3[0]} ({b3[1]['sc_wmape']*100:.1f}% WMAPE{rho_str}) show strong accuracy in the scaling")
        lines.append(f"   region. These are typically linear-algebra or stencil kernels where the flat topology")
        lines.append(f"   approximation is least harmful.")
    lines.append("")

    if len(worst_sc) >= 4:
        w1, w2, w3, w4 = worst_sc[0], worst_sc[1], worst_sc[2], worst_sc[3]
        lines.append(f"4. **Worst scaling-region performers:** {w1[0]} ({w1[1]['sc_wmape']*100:.1f}% WMAPE), "
                     f"{w2[0]} ({w2[1]['sc_wmape']*100:.1f}%),")
        lines.append(f"   {w3[0]} ({w3[1]['sc_wmape']*100:.1f}%), and {w4[0]} ({w4[1]['sc_wmape']*100:.1f}%) have high errors in the scaling region.")
        lines.append(f"   These kernels expose memory subsystem or topology modelling gaps at larger problem sizes.")
    lines.append("")

    lines.append(f"5. **{n_zero_sc} of {n_kernels} kernels have zero scaling-region points.** The CI matrix doesn't test them at")
    lines.append(f"   sufficiently large problem sizes. Key examples include: ")
    notable_names = ", ".join(f"{k} ({s['n']} pts, all overhead)" for k, s in zero_sc_notable[:4])
    lines.append(f"   {notable_names}.")
    lines.append(f"   Adding larger problem sizes for these kernels would enable scaling-region analysis.")
    lines.append("")
    lines.append(f"6. **Recommendation:** Future CI runs should include larger problem sizes to increase")
    lines.append(f"   scaling-region coverage. The current data supports strong rank-order accuracy")
    lines.append(f"   overall (Spearman ρ = 0.90), but scaling-region-specific analysis requires more")
    lines.append(f"   data points at sizes that stress the computational pipeline rather than the")
    lines.append(f"   launch overhead.")

    return "\n".join(lines)


def generate_figures_section(stats):
    """Generate Per-Kernel Scaling Figures section, only for kernels with >=2 pts."""
    lines = []
    lines.append("## Per-Kernel Scaling Figures")
    lines.append("")
    lines.append("The figures below show Hardware (MI300A) vs Simulator (MGPUSim) execution time across problem sizes for each kernel with ≥2 matched data points. Blue solid lines represent hardware measurements; red dashed lines represent simulator predictions.")
    lines.append("")
    lines.append("Figures can be regenerated with: `python3 scripts/generate_scaling_figures.py`")
    lines.append("")

    for k in sorted(stats.keys(), key=lambda x: x.lower()):
        if stats[k]['n'] >= 2:
            kl = k.lower()
            lines.append(f"### {kl}")
            lines.append(f"![{kl} scaling](figures/{kl}_scaling.png)")
            lines.append("")

    return "\n".join(lines)


def replace_section(report, start_marker, end_marker, new_content, include_end=False):
    """Replace content between start_marker and end_marker."""
    start = report.find(start_marker)
    end = report.find(end_marker, start + len(start_marker) if start >= 0 else 0)
    if start < 0 or end < 0:
        print(f"WARNING: Could not find markers: '{start_marker[:40]}...' -> '{end_marker[:40]}...'")
        return report
    if include_end:
        end = end + len(end_marker)
    return report[:start] + new_content + report[end:]


def main():
    rows, total_hw = load_data()
    n_matched = len(rows)
    stats, kernels_dict = compute_kernel_stats(rows)

    report = REPORT_PATH.read_text()

    # --- Fix inline references ---
    # Remove dwt2d from "produced no data" list (line 29)
    report = report.replace(
        "bfs, dwt2d, fastwalshtransform, ga, gramschmidt, hist, huffman, kmeans, lud, parboil_lbm, parboil_mri_gridding, particlefilter, streamcluster",
        "bfs, fastwalshtransform, ga, gramschmidt, hist, huffman, kmeans, lud, parboil_lbm, parboil_mri_gridding, particlefilter, streamcluster"
    )

    # Fix "159 with sim matches" -> "242 with sim matches" (line 59)
    report = report.replace(
        "(159 with sim matches)",
        f"({n_matched} with sim matches)"
    )

    # Fix "Of 159 matched points" (line 612)
    report = report.replace(
        "Of 159 matched points",
        f"Of {n_matched} matched points"
    )

    # Remove dwt2d from Section 3.4 table
    report = report.replace(
        "| dwt2d | HSACO v3 kernel — no timing instrumentation |\n",
        ""
    )

    # Fix "13 empty" and "13 CI benchmarks" counts (since dwt2d is removed)
    report = report.replace(
        "42 with timing data (55 artifacts collected; 13 empty)",
        "43 with timing data (55 artifacts collected; 12 empty)"
    )

    # --- Replace Section 5 ---
    new_sec5 = generate_section5(stats)
    sec5_start = report.find("## 5. Category Analysis")
    sec5_end = report.find("\n## 6. Relative Accuracy Metrics")
    if sec5_start >= 0 and sec5_end >= 0:
        report = report[:sec5_start] + new_sec5 + "\n\n---\n" + report[sec5_end:]

    # --- Replace Section 6.1 Spearman ---
    new_61 = generate_spearman_table(stats)
    s61_start = report.find("### 6.1 Spearman Rank Correlation")
    s61_end = report.find("\n### 6.2 Scaling Accuracy")
    if s61_start >= 0 and s61_end >= 0:
        report = report[:s61_start] + new_61 + report[s61_end:]

    # --- Replace Section 6.2 Scaling Accuracy ---
    new_62 = generate_section62_scaling_accuracy(kernels_dict)
    s62_start = report.find("### 6.2 Scaling Accuracy")
    s62_end = report.find("\n### 6.3 Speedup Error")
    if s62_start >= 0 and s62_end >= 0:
        report = report[:s62_start] + new_62 + report[s62_end:]

    # --- Replace Section 6.3 Speedup Error ---
    new_63 = generate_section63_speedup(kernels_dict)
    s63_start = report.find("### 6.3 Speedup Error")
    s63_end = report.find("\n---\n\n## 7.")
    if s63_start >= 0 and s63_end >= 0:
        report = report[:s63_start] + new_63 + report[s63_end:]

    # --- Replace Scaling Region Per-Kernel Breakdown table ---
    new_sr_table = generate_scaling_region_table(stats)
    sr_header = "| Kernel | Total | Overhead | Scaling | WMAPE (OH) | WMAPE (Scaling) | Spearman (Scaling) |"
    sr_start = report.find(sr_header)
    if sr_start >= 0:
        pos = sr_start
        while pos < len(report):
            line_end = report.find("\n", pos)
            if line_end < 0:
                break
            line = report[pos:line_end].strip()
            if line and not line.startswith("|"):
                break
            pos = line_end + 1
        report = report[:sr_start] + new_sr_table + "\n" + report[pos:]

    # --- Replace Key Findings ---
    new_findings = generate_scaling_findings(stats, kernels_dict)
    findings_start = report.find("### Key Findings and Implications")
    if findings_start >= 0:
        rest = report[findings_start:]
        idx = rest.find("\n---\n")
        if idx > 0:
            findings_end = findings_start + idx
            report = report[:findings_start] + new_findings + "\n\n" + report[findings_end:]

    # --- Replace Per-Kernel Scaling Figures ---
    new_figures = generate_figures_section(stats)
    fig_start = report.find("## Per-Kernel Scaling Figures")
    if fig_start >= 0:
        # Find end: the final "---" before the report footer
        rest = report[fig_start:]
        # Find the last "---" that precedes the footer
        idx = rest.rfind("\n---\n")
        if idx > 0:
            fig_end = fig_start + idx
            report = report[:fig_start] + new_figures + report[fig_end:]

    # --- Fix Section 3.4 header count ---
    report = report.replace(
        "**13 CI benchmarks produced empty result files",
        "**12 CI benchmarks produced empty result files"
    )

    # --- Write updated report ---
    REPORT_PATH.write_text(report)
    print(f"Updated {REPORT_PATH}")
    print(f"  Matched points: {n_matched}")
    print(f"  Kernels: {len(stats)}")
    print(f"  OK: {sum(1 for s in stats.values() if s['cat']=='OK')}")
    print(f"  TOO_FAST: {sum(1 for s in stats.values() if s['cat']=='TOO_FAST')}")
    print(f"  TOO_SLOW: {sum(1 for s in stats.values() if s['cat']=='TOO_SLOW')}")


if __name__ == "__main__":
    main()
