import csv
import hashlib
import json
import tempfile
import unittest
from pathlib import Path

import build_loop_kernel_reference
import build_reference

SIZES = build_reference.SIZES
FIELDS = ["Kernel_Name", *SIZES, "Start_Timestamp", "Duration_ns"]


def collection(folder, name, plan, reps):
    """Write a plan and a collection of it; reps maps (case, rep) to launches or a status."""
    directory = Path(folder) / name
    plan_path = Path(folder) / f"{name}.json"
    plan_path.write_text(json.dumps(plan))
    benchmarks = {case["case"]: case["benchmark"] for case in plan}
    lines = []
    for (case_id, rep), launches in reps.items():
        status = launches if isinstance(launches, str) else "ok"
        row = {"case_id": case_id, "rep": rep, "benchmark": benchmarks[case_id], "status": status,
               "started_at": 1795732304.0 + 10 * len(lines), "seconds": 2.5,
               "core_clock_mhz_max": 1600 + rep}
        if status == "ok":
            row["measured_dispatches"] = len(launches)
            trace = directory / "dispatch" / row["benchmark"] / str(case_id) / str(rep) / "trace"
            trace.mkdir(parents=True)
            with (trace / "measured-dispatches.csv").open("w", newline="") as file:
                writer = csv.writer(file)
                writer.writerow(FIELDS)
                # Rows are written out of start order.
                for start, launch in reversed(list(enumerate(launches))):
                    writer.writerow([*launch[:-1], 1000 * start, launch[-1]])
        lines.append(json.dumps(row))
    directory.mkdir(exist_ok=True)
    (directory / "collection-current.log").write_text("\n".join(lines) + "\n")
    (directory / "collection.json").write_text(json.dumps({
        "plan_sha256": hashlib.sha256(plan_path.read_bytes()).hexdigest(),
        "benchmarks_revision": "abc", "core_clock_limit_mhz": 1680, "timer": "rocprofv3",
    }))
    return plan_path, directory


KERNEL = ["kernel(float*)", 256, 1, 1, 64, 1, 1]
OTHER = ["other(float*)", 64, 1, 1, 64, 1, 1]


def planned(case_id, x, series="block_size=64", launches=(KERNEL,)):
    return {"benchmark": "example", "sample": "example", "axis": "size", "series": series,
            "x": x, "case": case_id, "args": [f"-size={x}"],
            "expected_launches": [list(launch) for launch in launches]}


class BuildReferenceTest(unittest.TestCase):
    def test_cases_reduce_to_median_launches_in_series(self):
        plan = [
            planned(2, 128),
            planned(1, 64),
            planned(3, 64, "block_size=32, mode=\"fast\"", (KERNEL, OTHER)),
            planned(4, 256),
            planned(5, 512),
            # A case without simulator arguments stays out.
            {"benchmark": "example", "series": "", "axis": "size", "x": 1, "case": 100000},
        ]
        reps = {
            (1, 1): [[*KERNEL, 30]], (1, 2): [[*KERNEL, 10]], (1, 3): [[*KERNEL, 20]],
            (1, 4): "failed",
            (2, 1): [[*KERNEL, 40]],
            (3, 1): [[*KERNEL, 5], [*OTHER, 7]], (3, 2): [[*KERNEL, 6], [*OTHER, 8]],
            (4, 1): "failed",
            (5, 1): [[*KERNEL, 5]], (5, 2): [[*OTHER, 5]],
            (100000, 1): [[*KERNEL, 5]],
        }
        with tempfile.TemporaryDirectory() as folder:
            plan_path, directory = collection(folder, "c", plan, reps)
            series, dropped = build_reference.build(plan, directory)
            info = build_reference.provenance(plan_path, directory)
        self.assertEqual(dropped, {"no_successful_repetitions": [4],
                                   "inconsistent_launches": [5]})
        self.assertEqual(series, [
            {"benchmark": "example", "sample": "example", "axis": "size",
             "parameters": {"block_size": 32, "mode": "fast"},
             "cases": [{"case": 3, "x": 64, "args": ["-size=64"], "repetitions": 2,
                        "launches": [[*KERNEL, 5.5], [*OTHER, 7.5]]}]},
            {"benchmark": "example", "sample": "example", "axis": "size",
             "parameters": {"block_size": 64},
             "cases": [
                 {"case": 1, "x": 64, "args": ["-size=64"], "repetitions": 3,
                  "launches": [[*KERNEL, 20]]},
                 {"case": 2, "x": 128, "args": ["-size=128"], "repetitions": 1,
                  "launches": [[*KERNEL, 40]]},
             ]},
        ])
        self.assertEqual(info["benchmark_revision"], "abc")
        self.assertEqual(info["collection_started_at"], "2026-11-26T22:31:44+00:00")
        self.assertEqual(info["collection_finished_at"], "2026-11-26T22:33:26+00:00")
        self.assertEqual(info["max_sampled_core_clock_mhz"], 1604)
        self.assertEqual((info["core_clock_limit_mhz"], info["timer"]), (1680, "rocprofv3"))

    def test_collections_that_do_not_fit_the_plan_are_rejected(self):
        for label, plan, reps in (
            ("unexpected launches", [planned(1, 64)], {(1, 1): [[*OTHER, 5]]}),
            ("non-positive duration", [planned(1, 64)], {(1, 1): [[*KERNEL, 0]]}),
        ):
            with self.subTest(label), tempfile.TemporaryDirectory() as folder:
                _, directory = collection(folder, "c", plan, reps)
                with self.assertRaises(ValueError):
                    build_reference.build(plan, directory)
        with tempfile.TemporaryDirectory() as folder:
            plan_path, directory = collection(folder, "c", [planned(1, 64)], {})
            plan_path.write_text("[]")
            with self.assertRaisesRegex(ValueError, "did not run the plan"):
                build_reference.provenance(plan_path, directory)


