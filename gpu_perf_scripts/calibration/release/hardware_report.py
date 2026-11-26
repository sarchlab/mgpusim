"""Show a hardware collection in the hosted calibration report while it runs.

The report gets one run for the collection, with the device as its only group
and one hardware series per benchmark. A case appears once its repetitions are
over, as the median time of each measured launch over its successful
repetitions, the number build_reference.py later puts into the reference.

The collector hands over dispatch rows as it writes them to
measured-dispatches.csv; this module knows nothing about a collection's files.
"""

import os
import re
import time
from datetime import datetime, timezone
from statistics import median
from urllib.parse import urlsplit

import report_db
from build_reference import fixed_parameters

# Kept in step with workflow.py, which this module cannot import: the collector
# runs on GPU machines with Python 3.9, and workflow.py needs 3.11.
UNITS = {
    "array_bytes": "B",
    "buffer_size_mb": "MiB",
    "cacheline_bytes": "B",
    "grid_spacing": "Å",
    "amb_temp": "°C",
}
# A report that cannot be reached holds the GPU up for at most this long, once.
WRITE_SECONDS = 30
# Cases an earlier invocation finished are sent again in calls of this many.
BATCH_CASES = 200
OPEN_RUN = "SELECT cr.open_run(%s, %s::json, %s::json, %s::json)"
PUT_POINTS = "SELECT cr.put_points(%s, %s::json, %s::json)"
FINISH_RUN = "SELECT cr.finish_run(%s, %s, %s::json)"


def now():
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def base_kernel_name(name):
    """The kernel's name as workflow.py labels the simulator's launches, without its signature."""
    name = name[:-3] if name.endswith(".kd") else name
    name = name.split("(", 1)[0]
    return (name[5:] if name.startswith("void ") else name).strip()


def launch(dispatch):
    """What makes two dispatches the same launch: everything but when and how long."""
    return tuple(
        sorted((key, value) for key, value in dispatch.items() if key not in ("Start_Timestamp", "Duration_ns"))
    )


def median_launches(repetitions):
    """(kernel name, median duration) per launch, or None when the repetitions disagree on the launches."""
    sequences = {tuple(launch(dispatch) for dispatch in repetition) for repetition in repetitions}
    if len(sequences) != 1:
        return None
    return [
        (first["Kernel_Name"], median(repetition[index]["Duration_ns"] for repetition in repetitions))
        for index, first in enumerate(repetitions[0])
    ]


def without_credentials(error):
    """An error's text, safe to print: a malformed connection string can be quoted in it."""
    text = f"{type(error).__name__}: {error}"
    dsn = os.environ.get("CALIBRATION_REPORT_DSN", "").strip()
    secrets = [dsn]
    try:
        secrets.append(urlsplit(dsn).password)
    except ValueError:
        pass
    for secret in secrets:
        if secret:
            text = text.replace(secret, "***")
    return re.sub(r"(password\s*=\s*)\S+", r"\1***", text)


class HardwareReport:
    """One run of the hosted report, fed by one collection.

    `cases` is the whole plan, so that invocations limited to some benchmarks
    add up to one run, and a case is complete with `reps` successful
    repetitions. Progress counts cases: complete ones, and ones whose
    repetitions ended in this invocation without all of them succeeding.
    """

    def __init__(self, run_key, device, cases, reps, description):
        self.run_key, self.cases, self.reps, self.description = run_key, cases, reps, description
        self.device_key, self.device_name = device
        self.started_at = now()
        self.complete = self.incomplete = 0
        self.available = True

    def write(self, sql, *arguments):
        """One ingest call. The report is secondary: it must not stop or stall the measurement.

        After the first failure nothing more is sent. Starting the collection
        again with the same run key sends every finished case again.
        """
        if not self.available:
            return
        try:
            report_db.call(sql, self.run_key, *arguments, deadline=time.monotonic() + WRITE_SECONDS)
        except Exception as error:  # the database, the network, or a run that is already completed
            self.available = False
            print(f"The calibration report is not updated any more. {without_credentials(error)}", flush=True)

    def header(self):
        return {
            "title": f"{self.device_name} hardware collection",
            "description": self.description,
            "live": True,  # Required by the schema; the database derives it from the run state.
            "groups": {self.device_key: {"name": self.device_name}},
        }

    def benchmarks(self):
        definitions = {}
        for case in self.cases:
            names = (case["axis"], "launch", *fixed_parameters(case["series"]))
            definition = definitions.setdefault(
                case["benchmark"],
                {
                    "scaling_param": case["axis"],
                    "params": {},
                    "values": {
                        self.device_key: {
                            "name": f"{self.device_name} kernel time (median)",
                            "unit": "ns",
                            "role": "hardware_measurement",
                        }
                    },
                },
            )
            definition["params"].update({name: {"unit": UNITS.get(name, "")} for name in names})
        return definitions

    def progress(self):
        return {
            "total": len(self.cases),
            "completed": self.complete,
            "unsuccessful": self.incomplete,
            "running": 0,
            "queued": len(self.cases) - self.complete - self.incomplete,
            "started_at": self.started_at,
            "updated_at": now(),
        }

    def points(self, case, repetitions, reason):
        """One point per launch of a case: its median, or why there is none.

        Without a successful repetition the launches are known only for a case
        that names them; another case gets no point, since a point under a
        made-up launch name would stay in the report after a later success.
        """
        launches = median_launches(repetitions) if repetitions else None
        if repetitions and launches is None:
            reason = "the repetitions measured different launches"
        if launches is None:
            launches = [(expected[0], None) for expected in case.get("expected_launches") or ()]
        points = []
        for index, (name, duration) in enumerate(launches):
            point = {
                "benchmark": case["benchmark"],
                "group": self.device_key,
                "x": case["x"],
                "non_scaling_params": {
                    "launch": f"#{index + 1} {base_kernel_name(name)}",
                    **fixed_parameters(case["series"]),
                },
                "values": {self.device_key: duration},
            }
            if duration is None:
                state = "timeout" if "timeout" in reason else "failed"
                point["statuses"] = {self.device_key: {"state": state, "reason": reason}}
            points.append(point)
        return points

    def begin(self, finished):
        """Open the run and send the cases earlier invocations completed: (case, repetitions) pairs."""
        self.complete = len(finished)
        self.write(OPEN_RUN, self.header(), self.benchmarks(), self.progress())
        for start in range(0, len(finished), BATCH_CASES):
            batch = finished[start:start + BATCH_CASES]
            points = [point for case, repetitions in batch for point in self.points(case, repetitions, "")]
            self.write(PUT_POINTS, points, self.progress())

    def case_over(self, case, repetitions, reason):
        """Publish a case whose repetitions just ended; `reason` is the status of the last one that failed."""
        if len(repetitions) == self.reps:
            self.complete += 1
        else:
            self.incomplete += 1
        self.write(PUT_POINTS, self.points(case, repetitions, reason), self.progress())

    def end(self):
        """Complete the run only when every case of the plan is; a failed run can be opened again."""
        state = "completed" if self.complete == len(self.cases) else "failed"
        self.write(FINISH_RUN, state, self.progress())
