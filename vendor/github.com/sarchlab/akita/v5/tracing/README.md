# tracing

Package `tracing` provides a task-based tracing framework for observing what
happens inside a simulation. It uses the `hooking` mechanism to collect
structured traces without modifying component logic.

## Core Concepts

### Tasks

A `Task` is the aggregate record a stateful tracer builds from the stream of
trace events. It has a start and end time, a location (the component name), an
optional parent, and lists of tags and milestones.

Components do not build a `Task` directly. They emit lightweight event structs;
the emit functions stamp the event time from the domain's clock and fire a hook.

### Tags

A `TaskTag` is a categorical label attached to a task while it is processed —
e.g. `"read-hit"`, `"write-miss"`. (Tags were previously called "steps".)

### Milestones

A `Milestone` records when a blocking condition is resolved during task
processing. Milestones carry a `Kind` (e.g. `MilestoneKindNetworkTransfer`,
`MilestoneKindQueue`). A milestone's location is inherited from its task.

## Emit API

The domain is always the first argument; the per-event data goes in a struct.
Callers pass **no** time — the emit functions read it from the domain's clock,
and only when a tracer is attached (the `NumHooks()==0` fast path keeps tracing
free when disabled). A domain must therefore be a `NamedHookable`, which is a
`naming.Named` + `hooking.Hookable` + `timing.TimeTeller`.

```go
tracing.StartTask(domain, tracing.TaskStart{
    ID: id, ParentID: parentID, Kind: "req_in", What: "ReadReq", Detail: msg,
})
tracing.AddTaskTag(domain, tracing.TaskTag{TaskID: id, What: "read-hit"})
tracing.AddMilestone(domain, tracing.Milestone{
    TaskID: id, Kind: tracing.MilestoneKindQueue, What: "queued",
})
tracing.EndTask(domain, tracing.TaskEnd{ID: id})
```

`TaskStart.Location` is optional and defaults to `domain.Name()`; set it
explicitly for network tracing.

### Message Tracing Helpers

Convenience functions for request/response message tracing:

```go
tracing.TraceReqInitiate(domain, msg, parentTaskID) // sender starts req
tracing.TraceReqReceive(domain, msg)                // receiver starts handling
tracing.TraceReqComplete(domain, msg)               // receiver done
tracing.TraceReqFinalize(domain, msg)               // sender gets response
```

The receiver-side task ID is derived from the message without mutating it, via
`MsgIDAtReceiver(msg, domain)` / `ForgetMsgIDAtReceiver(msgID, domain)`.

## Tracer Interface

Each method receives the event struct for its trace point:

```go
type Tracer interface {
    StartTask(t TaskStart)
    EndTask(t TaskEnd)
    AddTaskTag(tag TaskTag)
    AddMilestone(m Milestone)
}
```

Embed `tracing.NopTracer` to implement only the methods you care about. Events
carry their own `Time`, so tracers do not need a `TimeTeller`.

### Connecting a Tracer to a Domain

```go
tracing.CollectTrace(domain, tracer)
```

## Built-in Tracers

| Tracer | Purpose |
|--------|---------|
| `AverageTimeTracer` | Average task duration for filtered tasks |
| `TotalTimeTracer` | Total task processing time |
| `BusyTimeTracer` | Non-overlapping busy time (merges overlapping tasks) |
| `TagCountTracer` | Counts how many times each tag name occurs |
| `BackTraceTracer` | Records in-flight tasks; `DumpBackTrace` prints the parent chain |
| `DBTracer` | Persists tasks, tags, and milestones to a `DataRecorder` (SQLite) |

The time/count tracers take only a `TaskFilter` (`func(TaskStart) bool`):

```go
t := tracing.NewTotalTimeTracer(func(t tracing.TaskStart) bool {
    return t.Kind == "req_in"
})
```

### DBTracer

```go
tracer := tracing.NewDBTracer(timeTeller, dataRecorder)
tracer.StartTracing()
// ... simulation runs ...
tracer.StopTracing()
tracer.Terminate()
```

Only tasks that overlap an active tracing window are recorded. `DBTracer` keeps
a `TimeTeller` solely to time-stamp the tracing-window segments. It writes three
data tables plus a segments table:

- `trace` — one row per task. `Location` is dictionary-encoded via the shared
  `location` table (`akita_data:"location"`) to keep the largest table small.
- `milestone` — one row per milestone (no location; inherited from the task).
- `tag` — one row per tag (no location; inherited from the task).

## Hook Positions

| Position | When |
|----------|------|
| `HookPosTaskStart` | Task begins |
| `HookPosTaskTag` | Tag recorded |
| `HookPosMilestone` | Milestone recorded |
| `HookPosTaskEnd` | Task ends |