def loop_case(case_id, benchmark, series, x):
    return {"benchmark": benchmark, "axis": "iterations", "series": series, "x": x,
            "case": case_id}


IC = 'instruction="v_fma_f32", dependency="chained", ops_per_iter={}, num_blocks=1, ' \
     'threads_per_block=256'
LW = 'layout="compiled", instruction="v_fma_f32", ops_per_iter=2, num_blocks=1, ' \
     'threads_per_block=64'
COPY = ["__amd_rocclr_copyBuffer", 512, 1, 1, 512, 1, 1]


class BuildLoopKernelReferenceTest(unittest.TestCase):
    def test_a_later_collection_replaces_a_configuration(self):
        first = [loop_case(10, "instruction_cost", IC.format(4), 4096),
                 loop_case(11, "instruction_cost", IC.format(0), 4096),
                 {**planned(1, 64), "case": 12}]
        second = [loop_case(20, "instruction_cost", IC.format(4), 4096),
                  loop_case(21, "loop_wait", LW, 16384),
                  # Longer than the default cap, so left out.
                  loop_case(22, "loop_wait", LW, 65536)]
        fma = ["ic_v_fma_f32_chained_k4", 256, 1, 1, 256, 1, 1]
        with tempfile.TemporaryDirectory() as folder:
            inputs = [
                # The first collection also records the copy that follows the kernel.
                collection(folder, "a", first, {
                    (10, 1): [[*fma, 100], [*COPY, 9]],
                    (11, 1): [["ic_empty", 256, 1, 1, 256, 1, 1, 50], [*COPY, 9]],
                    (12, 1): [[*KERNEL, 5]],
                }),
                collection(folder, "b", second, {
                    (20, 1): [[*fma, 110]], (20, 2): [[*fma, 120]],
                    (21, 1): [["lw_compiled_v_fma_f32_k2", 64, 1, 1, 64, 1, 1, 70]],
                    (22, 1): [["lw_compiled_v_fma_f32_k2", 64, 1, 1, 64, 1, 1, 280]],
                }),
            ]
            loaded = [(json.loads(plan.read_text()), directory) for plan, directory in inputs]
            series, dropped, replaced = build_loop_kernel_reference.build(loaded)
            uncapped, _, _ = build_loop_kernel_reference.build(loaded, max_iterations=65536)
        self.assertEqual([case["x"] for case in uncapped[1]["cases"]], [16384, 65536])
        self.assertEqual(replaced, [10])
        self.assertEqual(dropped, {"no_successful_repetitions": [], "inconsistent_launches": []})
        self.assertEqual([row["benchmark"] for row in series], ["instruction_cost", "loop_wait"])
        ic, lw = series
        self.assertEqual(ic["parameters"],
                         {"instruction": "v_fma_f32", "num_blocks": 1, "threads_per_block": 256})
        self.assertEqual(ic["axis"], "iterations")
        self.assertEqual(ic["cases"], [
            {"case": 11, "x": 4096, "parameters": {"dependency": "chained", "ops_per_iter": 0},
             "args": ["-instruction=v_fma_f32", "-num-blocks=1", "-threads-per-block=256",
                      "-dependency=chained", "-ops-per-iter=0", "-iterations=4096"],
             "repetitions": 1, "launches": [["ic_empty", 256, 1, 1, 256, 1, 1, 50]]},
            {"case": 20, "x": 4096, "parameters": {"dependency": "chained", "ops_per_iter": 4},
             "args": ["-instruction=v_fma_f32", "-num-blocks=1", "-threads-per-block=256",
                      "-dependency=chained", "-ops-per-iter=4", "-iterations=4096"],
             "repetitions": 2, "launches": [[*fma, 115]]},
        ])
        self.assertEqual([case["x"] for case in lw["cases"]], [16384])
        self.assertEqual(lw["cases"][0]["parameters"], {"layout": "compiled", "ops_per_iter": 2})
        self.assertEqual(lw["cases"][0]["args"][-3:],
                         ["-layout=compiled", "-ops-per-iter=2", "-iterations=16384"])

    def test_a_kernel_other_than_the_samples_is_rejected(self):
        for label, launch in (
            ("other kernel", ["ic_v_fma_f32_independent_k4", 256, 1, 1, 256, 1, 1, 9]),
            ("other shape", ["ic_v_fma_f32_chained_k4", 64, 1, 1, 64, 1, 1, 9]),
        ):
            plan = [loop_case(10, "instruction_cost", IC.format(4), 4096)]
            with self.subTest(label), tempfile.TemporaryDirectory() as folder:
                _, directory = collection(folder, "a", plan, {(10, 1): [launch]})
                with self.assertRaises(ValueError):
                    build_loop_kernel_reference.build([(plan, directory)])

    def test_kernel_names_follow_the_benchmarks(self):
        kernel = build_loop_kernel_reference.kernel
        p = {"instruction": "s_branch", "dependency": "independent", "ops_per_iter": 8}
        self.assertEqual(kernel("instruction_cost", p), "ic_s_branch_k8")
        self.assertEqual(kernel("instruction_cost", {**p, "ops_per_iter": 0}), "ic_empty")
        self.assertEqual(kernel("instruction_cost", {**p, "instruction": "v_mov_b64"}),
                         "ic_v_mov_b64_independent_k8")
        self.assertEqual(kernel("loop_wait", {**p, "layout": "sgpr_at_head"}),
                         "lw_sgpr_at_head_s_branch_k8")


if __name__ == "__main__":
    unittest.main()
