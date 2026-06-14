# Tracing API Refactoring — Design

**Status:** Agreed plan, ready to implement
**Branch:** `worktree-improve-tracing-api`
**Scope:** the `tracing` package, its ~100 call sites across the repo, the
`datarecording` location feature, and daisen's trace reader.

This is the v5 tracing refactor. We are **not** preserving backward
compatibility — it is fine to break existing trace databases and the on-disk
schema, as long as daisen is updated in lockstep.

---

## 1. Background — how tracing works today

Tracing rides on the generic `hooking` observer mechanism. A component (a
"domain", i.e. a `NamedHookable`) emits trace events by calling free functions
in `tracing`; each builds a `Task` or `Milestone`, wraps it in a
`hooking.HookCtx`, and calls `domain.InvokeHook`. A registered `traceHook`
forwards every event to a `Tracer`, which aggregates or persists it.

```
component ──StartTask/AddMilestone/…──▶ domain.InvokeHook(HookCtx)   [skipped if NumHooks()==0]
                                              │
                                              ▼
                                        traceHook.Func ──switch Pos──▶ Tracer.StartTask/…
                                                                            │
                                          DBTracer ──▶ DataRecorder ──▶ SQLite (read by daisen)
```

Key facts that shape this design:

- **The `NumHooks()==0` fast path** makes tracing near-free when no tracer is
  attached. Every emit function early-returns on it. We preserve this.
- **Hooking is a shared bus**, not tracing-private: `timing` (event hooks),
  `messaging` (port send/recv), and `queueing` (buffer push/pop) all emit
  through the same mechanism. Tracing is one consumer. We keep hooking as-is.
- **Steps are emitted but never persisted.** `DBTracer.StepTask` is a no-op;
  only the in-memory `StepCountTracer` reads steps. Milestones, by contrast,
  get a dedicated `milestone` table.
- **Milestone `Location` is redundant.** All 24 `AddMilestone` call sites pass
  the component's own name (`m.comp.Name()`), which already equals the task's
  location.

Call-site frequency (whole repo): `MsgIDAtReceiver` 48, `AddMilestone` 24,
`AddTaskStep` 21, `TraceReqComplete` 17, `EndTask` 16, `TraceReqReceive` 15,
`TraceReqFinalize` 13, `TraceReqInitiate` 12, `CollectTrace` 9,
`StartTaskWithSpecificLocation` 5, `StartTask` 4. Message tracing dominates, so
the `TraceReq*` helpers must stay ergonomic.

---

## 2. Goals / non-goals

### Goals

- **Honest naming** — rename task "steps" to "tags" (their real use is
  categorical labels), and actually persist them.
- **Safe parameters** — replace wide positional argument lists with small input
  structs so `kind`/`what`/`location` strings cannot be transposed.
- **Caller-supplied time** — events carry their own timestamp; tracers stop
  injecting a `TimeTeller`.
- **Smaller databases** — intern the repetitive `Location` string via the
  `datarecording` location table, and drop `Location` where it is redundant.
- **Consistency** — uniform `(domain, struct)` parameter shape, uniform
  `NumHooks` guards, checked type assertions.
- **Stay idiomatic** — plain structs, no fluent `With*` setters; keep the
  `NumHooks()==0` fast path.

### Non-goals

- Replacing the `hooking` mechanism. It is a shared bus; we only fix tracing's
  local use of it (checked assertions).
- Centralizing task `kind` values. Kinds are open-ended and component-specific
  (`"instruction"`, `"read_txn"`, …); `Kind` stays a plain `string`.
- Interning `What` / `Kind`, or giving them dictionary tables — overkill, and
  they do not belong in the *location* dictionary.
- Redesigning the receiver-ID **registry** or adding **context/handle**
  ergonomics — both deferred to separate discussions (see §5).

---

## 3. Target design

### 3.1 Emit API

`domain` is always a separate first argument; the per-event data lives in a
small struct; the caller supplies `Time`.

