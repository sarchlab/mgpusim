# Page migration

MGPUSim models fault-driven migration of unified memory between GPUs, the
default behavior of the NVIDIA UVM driver. The design is recorded in
[sarchlab/akita#589](https://github.com/sarchlab/akita/issues/589). Every
interaction is a message and all mutable data lives in State, so checkpoints
save migrations in flight and the inspector shows every connection.

Only pages allocated with `AllocateUnifiedMemory` migrate.

## Running

```bash
./fir -timing -gpus=1,2 -use-unified-memory -page-migration=fault
```

| Flag | Default | Meaning |
|---|---|---|
| `-page-migration` | `none` | `none` or `fault` |
| `-pm-unit-kb` | 4 | migration unit, a power of two from 4 to 2048 KB |
| `-pm-sw-latency-ns` | 5000 | time the driver collects a batch of faults |
| `-pm-batch` | 256 | most faults in a batch (`uvm_perf_fault_batch_count`) |
| `-pm-thrashing-lapse-ns` | 500000 | a migration this soon after the previous one is a thrashing event (`uvm_perf_thrashing_lapse_usec`) |
| `-pm-thrashing-threshold` | 3 | consecutive thrashing events that pin a unit (`uvm_perf_thrashing_threshold`) |
| `-pm-pin-ns` | 150000000 | how long a pinned unit stays pinned (`uvm_perf_thrashing_pin`) |

With `-report-all`, the driver reports `pm_faults`, `pm_migrations`,
`pm_pages_migrated`, `pm_bytes_migrated`, `pm_releases`, `pm_pins`, and the
summed migration, copy, and shootdown times.

## Components

| Component | Role |
|---|---|
| MMU (Akita) | A walk of a unified page that is on another GPU and not pinned faults: the MMU answers at once with a fault response and notifies `Driver.PageMigration` from its `Notification` port. It never holds a walk. |
| TLBs (Akita) | Forward a fault response without caching it. |
| Address translators (Akita) | Hold faulted accesses and accesses to held pages in a list that uses no transaction slot; `CmdHold` acks once no access to the pages and no translation of them is in flight; `CmdRelease` translates the held accesses again. |
| Driver (`amd/driver/pagemigration.go`) | Batches faults, chooses the target (the faulting GPU), allocates frames from the Runtime, updates the page table, detects thrashing, and pins. |
| Command processor (`amd/timing/cp/migration.go`) | Turns each driver command into a control sequence for its address translators, TLBs, or data mover. Migration sequences queue with cache flush and kernel start. |
| Data mover (Akita, one per GPU) | Copies a page: it reads the old frame from the source GPU's `RDMADataOutside` (through the source L2, so dirty lines are copied) and writes the new frame to its own DRAM controllers, around the L2 (and, on MI300X, the MALL). |
| Runtime allocator | Physical addresses only increase, so a new frame has never been used and no cache holds a line of it. |

## A migration

GPU *g* accesses unified page *P*, resident on GPU *s*.

1. **Fault.** The MMU walk finds *P* on *s*, answers with a fault, and
   notifies the driver. *g*'s address translator holds the access.
2. **Batch.** The driver collects faults for `-pm-sw-latency-ns`, up to
   `-pm-batch`; later faults go to the next batch, and a notification that
   finds both full is dropped, as a real fault buffer does on overflow. A
   unit that already migrates ignores the fault; its release lets the access
   go.
3. **Hold.** Every GPU's command processor holds the unit's pages in all its
   address translators.
4. **Copy.** The driver allocates frames on *g*; *g*'s data mover copies.
5. **Remap.** The driver updates the page table. The old frames are retired.
6. **Shootdown.** Every command processor pauses, invalidates, and enables
   all its TLBs, once.
7. **Release.** Every command processor releases the pages; the held accesses
   translate again, to the new local frame.

While the pages are held no translation of them is in flight, so the page
table changes when no TLB can be refilled with the old mapping, and the single
shootdown that follows drops every stale entry.

When nothing moves (the page is already on *g*, or the unit is pinned), the
driver tells *g*'s command processor to release the unit; on the replay, the
MMU returns the local or the pinned remote translation. A memory copy never runs on a
migrating unit, and a unit never migrates under a running memory copy.

## Thrashing

A migration within the lapse of the previous one is a thrashing event. After
the threshold of consecutive events, the unit is pinned: its pages are marked
`IsPinned`, the MMU returns the remote translation, and faults on it are
answered by a release without migration. When the pin expires, the driver
clears `IsPinned` and shoots down the unit's translations, so the next access
faults again. Unlike UVM, the faulting processor is not throttled before the
pin.

## Limitations

- No prefetching ([sarchlab/akita#591](https://github.com/sarchlab/akita/issues/591)):
  a sequential sweep faults once per unit.
- No access-counter migration ([sarchlab/akita#590](https://github.com/sarchlab/akita/issues/590)).
- Freed physical memory is never reused, so long runs that allocate and free
  often can run out of physical memory.
