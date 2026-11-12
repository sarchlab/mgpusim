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
| `BatchSize` | 256 | most requests the driver takes in one batch (`uvm_perf_fault_batch_count`) |
| `QueueSize` | 1024 | most requests that wait for a batch; beyond it, notifications stay with their senders |
| `AccessCounterThreshold` | 256 | remote accesses of a GPU to a frame that notify the driver with `counter`; 0 turns the counters off (`uvm_perf_access_counter_threshold`) |
| `PrefetchThreshold` | 51 | a migration takes the rest of the largest aligned region of its 2 MiB block along in which more than this percent of the page positions are on the destination; 0 turns prefetching off (`uvm_perf_prefetch_threshold`) |
| `ThrashingThreshold` | 3 | thrashing events that make a page thrashing (`uvm_perf_thrashing_threshold`) |
| `ThrashingLapseNS` | 500 000 | a migration this soon after the previous one of the page is a thrashing event (`uvm_perf_thrashing_lapse_usec`) |
| `ThrashingNapNS` | 500 000 | how long a GPU that asks for a thrashing page is throttled (`uvm_perf_thrashing_nap`) |
| `ThrashingEpochNS` | 2 000 000 000 | a page forgets its thrashing when nothing happens to it for this long (`uvm_perf_thrashing_epoch`) |
| `PinThreshold` | 10 | throttling operations in a row that pin a page (`uvm_perf_thrashing_pin_threshold`) |
| `PinNS` | 150 000 000 | how long a page stays pinned (`uvm_perf_thrashing_pin`) |
| `DataMoverPages` | 64 | page copies of one direction that a data mover keeps in flight |

With `-report-all`, a platform with more than one GPU reports the statistics
of the driver: `pm_faults`, `pm_coalesced_faults`, `pm_retained_faults`,
`pm_notifications`, `pm_queue_full_waits`, `pm_batches`, `pm_windows`,
`pm_migrations`, `pm_pages_prefetched`, `pm_pages_migrated`,
`pm_bytes_migrated`, `pm_frames_retired`, `pm_releases`,
`pm_thrashing_events`, `pm_thrashing_pages`, `pm_throttles`, `pm_pins`,
`pm_unpins`, and the summed migration, copy, and shootdown times.

## Components

| Component | Role |
|---|---|
| MMU (Akita) | A walk of a unified page that is on another GPU and not pinned faults: the MMU answers at once with a fault response and notifies `Driver.MigrationNotification` from its `Notification` port. It never holds a walk, and keeps the notifications that the driver does not take yet. |
| TLBs (Akita) | Forward a fault response without caching it. |
| Address translators (Akita) | Hold faulted accesses and accesses to held pages in a list that uses no transaction slot; a Hold acknowledges once no access to the page is outstanding and no translation of it is in flight; a Release translates the held accesses again. |
| RDMA engines | With `counter`, count the requests on `RDMARequestInside` (the remote demand accesses of their GPU) per physical page frame. They also pass the page copies of other GPUs' data movers from `MigrationOutside` to the memory below the L2 (`MigrationInside`). |
| Driver (`amd/driver/pagemigration*.go`) | Takes the notifications, decides in batches what each page does (target, prefetching, thrashing), allocates frames from the Runtime, updates the page table, and runs every page through its phases. |
| Command processor (`amd/timing/cp/migration.go`) | Turns each one-page command of the driver into control requests for its address translators, TLBs, L2 banks, or data movers. It works on the commands of different pages at the same time. |
| Data movers (Akita, two per GPU) | The inbound data mover copies pages from other GPUs to this one; the outbound one copies pages of this GPU to others. Each only ever copies in its direction, so neither waits to change direction, and each keeps `DataMoverPages` copies in flight. |
| Runtime allocator | Hands out the frames, from the free list or the buddy allocator. |

## A migration

GPU *g* accesses unified page *P*, resident on GPU *s*. Each page is serviced
on its own: no page waits for another one, for a kernel, or for ordinary
memory traffic, except for its turn at a data mover.

1. **Fault.** The MMU walk finds *P* on *s*, answers with a fault, and
   notifies the driver. *g*'s address translator holds the access.
2. **Batch.** The driver starts a batch as soon as requests wait: it takes
   what waits at that moment, at most `BatchSize`, and never waits to fill a
   batch.
3. **Hold.** Every GPU's command processor holds *P* in all its address
   translators. After the last acknowledgement, no access to *P* is
   outstanding anywhere: every earlier access has finished, with its writes
   acknowledged by the L2.
4. **Prepare.** *s* writes the dirty L2 lines of the old frame back and,
   after the acknowledgement of the Flush, which follows the acknowledgement
   of the writebacks by the lower level, drops the lines of the frame
   (Invalidate). On each L2 bank the pair runs alone: a bank does one
   maintenance operation at a time, and the cache flush of the driver waits
   for the banks. *g* drops the L2 lines of the new frame, which may be dirty
   leftovers of an earlier user.