```go
// Task lifecycle
func StartTask(domain NamedHookable, t TaskStart)
func EndTask(domain NamedHookable, t TaskEnd)

// Events within a task
func AddTaskTag(domain NamedHookable, tag TaskTag)
func AddMilestone(domain NamedHookable, m Milestone)

// Message helpers — same roles as today, re-expressed on top of the above.
func TraceReqInitiate(domain NamedHookable, msg messaging.Msg, parentID uint64, now timing.VTimeInPicoSec)
func TraceReqReceive(domain NamedHookable, msg messaging.Msg, now timing.VTimeInPicoSec)
func TraceReqComplete(domain NamedHookable, msg messaging.Msg, now timing.VTimeInPicoSec)
func TraceReqFinalize(domain NamedHookable, msg messaging.Msg, now timing.VTimeInPicoSec)
```

Input structs:

```go
type TaskStart struct {
    ID       uint64
    ParentID uint64
    Kind     string                 // open-ended, plain string
    What     string
    Location string                 // optional; defaults to domain.Name() when empty
    Time     timing.VTimeInPicoSec
    Detail   any                    // not persisted
}

type TaskEnd struct {
    ID   uint64
    Time timing.VTimeInPicoSec
}

type TaskTag struct {
    ID     uint64                   // generated by AddTaskTag, like milestones
    TaskID uint64
    What   string
    Time   timing.VTimeInPicoSec
}

type Milestone struct {
    ID     uint64                   // generated by AddMilestone
    TaskID uint64
    Kind   MilestoneKind
    What   string
    Time   timing.VTimeInPicoSec
    // No Location — inherited from the task.
}
```

Notes:

- **Time is sourced from the domain, not the caller.** `NamedHookable` embeds
  `timing.TimeTeller`, and each emit function stamps the event's `Time` with
  `domain.CurrentTime()` *after* the `NumHooks()==0` guard. This is a change
  from the original "caller supplies `Time`" plan: passing `domain.CurrentTime()`
  as a call-site argument is evaluated eagerly, which (a) defeats the
  cheap-when-disabled guarantee and (b) panics the many unit tests that build
  components with `WithEngine(nil)`. Sourcing the time inside the guard keeps
  tracing free when no tracer is attached and leaves those tests untouched.
  Callers therefore pass **no** time; the `Time` fields exist only for the
  emit→tracer handoff. `TraceReq*` helpers take `(domain, msg, ...)` with no
  `now`.
- `Location` field on `TaskStart` **subsumes** `StartTaskWithSpecificLocation`
  (set it for network tracing, omit it otherwise). That function is deleted.
- `Kind` stays `string`. Each component package may declare its own constants
  (`const TaskKindInstruction = "instruction"`) but `tracing` does not own an
  enum.
- The `MsgIDAtReceiver` / `ForgetMsgIDAtReceiver` registry functions are
  unchanged (see §5).

### 3.2 Consume API

The same event structs flow all the way through — caller → emit function → hook
item → tracer — so no event is awkwardly packed into a `Task`:

```go
type Tracer interface {
    StartTask(t TaskStart)
    EndTask(t TaskEnd)
    AddTaskTag(tag TaskTag)
    AddMilestone(m Milestone)
}

// Embeddable no-op base so a tracer implements only what it needs.
type NopTracer struct{}
func (NopTracer) StartTask(TaskStart)   {}
func (NopTracer) EndTask(TaskEnd)       {}
func (NopTracer) AddTaskTag(TaskTag)    {}
func (NopTracer) AddMilestone(Milestone){}
```

Because each event already carries `Time`, **no tracer needs a `TimeTeller`**.
`TotalTimeTracer`, `AverageTimeTracer`, and `BusyTimeTracer` keep an inflight
map keyed by task ID and compute `EndTime - StartTime` from the event
timestamps; their shared bookkeeping is extracted into a small common base.

`Task` remains the **aggregate record** that stateful tracers (`DBTracer`,
`BackTraceTracer`) build from the event stream. Its `Steps` field is renamed to
`Tags`:

```go
type Task struct {
    ID, ParentID uint64
    Kind, What, Location string
    StartTime, EndTime   timing.VTimeInPicoSec
    Tags       []TaskTag       // was: Steps []TaskStep
    Milestones []Milestone
    Detail     any
    ParentTask *Task
}

type TaskFilter func(t TaskStart) bool   // applied at StartTask
```

`CollectTrace(domain, tracer)` is unchanged.

### 3.3 Naming: `step` → `tag`

"Steps" are categorical labels a component attaches to a task — `"write-hit"`,
`"read-mshr-hit"`, `"hit"` — not sequential steps. Rename the whole concept:

