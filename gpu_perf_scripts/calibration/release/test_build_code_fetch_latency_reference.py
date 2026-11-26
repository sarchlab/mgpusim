import unittest

from build_code_fetch_latency_reference import build

NAME = "fetch_kernel_64(float*, float, float, int)"
SHAPE = [64, 1, 1, 64, 1, 1]


def reduced(rep, nanoseconds, **extra):
    row = {"body_ops": "64", "loops": "4", "rep": str(rep), "dispatch_ns": str(nanoseconds)}
    return {**row, **extra}


def traced(rep, start, end, **extra):
    row = {"body_ops": "64", "loops": "4", "rep": str(rep), "Kernel_Name": NAME}
    row.update(Start_Timestamp=str(start), End_Timestamp=str(end))
    row.update(Grid_Size_X="64", Grid_Size_Y="1", Grid_Size_Z="1")
    row.update(Workgroup_Size_X="64", Workgroup_Size_Y="1", Workgroup_Size_Z="1")
    return {**row, **extra}


class BuildTest(unittest.TestCase):
    def test_reduced_and_traced_rows_give_the_same_case(self):
        expected = {
            "case": 100001,
            "x": 64,
            "args": ["-body-ops=64", "-loops=4"],
            "repetitions": 3,
            "launches": [[NAME, *SHAPE, 2000]],
        }
        for rows in (
            [reduced(1, 1900), reduced(2, 2000), reduced(3, 2500)],
            [traced(1, 100, 2000), traced(2, 500, 2500), traced(3, 0, 2500)],
        ):
            (series,) = build(rows)
            self.assertEqual(series["benchmark"], "code_fetch_latency")
            self.assertEqual(series["sample"], "code_fetch_latency")
            self.assertEqual(series["axis"], "body_ops")
            self.assertEqual(
                series["parameters"], {"loops": 4, "num_blocks": 1, "threads_per_block": 64}
            )
            self.assertEqual(series["cases"], [expected])

    def test_failed_and_other_rows_are_dropped(self):
        rows = [
            reduced(1, 2000, rc="0", n_dispatch="1", op="fma"),
            reduced(2, 9000, rc="1", n_dispatch="1", op="fma"),
            reduced(3, 9000, rc="0", n_dispatch="0", op="fma"),
            reduced(4, 9000, rc="0", n_dispatch="1", op="xor_add"),
            # A failed run of the collection script has no trace columns.
            traced(5, "", "", status="failed", n_fetch_dispatches="0", Kernel_Name=""),
            traced(6, 0, 2000, status="ok", n_fetch_dispatches="1"),
        ]
        (series,) = build(rows)
        self.assertEqual(series["cases"][0]["repetitions"], 2)
        self.assertEqual(series["cases"][0]["launches"][0][-1], 2000)

    def test_series_and_cases_are_ordered_and_numbered(self):
        rows = [
            {**reduced(1, 10), "loops": "16", "body_ops": "32"},
            {**reduced(1, 10), "loops": "1", "body_ops": "64"},
            {**reduced(1, 10), "loops": "1", "body_ops": "16"},
        ]
        series = build(rows)
        self.assertEqual([row["parameters"]["loops"] for row in series], [1, 16])
        self.assertEqual([case["x"] for case in series[0]["cases"]], [16, 64])
        self.assertEqual(
            [case["case"] for row in series for case in row["cases"]], [100001, 100002, 100003]
        )

    def test_inconsistent_input_is_rejected(self):
        for rows in (
            [reduced(1, 10), reduced(1, 12)],
            [reduced(1, 0)],
            [traced(1, 0, 10, Kernel_Name="other_kernel(float*)")],
            [traced(1, 0, 10, Grid_Size_X="128")],
            [traced(1, 0, 10), traced(2, 0, 10, Kernel_Name="fetch_kernel_64()")],
        ):
            with self.subTest(rows=rows), self.assertRaises(ValueError):
                build(rows)


if __name__ == "__main__":
    unittest.main()