5. **Copy.** *g*'s inbound data mover pulls the page, around the L2 on both
   ends: it reads the old frame below *s*'s L2 and writes the new frame below
   *g*'s. That is the DRAM on R9 Nano and the MALL, which fronts DRAM for
   every access, on MI300X.
6. **Invalidate.** *g* drops the L2 lines of the new frame again.
7. **Remap.** The driver updates the page table.
8. **Shootdown.** Every command processor drops the translation of *P* from
   its TLBs.
9. **Release.** Every command processor releases *P*; the held accesses
   translate again, to the new local frame.

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
a running memory copy: its faults wait for the copy.

## No fault is lost

A fault is an access that waits; losing one stalls a kernel forever. The
driver therefore never drops or ignores a fault.

- A fault of a GPU that already waits on the page is the same fault: the
  driver keeps one entry per process, page, and GPU.
- A fault that arrives while its page is serviced, in any phase including the
  Release, waits on the page. Only after the **last Release acknowledgement**
  of the page are the faults that wait considered again, against the mapping
  and the pin as they are then.
- The notifications arrive on their own port (`MigrationNotification`). When
  the queue is full, the driver leaves them in the port, and the MMU keeps
  the ones behind them. The answers of the command processors use another
  port and are never behind a notification.

## Windows and the data movers

The pages that one fault or notification moves are a window: the triggering
page first, then the prefetched pages by address. Every page of a window is
its own request, with its own hold, flush, copy, and release.

On the inbound data mover of a GPU, a window is submitted completely before
the next window starts, and its pages are submitted in its order. Submitting
is all that is ordered: once submitted, a page completes on its own, so pages
of several windows are in the data mover together, and a later window enters
it while an earlier one is still being copied. A window to another GPU uses
that GPU's inbound data mover and waits for nothing here.

## Prefetching

As in UVM, a migration also prefetches
([sarchlab/akita#591](https://github.com/sarchlab/akita/issues/591)), for
faults and for access-counter notifications alike. The page positions of each
2 MiB block form a binary tree. Going up from the triggering page, the driver
selects the largest aligned region in which more than `PrefetchThreshold`
percent of **all the page positions** are on the destination or migrate to it,
counting the triggering page. A position without a unified page counts for
nothing, but it is a position.

The pages of that region move with the trigger if they exist and are eligible:
unified, on another GPU, not pinned, not thrashing, not serviced, not waited
for by a fault of their own, and not used by a memory copy. A triggering page
that thrashes does not widen its window.

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
   The GPU whose nap ends becomes the selected one and may migrate the page,
   so the page stays for at least a nap where it is used.
3. **Pinning.** After `PinThreshold` throttling operations in a row, the page
   is pinned **where it is**: the page table marks it `IsPinned`, the GPUs
   that wait are released, and they access the page remotely.
4. **Unpinning.** After `PinNS` the pin is cleared under a hold of every GPU,
   and the translations of the page are shot down, so the next access faults
   or is counted again. The timeout is background simulation work
   (`TickAtBackground`): it does not keep a simulation running that has
   nothing else to do.

## Frames

A page gets its new frame from the ordinary allocator of the destination, so
the frame may have been used and freed before. The caches of the GPU can still
hold lines of such a frame:

- The L2 lines are dropped by the migration itself, before and after the
  copy.
- For the L1 lines, MGPUSim assumes that every kernel that used a freed
  allocation has finished, and that the first kernel to use a recycled frame
  launches after the free. The L1 data caches are invalidated when a kernel
  starts.
- A host copy writes memory around the caches. An allocation that gets a
  recycled frame is therefore treated like a buffer that a kernel has
  written: the driver flushes the caches before a host copy into it.

The frame that a page leaves is **retired**: it is not returned to the
allocator, so nothing that still names it, such as an L1 line of another GPU
that read the page remotely, can meet another page. `FreeMemory` returns the
frame that a page has when it is freed.

## Tests

- `amd/driver`: the policy and the phases of a page, against a fixture that
  plays the MMU, the RDMA engines, and the command processors.
- `amd/timing/cp`, `amd/timing/rdma`, `amd/runtime/internal`: the per-page
  commands, the frame counters and the path below the L2, and the allocators.
- `amd/tests/pagemigration`: acceptance on complete R9 Nano and MI300X
  platforms with real kernels: page contents with dirty source lines and
  reused destination frames, faults under backpressure and during the
  Release, two windows on a data mover, a sparse window, counter-triggered
  prefetching, throttling, pinning and timed unpinning, a checkpoint in the
  middle of a copy, several GPUs fighting for the same pages, and the
  multi-GPU sample workloads with both policies. The tests run full
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
- The L1 caches of the GPUs are not coherent with each other: a GPU that reads
  a page remotely, while it is pinned or with the counter policy, can keep a
  line that the owner has written since. This is not new with page migration.