| Today | New |
|---|---|
| `AddTaskStep` | `AddTaskTag` |
| `TaskStep` (`{Time, What}`) | `TaskTag` |
| `Task.Steps` | `Task.Tags` |
| `HookPosTaskStep` | `HookPosTaskTag` |
| `Tracer.StepTask` | `Tracer.AddTaskTag` |
| `StepCountTracer`, `GetStepNames`, `GetStepCount` | `TagCountTracer`, `GetTagNames`, `GetTagCount` |

This also resolves the old `StepTask`-vs-`AddTaskStep` asymmetry: emit and
consume both become `AddTaskTag`.

### 3.4 Database schema & location interning

Location is the component name — long, hierarchical, and repeated across the
largest tables. The `datarecording` package already supports interning a
`string` field via `akita_data:"location"`: the row stores a small integer id
and the string is written once into a shared `location(ID, Locale)` table.
Today `DBTracer` instead tags `Location` as `index`, storing the full string in
every row. We switch to interning, and drop `Location` where it is redundant.

Two supporting changes in `datarecording`:

- **Always index `location` columns.** Extend `createIndexesForTable` to build
  an index on `location`-tagged (now integer) fields, so we keep fast location
  filtering. (Tags are mutually exclusive, so a field can't be both `location`
  and `index`; this makes `location` imply an index.)
- **Index the dictionary.** Make `location.ID` a primary key so the
  restore-join is fast.

Resulting tables:

| Table | Columns |
|---|---|
| **trace** | `ID` unique · `ParentID` idx · `Kind` idx · `What` idx · **`Location` location (auto-indexed)** · `StartTime` idx · `EndTime` idx |
| **milestone** | `ID` unique · `TaskID` idx · `Time` idx · `Kind` idx · `What` idx — **no Location** |
| **tag** (new) | `ID` unique · `TaskID` idx · `Time` idx · `What` idx — **no Location** |

Only `trace` carries `Location` (interned). Milestones and tags inherit their
location from the owning task via `TaskID`, so they store no location at all —
*smaller* than interning.

`What` and `Kind` stay plain indexed strings; we deliberately do not intern
them.

Daisen's reader (which queries the SQLite directly, not through the
`DataReader`) must change in lockstep:

- `trace` reads `JOIN location ON trace.Location = location.ID` to recover the
  string.
- The component list (`SELECT DISTINCT Location FROM trace`) becomes "read the
  `location` table" — cheaper.
- Milestone reads drop `Location`; daisen sets it from the parent task it has
  already loaded.
- New: daisen reads the `tag` table (tags are persisted for the first time).

### 3.5 Hooking: keep it, but type-check

Hooking stays as the transport. The one wart — `traceHook.Func` doing unchecked
`ctx.Item.(Task)` assertions — is fixed locally with checked assertions that
panic with a clear message naming the position and actual type. `TraceReqFinalize`
gains the `NumHooks()==0` guard that every other emit function already has, for
uniformity.

---

## 4. Migration plan

Three phases. Each keeps `go build ./...` and the package suites
(`tracing`, `mem/...`, `noc/...`) green, and keeps daisen working.

**Phase 1 — non-breaking cleanups.** Checked assertions in `traceHook`; add the
`NumHooks` guard to `TraceReqFinalize`. No call-site or schema churn.

**Phase 2 — emit & consume API redesign (one sweep).** This is the wide,
scriptable codemod; doing the rename and the struct change together avoids
touching call sites twice.
- Introduce the `TaskStart` / `TaskEnd` / `TaskTag` structs and the new
  `(domain, struct)` signatures; delete `StartTaskWithSpecificLocation`.
- Rename `step` → `tag` everywhere (§3.3).
- Move the `Tracer` interface to the event structs; add `NopTracer`; drop every
  tracer's `TimeTeller`; extract the shared inflight base.
- Drop the `location` argument from `AddMilestone` and the `Location` field from
  `Milestone`. The `DBTracer` keeps populating the (still-present) milestone DB
  column from the running task's location, so daisen is unaffected this phase.
- Add the `tag` table to `DBTracer` and write tags on `EndTask` (gated by the
  same `toRecord` window as milestones). Tags become persisted for the first
  time.

