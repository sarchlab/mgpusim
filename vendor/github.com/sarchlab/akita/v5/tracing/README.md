# tracing

Package `tracing` provides a task-based tracing framework for observing what
happens inside a simulation. It uses the `sim.Hook` mechanism to collect
structured traces without modifying component logic.

## Core Concepts

### Tasks

A `Task` represents a unit of work with a start time, end time, location, and
optional parent for hierarchical tracing:

```go
type Task struct {
    ID         uint64         // unique task ID
    ParentID   uint64         // parent task (0 = root)
    Kind       string         // e.g., "req_in", "req_out"
    What       string         // e.g., "ReadReq", "WriteReq"
    Location   string         // component name
    StartTime  sim.VTimeInSec // picoseconds
    EndTime    sim.VTimeInSec
    Steps      []TaskStep
    Milestones []Milestone
}
```

### Milestones

A `Milestone` records when a blocking condition is resolved during task
processing. Milestones carry a `Kind` (e.g., `MilestoneKindNetworkTransfer`,
`MilestoneKindQueue`) for categorization.

## Tracing API

Components call these functions to emit trace events through hooks:

```go
// Start a task
tracing.StartTask(id, parentID, domain, kind, what, detail)

// Record a step within a task
tracing.AddTaskStep(id, domain, what)

// Record a milestone
tracing.AddMilestone(taskID, kind, what, location, domain)

// End a task
tracing.EndTask(id, domain)
```

### Message Tracing Helpers

Convenience functions for request/response message tracing:

```go
tracing.TraceReqInitiate(msg, domain, parentTaskID) // sender starts req
tracing.TraceReqReceive(msg, domain)                // receiver starts handling
tracing.TraceReqComplete(msg, domain)               // receiver done
tracing.TraceReqFinalize(msg, domain)               // sender gets response
```

## Tracer Interface

```go
type Tracer interface {
    StartTask(task Task)
    StepTask(task Task)
    AddMilestone(milestone Milestone)
    EndTask(task Task)
}
```

### Connecting a Tracer to a Domain

```go
tracing.CollectTrace(domain, tracer)
```

This installs a hook on the `NamedHookable` domain that forwards trace events
to the tracer.

## Built-in Tracers

| Tracer | Purpose |
|--------|---------|
| `AverageTimeTracer` | Tracks average task duration for filtered tasks |
| `TotalTimeTracer` | Accumulates total task processing time |
| `BusyTimeTracer` | Tracks non-overlapping busy time (merges overlapping tasks) |
| `StepCountTracer` | Counts how many times each step name occurs |
| `BackTraceTracer` | Records in-flight tasks for debugging; `DumpBackTrace` prints parent chain |
| `DBTracer` | Persists tasks and milestones to a SQLite database via `DataRecorder` |

### DBTracer

The `DBTracer` writes to a `DataRecorder` backend and supports on/off tracing:

```go
tracer := tracing.NewDBTracer(timeTeller, dataRecorder)
tracer.StartTracing()   // begin recording
// ... simulation runs ...
tracer.StopTracing()    // flush and stop
tracer.Terminate()      // final cleanup
```

Only tasks that overlap with an active tracing period are recorded.

## Hook Positions

| Position | When |
|----------|------|
| `HookPosTaskStart` | Task begins |
| `HookPosTaskStep` | Task step recorded |
| `HookPosMilestone` | Milestone recorded |
| `HookPosTaskEnd` | Task ends |
