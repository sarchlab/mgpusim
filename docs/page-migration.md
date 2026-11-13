# Page migration

MGPUSim models migration of unified memory between GPUs, after the NVIDIA UVM
driver. Pages migrate on faults, the default
([sarchlab/akita#589](https://github.com/sarchlab/akita/issues/589)), or on
access-counter notifications
([sarchlab/akita#590](https://github.com/sarchlab/akita/issues/590)). Every
interaction is a message and all mutable data lives in State, so checkpoints
save migrations in flight and the inspector shows every connection.

Only pages allocated with `AllocateUnifiedMemory` migrate. A page is always a
page of the system page size, 4 KiB on R9 Nano and 2 MiB on MI300X: faults,
access counters, prefetching, thrashing, pins, and releases all work on it.
There is no separate migration unit.

## Configuration

Page migration is part of the system specification, not of the command line.
The timing platform migrates pages on faults by default:

```bash
./fir -timing -gpus=1,2 -use-unified-memory
```

The policy and its parameters are `driver.PageMigrationSpec`, the
`PageMigration` record of the driver's Spec. `driver.DefaultSpec()` holds the
defaults, which follow UVM; a platform changes them with
`timingconfig.Builder.WithPageMigration`:

```go
spec := driver.DefaultPageMigrationSpec()
spec.Policy = driver.PageMigrationCounter // or PageMigrationNone
platform := timingconfig.MakeBuilder().WithPageMigration(spec)
```

| Field | Default | Meaning |
|---|---|---|
| `Policy` | `fault` | `fault`, `counter`, or `none` |
| `BatchSize` | 256 | most requests the driver services in one batch (`uvm_perf_fault_batch_count`) |
| `QueueSize` | 1024 | entries of the request queue: a notification that needs one is taken in only while the queue has fewer, and waits in the port, and at its sender, otherwise (see [The queue is finite](#the-queue-is-finite)) |
| `AccessCounterThreshold` | 256 | remote demand accesses of one GPU to one physical page frame that notify the driver with `counter`, whatever the system page size; 0 turns the counters off (`uvm_perf_access_counter_threshold`, see [Access-counter migration](#access-counter-migration)) |
| `PrefetchThreshold` | 51 | a migration takes the rest of the largest aligned region of its 2 MiB block along in which more than this percent of the page positions are on the destination; 0 turns prefetching off (`uvm_perf_prefetch_threshold`) |
| `ThrashingThreshold` | 3 | thrashing events that make a page thrashing (`uvm_perf_thrashing_threshold`) |
| `ThrashingLapseNS` | 500 000 | a migration this soon after the previous one of the page is a thrashing event (`uvm_perf_thrashing_lapse_usec`) |
| `ThrashingNapNS` | 500 000 | how long a GPU that asks for a thrashing page is throttled (`uvm_perf_thrashing_nap`) |
| `ThrashingEpochNS` | 2 000 000 000 | a page forgets its thrashing when nothing happens to it for this long (`uvm_perf_thrashing_epoch`) |
| `PinThreshold` | 10 | throttling operations in a row that pin a page (`uvm_perf_thrashing_pin_threshold`) |
| `PinNS` | 150 000 000 | how long a page stays pinned (`uvm_perf_thrashing_pin`) |
| `DataMoverPages` | 64 | page copies that the data mover of a GPU keeps in flight |

### Statistics

With `-report-all`, a platform with more than one GPU reports the statistics
of the driver:

| Statistic | Meaning |
|---|---|
| `pm_faults` | fault notifications that the driver took in |
| `pm_coalesced_faults` | of those, faults of a GPU that already waited on the page |
| `pm_retained_faults` | faults that arrived while their page was serviced and waited on the page |
| `pm_deferred_faults` | pages whose faults a batch passed over because a memory copy used the page, counted once for each time they waited for a copy |
| `pm_notifications` | access-counter notifications that the driver took in |
| `pm_coalesced_notifications` | of those, notifications of a GPU for a frame that were already queued |
| `pm_ignored_notifications` | notifications whose page did not move: it was on the notifier, serviced, pinned, not unified, or used by a memory copy |
| `pm_free_frame_notifications` | notifications for a frame that no page mapped when they were serviced |
| `pm_throttled_notifications` | notifications of a GPU that was throttled on the page |
| `pm_queue_full_waits` | times a fault or a notification stayed in the port because the queue was full |
| `pm_batches` | batches that serviced a request |
| `pm_windows` | windows: migrations of a triggering page with the pages prefetched with it |
| `pm_migrations` | pages moved for a fault or a notification |
| `pm_pages_prefetched` | pages moved with the window of another page |
| `pm_pages_migrated` | pages moved: `pm_migrations` plus `pm_pages_prefetched` |
| `pm_bytes_migrated` | bytes of the pages moved |
| `pm_frames_retired` | frames that pages left |
| `pm_releases` | services that moved nothing: hold and release |
| `pm_thrashing_events` | migrations of a page within `ThrashingLapseNS` of its previous one, counted until the page is thrashing |
| `pm_thrashing_pages` | times a page became thrashing |
| `pm_throttles` | throttling operations |
| `pm_pins` | times a page was pinned |
| `pm_unpins` | pins that timed out |
| `pm_migration_time_sum` | summed time of the migrations, each from its Hold to its last Release answer, in seconds |
| `pm_copy_time_sum` | summed time of the page copies, each from its submission to the answer of the data mover, in seconds |
| `pm_shootdown_time_sum` | summed time of the shootdowns, in seconds |

An access-counter notification that the driver took in either repeats one
that is queued (`pm_coalesced_notifications`) or is serviced. When it is
serviced, it names a free frame, is ignored, or comes from a throttled GPU
(`pm_free_frame_notifications`, `pm_ignored_notifications`,
`pm_throttled_notifications`); otherwise it starts a window or pins its page.

## Components

| Component | Role |
|---|---|
| MMU (Akita) | A walk of a unified page that is on another GPU and not pinned faults: the MMU answers at once with a fault response and notifies `Driver.MigrationNotification` from its `Notification` port. It never holds a walk, and keeps the notifications that the driver does not take yet. |
| TLBs (Akita) | Forward a fault response without caching it. |
| Address translators (Akita) | Hold faulted accesses and accesses to held pages in a list that uses no transaction slot; a Hold acknowledges once no access to the page is outstanding and no translation of it is in flight, without waiting for the Holds of other pages; the page stays held until a Release, which translates the held accesses again. |
| RDMA engines | With `counter`, count the requests on `RDMARequestInside` (the remote demand accesses of their GPU) per physical page frame. They also pass the reads of other GPUs' data movers from `MigrationOutside` to the memory below the L2 (`MigrationInside`). |
| Driver (`amd/driver/pagemigration*.go`) | Takes the notifications, decides in batches what each page does (target, prefetching, thrashing), allocates frames from the Runtime, updates the page table, and runs every page through its phases. |
| Command processor (`amd/timing/cp/migration.go`) | Turns each one-page command of the driver into control requests for its address translators, TLBs, or L2 banks, or into a request for its data mover. It works on the commands of different pages at the same time. |
| Data mover (Akita, one per GPU) | Pulls the pages that migrate to its GPU: it reads a page from the memory below the L2 of the GPU that has it, through that GPU's RDMA engine, and writes it to the memory below the L2 of its own GPU. It keeps `DataMoverPages` copies in flight. A GPU that a page leaves uses no data mover of its own for it. |
| Runtime allocator | Hands out the frames, from the free list or the buddy allocator. |

## A migration

GPU *g* accesses unified page *P*, resident on GPU *s*. Each page is serviced
on its own, with its own commands and acknowledgements: no page waits for the
service of another page to end, for a kernel, or for ordinary memory traffic.
The places where the platform orders the steps of different pages are listed
under [What is ordered](#what-is-ordered).

1. **Fault.** The MMU walk finds *P* on *s*, answers with a fault, and
   notifies the driver. *g*'s address translator holds the access.
2. **Batch.** The driver starts a batch as soon as requests wait: it takes
   what waits at that moment, at most `BatchSize`, and never waits to fill a
   batch.
3. **Hold.** Every GPU's command processor holds *P* in all its address
   translators. After the last acknowledgement, no access to *P* is
   outstanding anywhere: every earlier access has finished, with its writes
   acknowledged by the L2.
4. **Prepare.** *s* writes the dirty L2 lines of the old frame back (Flush).
   Each L2 bank drops the lines of the frame (Invalidate) once it has
   acknowledged the Flush, which follows the acknowledgement of the
   writebacks by the lower level. At the same time *g* drops the L2 lines of
   the new frame, which may be dirty leftovers of an earlier user of the
   frame. A bank acknowledges that Invalidate only when no writeback of a
   line of the frame is on its way anymore: a line that the L2 was already
   evicting has reached the memory below it, and has been acknowledged.
5. **Copy.** With both answers, *g*'s data mover pulls the page, around the
   L2 on both ends: it reads the old frame below *s*'s L2 and writes the new
   frame below *g*'s. That is the DRAM on R9 Nano and the MALL, which fronts
   DRAM for every access, on MI300X.
6. **Remap.** With the answer of the data mover, the driver updates the page
   table.
7. **Shootdown.** Every command processor drops the translation of *P* from
   its TLBs.
8. **Release.** Every command processor releases *P*; the held accesses
   translate again, to the new local frame.

The L2 of *g* is not invalidated again after the copy. Between the Invalidate
of step 4 and the Remap, no access can bring a line of the new frame into the
L2: no page maps the frame, *P* is held, and the copy does not go through the
L2.

Every step names one page by a half-open byte range
(`memcontrolprotocol.Req.HasRange`, `Range`), and the driver counts the
acknowledgements of every page separately.

While *P* is held no translation of it is in flight, so the page table changes
when no TLB can be refilled with the old mapping, and the shootdown that
follows drops every stale entry. The TLBs are shot down in rounds: a round
pauses the TLBs once, invalidates every page that waits, and enables them
again.

When nothing moves (the page is already on *g*, or it is pinned), the driver
tells *g*'s command processor to hold and then release the page; on the
replay, the MMU returns the local or the pinned remote translation. The Hold
is needed because the fault notification reaches the driver directly while the
fault response still travels through *g*'s TLBs: a Release that overtook it
would not clear the fault, and the access would wait for another Release. The
Hold acknowledges only after the response has arrived.

A memory copy never runs on a migrating page, and a page never migrates under
a running memory copy: its faults wait for the copy. A copy of another command
queue whose pages are idle starts. The faults that wait for a copy keep their
entries of the request queue and take no place in a batch, so the requests
that are in the queue behind them are serviced in the next batch. Their
entries still count: see [The queue is finite](#the-queue-is-finite).

## No fault is lost

A fault is an access that waits; losing one stalls a kernel forever. The
driver therefore never drops or ignores a fault.

- A fault of a GPU that already waits on the page is the same fault: the
  driver keeps one entry per process, page, and GPU. Pages are found by the
  whole process ID and the virtual page number, so pages of two processes at
  one address are never taken for each other.
- A fault that arrives while its page is serviced, in any phase including the
  Release, waits on the page. Only after the **last Release acknowledgement**
  of the page are the faults that wait considered again, against the mapping
  and the pin as they are then.
- The notifications arrive on their own port (`MigrationNotification`). When
  the queue is full, the driver leaves them in the port, and the MMU keeps
  the ones behind them. The answers of the command processors use another
  port and are never behind a notification.

### The queue is finite

The request queue has `QueueSize` entries, and there is no overflow queue
behind it. What is bounded is what the driver takes in:

- The driver takes the notifications from the port in the order they
  arrived. A notification needs an entry if it is a fault on a page that is
  neither queued nor serviced, or an access-counter notification that is not
  queued yet. It is taken in only while the queue has fewer than `QueueSize`
  entries.
- While the queue is full, that notification stays at the head of the port,
  and everything behind it stays behind it, whatever page it is for. The port
  fills, and the MMU and the RDMA engines keep what they cannot send. Nothing
  is dropped: the faults are serviced when entries are free again.
- An entry is free when a batch has serviced its request. A fault that waits
  for a memory copy is not serviced, so it keeps its entry until the copy
  ends. It takes no place in a batch: requests that are already in the queue
  behind it, for other pages, are serviced. But its entry counts: when such
  faults fill the queue, new notifications wait in the port, also those for
  other pages, until a copy ends.
- The driver queues a page again by itself for faults that it has taken in
  before, even if the queue is full, because those faults must not be lost.
  There are two cases. A fault that arrives while its page is serviced takes
  no entry and waits on the page; the page is queued when the service ends.
  The faults of throttled GPUs leave the queue when the GPUs are throttled
  and wait on the page; the page is queued when the nap of one of them ends.
  These are the only entries beyond `QueueSize`, at most one for each such
  page.

## Windows and the data mover

The pages that one fault or notification moves are a window: the triggering
page first, then the prefetched pages by address. Every page of a window is
its own request, with its own hold, flush, copy, and release.

Each GPU has one data mover, and a page is always pulled by the data mover of
the GPU that it moves to: for a migration from *s* to *g*, *g*'s command
processor gives the copy to *g*'s data mover, and *s*'s RDMA engine passes
its reads on to the memory below *s*'s L2. The copy takes one of the
`DataMoverPages` places of *g*'s data mover and none on *s*, so all the pages
that move to a GPU share its data mover, wherever they come from.

On the data mover of a GPU, a window is submitted completely before the next
window starts, and its pages are submitted in its order. Submitting is all
that is ordered: once submitted, a page completes on its own, so pages of
several windows are in the data mover together, and a later window enters it
while an earlier one is still being copied. A window to another GPU uses
that GPU's data mover and waits for nothing here.

## What is ordered

The pages are independent in what they wait for: a page never waits for the
service of another page to end. The platform still does some steps of
different pages in an order:

- **Queue and batch.** Notifications are taken in from the port in arrival
  order, while the queue has fewer than `QueueSize` entries, and a batch
  services at most `BatchSize` requests of the queue, in arrival order. A
  fault that waits for a memory copy does not hold back the requests behind
  it in the queue, but while such faults fill the queue, a new notification
  for any page waits in the port for the end of a copy.
- **Address translators.** An address translator acknowledges the Hold of a
  page once no access to the page is outstanding and no translation of it is
  in flight. It does not wait for the Holds of other pages: a Hold whose page
  is quiet is acknowledged before a Hold that arrived earlier and still waits
  for an access. Only the Holds of one process that have a page in common are
  acknowledged in the order they arrived. A page stays held after the
  acknowledgement, until its Release, and the driver waits for the Release
  acknowledgements of every GPU before it services the page again.
- **L2 banks.** The frames of different pages are maintained at the same
  time: a command processor sends the Flush or the Invalidate of a frame to
  every bank as soon as the command arrives, whatever the banks still do for
  other frames, and a bank gets the Invalidate that follows a Flush once that
  bank has acknowledged the Flush of that frame. What is ordered:
  - Frames that have a byte in common are maintained in the order their
    commands arrived, also behind a command that itself still waits.
    Migrations that are in flight together do not share a frame: a new frame
    comes from the allocator, and the frame that a page leaves is retired.
  - The cache flush of the driver, which drains and flushes the whole of
    every bank for a host copy, goes first. When it arrives, no maintenance
    of a frame starts anymore. The maintenance that has started runs to its
    end, each bank still gets the Invalidate that follows its Flush, and
    then the caches are drained, flushed, invalidated, and enabled. The
    maintenance that waited starts after that, in arrival order, also when
    another flush of the driver is next: that flush waits for it in turn, so
    neither keeps the other waiting for good.
  - A bank itself takes one request after the other, so the requests of
    several frames queue at a bank. A command processor sends all its cache
    control requests from one port: while a bank has more requests than its
    Control port holds, the requests behind them wait in that port, also the
    L1 invalidation of a kernel that starts.
- **Data movers.** A window is submitted completely before the next window to
  the same GPU starts (see above). A data mover keeps `DataMoverPages` copies
  in flight and completes them oldest first.
- **TLBs.** The shootdowns of a GPU run in rounds, each for all the pages that
  wait.
- **Memory copies.** A copy and the migration of a page that it uses exclude
  each other, as described above. The Flush and the copy requests of a host
  copy share a port of each command processor with the kernel launches and
  wait behind a launch that waits for a dispatcher (see
  [Limitations](#limitations)).
- **Naps.** On a thrashing page, a throttled GPU takes the page when its own
  nap is over, and of several such GPUs the one whose nap ended first (see
  [Thrashing](#thrashing)).

## Prefetching

As in UVM, a migration also prefetches
([sarchlab/akita#591](https://github.com/sarchlab/akita/issues/591)), for
faults and for access-counter notifications alike. The page positions of each
2 MiB block form a binary tree. Going up from the triggering page, the driver
selects the largest aligned region in which more than `PrefetchThreshold`
percent of **all the page positions** are on the destination or migrate to it,
counting the triggering page. A position without a unified page counts for
nothing, but it is a position. A page that is serviced without moving, held
and released where it is or unpinned, counts where it is.

The pages of that region move with the trigger if they exist and are eligible:
unified, on another GPU, not pinned, not thrashing, not serviced, not waited
for by a fault of their own, and not used by a memory copy. A page thrashes
no longer once nothing has happened to it for `ThrashingEpochNS`, whether it
is asked for itself or only a neighbor is. A triggering page that thrashes
does not widen its window.

A sequential sweep therefore faults on the first pages of each region and
prefetches the rest: after faults on pages 0 and 1, a fault on page 2 also
moves page 3; a fault on page 4 then moves pages 5 to 7; and so on, up to the
whole block. With 2 MiB pages, a block is one page and nothing is prefetched.
UVM also maps fully resident 64 KiB and 2 MiB regions with big pages after a
migration; MGPUSim does not, because its TLBs use one page size.

## Access-counter migration

With `Policy: counter`, the MMU raises no faults: a GPU accesses a unified
page on another GPU remotely, through its RDMA engine. Each RDMA engine counts
the remote demand accesses of its GPU per physical page frame. At the
threshold it clears the count and sends a one-way notification with the frame
and the GPU. No access waits for a notification.

`AccessCounterThreshold` is a number of accesses of one GPU to one frame, 256
by default, and does not depend on the system page size: a 4 KiB frame of
R9 Nano and a 2 MiB frame of MI300X both notify with the 256th access of a
GPU. The first 255 accesses of a GPU to a frame send nothing, the 256th sends
one notification, and the count starts over, so the next notification is the
512th access. An access is a request that reaches the RDMA engine from the
caches of its GPU, so accesses that repeat a line count, and a frame can
reach the threshold with fewer lines than accesses. Every GPU counts its own
accesses: two GPUs that access a frame 255 times each send nothing.

The engine counts frames, not pages. A frame that is freed and allocated again
keeps the count it had. The driver resolves the frame when it services the
notification: a frame that no page maps is skipped in silence, and a frame
that was allocated again is judged by the page that has it now. A notification
is ignored when that page is on the notifier, serviced, pinned, or not
unified. The data movers read below the L2 through `MigrationOutside`, not
`RDMARequestInside`, so copies are not counted.

The counters see only remote accesses: the resident GPU's own use is never
counted. When two GPUs use a page about equally, the page can move back and
forth; as in UVM, the thrashing policy bounds this.

## Thrashing

The policy follows the stages of UVM (`uvm_perf_thrashing.c`).

1. **Detection.** Every migration of a page, whether a fault, a notification,
   or a prefetch moves it, within `ThrashingLapseNS` of its previous migration
   is a thrashing event. A page with `ThrashingThreshold` events is thrashing.
2. **Throttling.** A GPU that asks for a thrashing page is throttled for
   `ThrashingNapNS`, unless it is the GPU selected to make progress. With
   faults, its access keeps waiting and its fault is kept; with counters, its
   notifications are skipped during the nap and its remote accesses go on.
   The GPU whose nap ends becomes the selected one and may migrate the page.

   Every GPU has a nap of its own on a page. It starts when that GPU is
   throttled, and a GPU that is throttled on the page earlier or later
   neither shortens nor extends it: if GPU 2 is throttled at time 0 and GPU 3
   half a nap later, GPU 2 may take the page after one nap and GPU 3 after
   one and a half. With faults, the driver wakes at the end of each nap by
   itself. When several GPUs wait and their naps are over, the GPU whose nap
   ended first takes the page, whichever of their faults arrived first; a GPU
   whose nap has not ended keeps waiting, even if its fault is the oldest.

   Between two GPUs, the page so stays for at least a nap where it is used.
   With more GPUs it goes on to the next GPU whose nap ends, so it can leave
   a GPU sooner than a nap after it arrived there. Every throttling operation
   of every GPU counts for the pin, which ends such a rotation.
3. **Pinning.** After `PinThreshold` throttling operations in a row, the page
   is pinned **where it is**: the page table marks it `IsPinned`, the GPUs
   that wait are released, also those whose nap has not ended, and they
   access the page remotely.
4. **Unpinning.** After `PinNS` the pin is cleared under a hold of every GPU,
   and the translations of the page are shot down, so the next access faults
   or is counted again. The timeout is background simulation work
   (`TickAtBackground`): it does not keep a simulation running that has
   nothing else to do.

## Frames

A page gets its new frame from the ordinary allocator of the destination, so
the frame may have been used and freed before. The caches of the GPU can still
hold lines of such a frame:

- The L2 lines are dropped by the migration itself, before the copy, and the
  copy waits for the lines of the frame that the L2 was already writing
  back.
- For the L1 lines, MGPUSim assumes that every kernel that used a freed
  allocation has finished, and that the first kernel to use a recycled frame
  launches after the free. The L1 data caches are invalidated when a kernel
  starts.
- A host copy reads and writes the memory below the caches. A buffer that
  gets a recycled frame, when it is allocated or when `Remap` or `Distribute`
  moves its pages to other GPUs, is therefore treated like a buffer that a
  kernel has written: the driver flushes the caches before a timed host copy
  into it. Frames are tracked by the GPU that has them: a page that is mapped
  through a unified GPU (`CreateUnifiedGPU`) is on the member GPU that gave
  the frame.

What the caches hold of a buffer belongs to its address space. The contexts
made with `InitWithExistingPID` share one: a kernel of any of them dirties the
buffers of all, and a timed host copy through any of them is flushed for a
dirty buffer of any other. Only the context that allocated a buffer frees it.
Address spaces with different process IDs do not affect each other.

A timed host copy of a page that has migrated goes through the GPU that has
the page now, which reads or writes the memory below its L2, where the data
mover put the page. The migration itself does not make the buffer dirty.

The frame that a page leaves is **retired**: it is not returned to the
allocator, so nothing that still names it, such as an L1 line of another GPU
that read the page remotely, can meet another page. `FreeMemory` returns the
frame that a page has when it is freed.

The allocator and the hardware share one physical layout: the CPU has the
first 4 GiB and GPU *i* has `[i × 4 GiB, (i+1) × 4 GiB)`. This is the range
that the L1 caches of GPU *i* take for local, that the other GPUs route to
its RDMA engine, for accesses and for page copies, and that the global storage
holds. The page at address 0, in the range of the CPU, is never allocated.
Because frames are retired, a run can reach the last frame of a GPU with
little memory in use.

### Magic memory copies

With `-magic-memory-copy`, the driver copies between the host and the global
storage itself, in no time and around the GPUs. Every host copy does, in
emulation and on the timing platforms alike, whatever the buffer has been
used for: a magic copy never flushes a cache and never goes through a GPU.

In emulation that is exact: the GPUs have no caches and read and write the
global storage themselves. On a timing platform it is **not coherent with
the caches**, and MGPUSim does not make it so. The global storage is the
DRAM. What a GPU has of a buffer can differ from it:

- **A download does not see dirty lines.** What a kernel wrote is in the L2
  of its GPU until the lines are written back. A magic download returns what
  the DRAM has: the contents from before the kernel, zeros for new memory.
  The output of a kernel therefore cannot be read back with magic copies,
  with or without page migration: a benchmark that verifies its results
  fails.
- **An upload does not reach cached lines.** A cache that has a line of the
  buffer keeps it, so the GPU goes on using the old contents, and a dirty L2
  line is written back over the upload later, for example when the page
  migrates. A page that got a recycled frame can have such lines from the
  last user of the frame.
- **On MI300X the MALL is one more cache.** The memory below the L2 is the
  MALL, a write-through cache in front of the DRAM. Writes that reach the
  MALL also reach the DRAM, so a magic download sees them. But a magic upload
  changes the DRAM below the MALL: a line that the MALL has keeps its old
  contents, for the L2, for timed copies, and for page copies, which all read
  the MALL. A page that migrates after such an upload takes the old lines
  along. On R9 Nano the memory below the L2 is the DRAM itself, and a page
  copy takes the upload.

Use magic copies where the host only loads memory that no GPU has touched
and results are not read back, for example to skip the time of an initial
upload. A simulation that reads results back, or that copies into memory in
use, needs timed copies, which is the default: they flush the caches when a
buffer may have lines in them and go through the GPU of each page.

## Tests

- `amd/driver`: the policy and the phases of a page, against a fixture that
  plays the MMU, the RDMA engines, and the command processors; memory copies
  and faults that wait for each other; timed copies that flush the caches
  and magic copies that are direct for the same dirty memory; pages of two
  processes at one address; three GPUs that are throttled on one page at
  different times, with faults and with notifications, through a pin, and
  through a checkpoint of the whole simulation.
- `amd/timing/cp`: the per-page commands; the L2 maintenance of frames that
  have no byte in common at the same time, with acknowledgements that arrive
  staggered, and of frames that have a byte in common in arrival order; a
  flush of the driver that arrives with maintenance in progress and
  waiting, also across a checkpoint; and two flushes with maintenance
  between them.
- `amd/timing/rdma`, `amd/runtime`, `amd/runtime/internal`: the frame
  counters and the path below the L2, the allocators, the physical range of
  each device, and recycled frames.
- `amd/samples/runner/timingconfig`: the wiring of both platforms; that the
  allocator, the caches, the RDMA engines, the data movers, and the storage
  agree on the first and the last page of every GPU; and that the RDMA
  engines of a platform with the default specification notify with the
  256th, and again with the 512th, access of a GPU to a frame.
- `amd/tests/pagemigration`: acceptance on complete R9 Nano and MI300X
  platforms with real kernels: page contents with dirty source lines, reused
  destination frames, and host uploads into reused frames (directly, after
  `Distribute`, and through a second context of the address space after
  `Remap`); what magic memory copies do around the L2 and the MALL; faults
  under backpressure and during the Release, two windows on a data mover, a
  sparse window, counter-triggered prefetching, throttling, pinning and
  timed unpinning, the kernels of two GPUs that are throttled at different
  times on a platform of three, a checkpoint in the middle of a copy,
  several GPUs fighting for the same pages, and the multi-GPU sample
  workloads with both policies. One test builds only the path of a page from
  the components of the platforms, with the DRAM and with the MALL below the
  L2, and holds back the writeback of an old line of the new frame: the copy
  does not start before the writeback is acknowledged. The tests run
  simulations and skip with `-short`; `-run '/r9nano'` and `-run '/mi300x'`
  select a platform, and `-slow` adds the two largest workloads.
- `amd/tests/acceptance -only-page-migration`: the sample binaries on several
  GPUs with unified memory.

## Limitations

- No big-page mapping after prefetching.
- Retired frames are never reused, so a long run that migrates a lot can run
  out of physical memory. With the buddy allocator, an allocation of several
  pages that was made as one block stays allocated as long as one of its
  frames is retired.
- A migration needs a free frame on the destination; there is no eviction.
- A page copy takes a place in the data mover of the destination only. The
  GPU that the page leaves serves the reads through its RDMA engine and has
  no limit of its own on the pages that leave it at one time.
- The L1 caches of the GPUs are not coherent with each other: a GPU that reads
  a page remotely, while it is pinned or with the counter policy, can keep a
  line that the owner has written since. This is not new with page migration.
- Magic memory copies are not coherent with the caches of a timing platform
  (see [Magic memory copies](#magic-memory-copies)).
- A timed host copy and faults can wait for each other through a saturated
  command processor. A command processor takes the kernel launches, the cache
  flushes, and the host copies of the driver from one port, in order. A launch
  that finds every dispatcher busy (8 per GPU by default) stays at the head of
  that port, and the requests behind it wait. The driver, for its part, keeps
  the faults on the pages of a host copy that has started until the copy ends.
  So when every kernel that occupies a dispatcher of a GPU waits for such a
  fault, and a further launch is queued on that GPU in front of the Flush or
  the copy request of that host copy, the kernels wait for the copy, the copy
  waits for the launch, and the launch waits for a kernel. Nothing breaks the
  cycle: the simulation runs out of events, and the runner reports the queues
  that stalled. It needs more kernels at once on a GPU than it has
  dispatchers, with all the running ones stalled on pages of a running copy.
  It follows from the two rules above and has not been reproduced in a run.
  This is accepted for now. Its remedy belongs with the stream and executor
  queues that are to live in GPU memory, where a launch that waits for a
  dispatcher no longer stands in front of the requests of other streams; the
  command processor is not redesigned for it here.
