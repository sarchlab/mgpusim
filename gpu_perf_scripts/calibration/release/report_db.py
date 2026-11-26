"""Write calibration results to the hosted calibration-report database.

Each call opens a short-lived connection, runs one ingest function in its own
transaction and closes. Every ingest call is safe to replay, so calls whose
outcome is unknown because the connection failed or the statement timed out
are retried. Validation and run-state errors are final.
"""

import json
import math
import os
import sys
import time
from functools import partial

import psycopg
from psycopg.types.json import Json

RETRY_SECONDS = 30 * 60
MAX_DELAY_SECONDS = 60
# Keepalives detect a dead connection and the statement timeout bounds a stuck
# call, so neither can block past the retry deadline.
CONNECT_OPTIONS = {
    "connect_timeout": 30,
    "keepalives": 1,
    "keepalives_idle": 30,
    "keepalives_interval": 10,
    "keepalives_count": 3,
}
STATEMENT_TIMEOUT = "SET LOCAL statement_timeout = '60s'"
_dumps = partial(json.dumps, separators=(",", ":"), allow_nan=False)


def dsn():
    value = os.environ.get("CALIBRATION_REPORT_DSN", "").strip()
    if not value:
        raise RuntimeError("CALIBRATION_REPORT_DSN is not set.")
    return value


def retryable(error):
    state = error.sqlstate or ""
    if isinstance(error, psycopg.OperationalError) and not state:
        return True  # The connection failed or broke.
    return state == "57014" or state.startswith(("08", "57P"))


def call(sql, *args, deadline=math.inf, connect=psycopg.connect, sleep=time.sleep,
         clock=time.monotonic):
    """Run one ingest call; no attempt starts after the monotonic deadline."""
    params = [arg if isinstance(arg, str) else Json(arg, dumps=_dumps) for arg in args]
    target = dsn()
    stop = min(clock() + RETRY_SECONDS, deadline)
    delay = 1
    last = None
    while True:
        now = clock()
        if now >= deadline:
            raise TimeoutError("The job deadline passed before the database write could start.")
        if last is not None and now >= stop:
            raise last  # The retry window closed during the backoff.
        try:
            # Leaving the block commits the transaction.
            with connect(target, **CONNECT_OPTIONS) as connection:
                connection.execute(STATEMENT_TIMEOUT)
                connection.execute(sql, params)
            return
        except psycopg.Error as error:
            if not retryable(error) or clock() + delay > stop:
                raise
            last = error
            print(f"Calibration report database unavailable, retrying in {delay} s: {error}",
                  file=sys.stderr, flush=True)
            sleep(delay)
            delay = min(delay * 2, MAX_DELAY_SECONDS)


def open_run(run_key, report, benchmarks, progress, deadline=math.inf):
    call("SELECT cr.open_run(%s, %s::json, %s::json, %s::json)",
         run_key, report, benchmarks, progress, deadline=deadline)


def put_points(run_key, points, deadline=math.inf):
    call("SELECT cr.put_points(%s, %s::json)", run_key, points, deadline=deadline)


def finish_run(run_key, state, progress):
    call("SELECT cr.finish_run(%s, %s, %s::json)", run_key, state, progress)
