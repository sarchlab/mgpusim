# queueing

Package `queueing` provides generic `Buffer[T]` and `Pipeline[T]` data
structures designed for use as serializable fields in component State structs.

Both types use exported fields with `json` tags, making them directly
embeddable in `modeling.State` types for checkpoint/restore.

## Buffer[T]

A fixed-capacity FIFO queue with hook support.

```go
type MyState struct {
    Inbox queueing.Buffer[MyRequest] `json:"inbox"`
}

// Initialize
state.Inbox = queueing.Buffer[MyRequest]{
    BufferName: "inbox",
    Cap:        16,
}

// Use
if state.Inbox.CanPush() {
    state.Inbox.PushTyped(req)
}
fmt.Println(state.Inbox.Size(), state.Inbox.Capacity())
```

### Key Methods

| Method | Description |
|--------|-------------|
| `CanPush() bool` | True if the buffer has room |
| `Push(e interface{})` | Add element (panics if full) |
| `PushTyped(e T)` | Type-safe push (panics if full) |
| `Clear()` | Remove all elements |
| `Size() int` | Current number of elements |
| `Capacity() int` | Maximum capacity |
| `Name() string` | Buffer name (for hooks/monitoring) |

`Buffer[T]` also satisfies the `BufferState` interface for type-erased
inspection by monitoring and arbitration code.

### Hook Positions

- `HookPosBufPush` — triggered after an element is pushed.
- `HookPosBufPop` — triggered after an element is popped.

## Pipeline[T]

A multi-lane, multi-stage pipeline that models fixed-latency processing.
Items enter at stage 0 and advance one stage per tick until they exit from
the last stage into a post-buffer.

```go
type MyState struct {
    Pipe    queueing.Pipeline[MyItem]  `json:"pipe"`
    PostBuf queueing.Buffer[MyItem]    `json:"post_buf"`
}

// Initialize: 4 lanes, 3 stages
state.Pipe = queueing.Pipeline[MyItem]{
    Width:     4,
    NumStages: 3,
}
state.PostBuf = queueing.Buffer[MyItem]{
    BufferName: "post",
    Cap:        8,
}

// Each tick
if state.Pipe.CanAccept() {
    state.Pipe.Accept(item)
}
madeProgress := state.Pipe.Tick(&state.PostBuf)
```

### Key Methods

| Method | Description |
|--------|-------------|
| `CanAccept() bool` | True if a lane is free at stage 0 |
| `Accept(item T)` | Insert item into the first stage |
| `Tick(postBuf *Buffer[T]) bool` | Advance pipeline; completed items go to postBuf |
| `TickFunc(accept func(T) bool) bool` | Advance with custom output function |

### Pipeline Behavior

- Each item occupies one lane at one stage.
- Items advance from stage `i` to stage `i+1` if the same lane is free.
- Items at the last stage with `CycleLeft == 0` are output.
- Total latency = `NumStages` ticks.
- Stages are processed from last to first to prevent double-advancement.
