import os
import subprocess
import sqlite3
from collections import namedtuple

TestCase = namedtuple("TestCase", "dir executable arguments")

cwd = os.getcwd()

cases = [
    TestCase("empty_kernel", "empty_kernel", ""),
    TestCase("memcopy", "memcopy", ""),
    TestCase("../../samples/fir", "fir", "-length=64"),
    TestCase("../../samples/fir", "fir", "-length=65536"),
]


def compile(dir):
    """Get into the test case directory and run `go build`."""
    os.chdir(dir)
    subprocess.check_call(["go build"], shell=True)
    os.chdir(cwd)


def compare_sqlite_files(file1, file2):
    """Compare mgpusim_metrics tables from two SQLite3 files."""
    conn1 = sqlite3.connect(file1)
    conn2 = sqlite3.connect(file2)

    # Get column names and data
    cursor1 = conn1.cursor()
    cursor2 = conn2.cursor()

    # Get column names
    cursor1.execute("SELECT * FROM mgpusim_metrics LIMIT 0")
    columns = [description[0] for description in cursor1.description]

    # Get all rows
    cursor1.execute("SELECT * FROM mgpusim_metrics")
    rows1 = cursor1.fetchall()
    cursor2.execute("SELECT * FROM mgpusim_metrics")
    rows2 = cursor2.fetchall()

    conn1.close()
    conn2.close()

    if len(rows1) != len(rows2):
        print(f"❌ Number of rows differ: {len(rows1)} vs {len(rows2)}")
        return False

    # Compare each row
    for i, (row1, row2) in enumerate(zip(rows1, rows2)):
        if row1 != row2:
            print(f"❌ Row {i} differs:")
            print("File 1:", dict(zip(columns, row1)))
            print("File 2:", dict(zip(columns, row2)))
            return False

    return True


def run(test_case, run_index):
    """Get into the test case directory and run the executable."""
    os.chdir(test_case.dir)

    # Remove any existing SQLite3 files
    for f in os.listdir("."):
        if f.startswith("akita_sim_") and f.endswith(".sqlite3"):
            os.remove(f)

    # Run the simulation
    subprocess.check_call(
        [f"./{test_case.executable} -timing -report-all {test_case.arguments}"],
        shell=True,
    )

    # Find the generated SQLite3 file
    sqlite_files = [
        f
        for f in os.listdir(".")
        if f.startswith("akita_sim_") and f.endswith(".sqlite3")
    ]
    if not sqlite_files:
        raise Exception("No SQLite3 file found after simulation")

    current_sqlite = sqlite_files[0]
    renamed_sqlite = f"deterministic_metrics_{run_index}.sqlite3"

    # Rename the current SQLite3 file
    subprocess.check_call(
        [f"mv {current_sqlite} {renamed_sqlite}"],
        shell=True,
    )

    # Compare with previous run if not the first run
    if run_index > 0:
        prev_sqlite = f"deterministic_metrics_{run_index - 1}.sqlite3"
        if not compare_sqlite_files(renamed_sqlite, prev_sqlite):
            raise Exception("Simulation results are not deterministic!")

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
