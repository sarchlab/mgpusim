import os
import subprocess
from collections import namedtuple

TestCase = namedtuple("TestCase", "dir executable arguments")

cwd = os.getcwd()

cases = [
    # TestCase("empty_kernel", "empty_kernel", ""),
    # TestCase("memcopy", "memcopy", ""),
    # TestCase("../../samples/fir", "fir", "-length=64"),
    TestCase("../../samples/fir", "fir", "-length=65536"),
]


def compile(dir):
    """Get into the test case directory and run `go build`."""
    os.chdir(dir)
    subprocess.check_call(["go build"], shell=True)
    os.chdir(cwd)


def run(test_case, run_index):
    """Get into the test case directory and run the executable."""
    os.chdir(test_case.dir)
    subprocess.check_call(
        [f"./{test_case.executable} -timing -report-all {test_case.arguments}"],
        shell=True,
    )
    subprocess.check_call(
        [f"mv metrics.csv deterministic_metrics_{run_index}.csv"], shell=True
    )

    if run_index > 0:
        subprocess.check_call(
            [
                f"diff deterministic_metrics_{run_index}.csv deterministic_metrics_{run_index - 1}.csv"
            ],
            shell=True,
        )

    os.chdir(cwd)


def test(test_case):
    """Run the test case."""
    compile(test_case.dir)
    for i in range(5):
        run(test_case, i)


def main():
    for test_case in cases:
        test(test_case)


if __name__ == "__main__":
    main()