**Phase 3 — DB size: location interning (lands with daisen).**
- `datarecording`: always-index `location` columns; primary-key the `location`
  table (§3.4).
- `DBTracer`: switch `trace.Location` to `akita_data:"location"`; remove the
  `Location` column from the milestone table; the new `tag` table has none.
- daisen: JOIN `location` for tasks; component list from the `location` table;
  inherit milestone location from the task.

Ordering rationale: Phase 1 is free and lands anytime; Phase 2 is purely inside
the Go API (daisen untouched); Phase 3 is the only phase that changes the
on-disk schema and therefore must ship together with the daisen reader.

---

## 5. Deferred — separate discussions

These are explicitly **out of scope** for this plan and parked for their own
discussions. The current API for both is **kept unchanged**.

### Receiver-ID registry (reference)

`registry.go` exists so a receiver can derive a **stable, unique** task ID for
an incoming message **without mutating the message**. The scenario:

- The sender emits a `req_out` task whose ID *is* the message ID
  (`msg.Meta().ID`), fixed at construction.
- The receiver emits a `req_in` task for *its* handling. It needs its own unique
  ID, with `ParentID = msg.Meta().ID` to link the two into a tree.

`MsgIDAtReceiver` keeps a process-global, mutex-guarded map
`(domain.Name(), msg.ID) → generated taskID`:
1. `TraceReqReceive` → first lookup generates and stores the id; `StartTask`
   uses it.
2. `AddTaskTag` / `AddMilestone` → same key returns the **same** id, so every
   event lands on that receiver task.
3. `TraceReqComplete` → `EndTask`, then `forget` deletes the entry.

Keyed by domain name because one message flows through many components, each
needing its own handling task. When `NumHooks()==0` it returns `0` and never
touches the map.

Known smells to revisit later: process-global lock contention, coupling across
simulations in one process, keying by name, and leak risk if a `forget` is
missed. A likely direction is deriving the id deterministically
(`hash(domainName, msgID)`), which removes the map, the mutex, and the `forget`
calls — but that is a separate discussion.

### Context / task handle (reference)

Whether to reduce the manual threading of `(id, domain)` into every
`AddTaskTag` / `AddMilestone` / `EndTask` — e.g. via a value handle returned by
`StartTask` — is deferred. Note: goroutine-local `context.Context` propagation
is the wrong fit for a discrete-event simulator (one goroutine interleaves many
unrelated tasks, and the parallel engine spreads a task across goroutines), so
any solution here must stay explicit and data-driven.

---

## 6. Risks & open questions

- **Daisen lockstep (Phase 3).** The location/schema change must land with the
  daisen reader update or the UI breaks. This is the main coordination risk.
- **Tag unique key.** The plan gives `TaskTag` a generated `ID` (symmetry with
  milestones). Alternative: drop the synthetic id and rely on SQLite's rowid
  with a `(TaskID, What, Time)` index — avoids spending ID-generator values on
  high-volume tags. Decide during Phase 2.
- **`Kind` as `string`.** Confirmed: no named type. Components own their own
  constants if they want them.
- **Location index restore cost.** With `location.ID` primary-keyed and the
  dictionary tiny, the JOIN is cheap; verify on a large trace.
- **`Detail any`.** Still `json:"-"` (in-memory only, never persisted).

---

## 7. Summary of decisions

| Area | Decision |
|---|---|
| Task kind | Plain `string`; no enum |
| Parameters | `(domain, struct)`; `TaskStart` / `TaskEnd` / `TaskTag` / `Milestone` |
| Time | Sourced from `domain.CurrentTime()` inside the NumHooks guard (NamedHookable is a TimeTeller); tracers drop their own `TimeTeller` |
| Steps | Renamed to **tags**; persisted in a new `tag` table |
| Milestone location | **Removed** (inherited from task) |
| Tag location | None (inherited from task); has a `What` field |
| `trace.Location` | Interned via `datarecording` location table |
| Location columns | Always indexed; dictionary primary-keyed |
| `What` / `Kind` | Plain indexed strings; not interned |
| Hooking | Kept; checked assertions; uniform `NumHooks` guard |
| Registry | **Kept as-is**; redesign deferred |
| Context/handle | **Deferred** |
| Backward compat | Not preserved; daisen updated in lockstep |
