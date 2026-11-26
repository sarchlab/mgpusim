import unittest

from build_code_fetch_latency_reference import BODY_SIZES, LOOPS, build

SHAPE = [64, 1, 1, 64, 1, 1]
REPS = 3


def name(body):
    return f"fetch_kernel_{body}(float*, float, float, int)"


def reduced(loops, body, rep, nanoseconds, **extra):
    row = {"body_ops": str(body), "loops": str(loops), "rep": str(rep)}
    return {**row, "dispatch_ns": str(nanoseconds), **extra}


def traced(loops, body, rep, nanoseconds, **extra):
    row = {"body_ops": str(body), "loops": str(loops), "rep": str(rep), "Kernel_Name": name(body)}
    row.update(Start_Timestamp=str(1000 * rep), End_Timestamp=str(1000 * rep + nanoseconds))
    row.update(Grid_Size_X="64", Grid_Size_Y="1", Grid_Size_Z="1")
    row.update(Workgroup_Size_X="64", Workgroup_Size_Y="1", Workgroup_Size_Z="1")
    return {**row, **extra}


def collection(make=reduced, reps=REPS, skip=()):
    """A complete sweep whose durations are body * loops + repetition."""
    return [
        make(loops, body, rep, body * loops + rep)
        for loops in LOOPS
        for body in BODY_SIZES
        for rep in range(1, reps + 1)
        if (loops, body, rep) not in skip
    ]


class BuildTest(unittest.TestCase):
    def test_reduced_and_traced_rows_give_the_same_reference(self):
        from_durations = build(collection(reduced), REPS)
        self.assertEqual(build(collection(traced), REPS), from_durations)
        self.assertEqual([row["parameters"]["loops"] for row in from_durations], LOOPS)
        for row in from_durations:
            self.assertEqual(
                [row[key] for key in ("benchmark", "sample", "axis")],
                ["code_fetch_latency", "code_fetch_latency", "body_ops"],
            )
            self.assertEqual(row["parameters"]["num_blocks"], 1)
            self.assertEqual(row["parameters"]["threads_per_block"], 64)
            self.assertEqual([case["x"] for case in row["cases"]], BODY_SIZES)
        cases = [case for row in from_durations for case in row["cases"]]
        self.assertEqual([case["case"] for case in cases], list(range(100001, 100053)))
        self.assertEqual(
            from_durations[1]["cases"][2],
            {
                "case": 100016,
                "x": 64,
                "args": ["-body-ops=64", "-loops=4"],
                "repetitions": REPS,
                "launches": [[name(64), *SHAPE, 64 * 4 + 2]],
            },
        )

    def test_failed_and_other_rows_are_dropped(self):
        rows = collection(traced) + [
            reduced(4, 64, 7, 9000, rc="1"),
            reduced(4, 64, 8, 9000, n_dispatch="0"),
            reduced(4, 64, 9, 9000, op="xor_add"),
            # A failed run of the collection script has no trace columns.
            traced(4, 64, 10, 9000, status="failed", n_fetch_dispatches="0", Kernel_Name="",
                   Start_Timestamp="", End_Timestamp="", Grid_Size_X=""),
        ]
        self.assertEqual(build(rows, REPS), build(collection(traced), REPS))

    def test_invalid_rows_are_rejected(self):
        for label, row in (
            ("no loops", reduced(0, 64, 9, 10)),
            ("negative loops", reduced(-4, 64, 9, 10)),
            ("chain beyond the sample's limit", reduced(32, 65536, 9, 10)),
            ("unsupported body size", reduced(4, 100, 9, 10)),
            ("no duration", reduced(4, 64, 9, "")),
            ("non-positive duration", reduced(4, 64, 9, 0)),
            ("second dispatch in a repetition", reduced(4, 64, 1, 10)),
            ("another kernel", traced(4, 64, 9, 10, Kernel_Name="other_kernel(float*)")),
            ("another body size's kernel", traced(4, 64, 9, 10, Kernel_Name=name(128))),
            ("blank kernel name", traced(4, 64, 9, 10, Kernel_Name="")),
            ("blank grid size", traced(4, 64, 9, 10, Grid_Size_X="")),
            ("blank work-group size", traced(4, 64, 9, 10, Workgroup_Size_Z="")),
            ("another grid", traced(4, 64, 9, 10, Grid_Size_X="128")),
            ("blank end timestamp", traced(4, 64, 9, 10, End_Timestamp="")),
        ):
            make = traced if "Kernel_Name" in row else reduced
            with self.subTest(label), self.assertRaises(ValueError):
                build(collection(make) + [row], REPS)

    def test_repetitions_must_agree_on_the_launch(self):
        rows = collection(traced)
        rows[0] = {**rows[0], "Kernel_Name": "fetch_kernel_16()"}
        with self.assertRaisesRegex(ValueError, "disagree"):
            build(rows, REPS)

    def test_an_incomplete_collection_is_rejected(self):
        whole_cell = {(8, 1024, rep) for rep in range(1, REPS + 1)}
        whole_series = {(16, body, rep) for body in BODY_SIZES for rep in range(1, REPS + 1)}
        for label, rows, message in (
            ("a missing cell", collection(skip=whole_cell), "1 .* cells missing"),
            ("a missing series", collection(skip=whole_series), "13 .* cells missing"),
            ("an unexpected cell", collection() + [reduced(2, 64, 1, 10)], "1 unexpected"),
            ("a short cell", collection(skip={(8, 1024, 2)}), "1 cells differ"),
            ("a long cell", collection() + [reduced(8, 1024, REPS + 1, 10)], "1 cells differ"),
            ("another repetition count", collection(reps=REPS + 1), "52 cells differ"),
            ("nothing kept", [{**row, "rc": "1"} for row in collection()], "52 .* missing"),
            ("no rows", [], "52 .* missing"),
        ):
            with self.subTest(label), self.assertRaisesRegex(ValueError, message):
                build(rows, REPS)


if __name__ == "__main__":
    unittest.main()
