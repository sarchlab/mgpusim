# Page migration

MGPUSim models migration of unified memory between GPUs, after the NVIDIA UVM
driver. Pages migrate on faults, the UVM default
([sarchlab/akita#589](https://github.com/sarchlab/akita/issues/589)), or on
access-counter notifications
([sarchlab/akita#590](https://github.com/sarchlab/akita/issues/590)). Every
interaction is a message and all mutable data lives in State, so checkpoints
save migrations in flight and the inspector shows every connection.

Only pages allocated with `AllocateUnifiedMemory` migrate.

## Running

```bash
./fir -timing -gpus=1,2 -use-unified-memory -page-migration=fault
./fir -timing -gpus=1,2 -use-unified-memory -page-migration=counter
```

| Flag | Default | Meaning |
|---|---|---|
| `-page-migration` | `none` | `none`, `fault`, or `counter` |
| `-pm-unit-kb` | 4 | migration unit, a power of two from 4 to 2048 KB |
| `-pm-prefetch-threshold` | 51 | a fault also migrates the rest of the largest aligned sub-region of its 2 MB block in which more than this percent of the units are on the faulting GPU; 0 turns prefetching off (`uvm_perf_prefetch_threshold`) |
| `-pm-access-counter-threshold` | 256 | remote L1 misses of a GPU to a unit that request its migration with `counter`; 0 turns the counters off (`uvm_perf_access_counter_threshold`) |
| `-pm-sw-latency-ns` | 5000 | time the driver collects a batch of faults or notifications |
| `-pm-batch` | 256 | most faults or notifications in a batch (`uvm_perf_fault_batch_count`) |
| `-pm-thrashing-lapse-ns` | 500000 | a migration this soon after the previous one is a thrashing event (`uvm_perf_thrashing_lapse_usec`) |
| `-pm-thrashing-threshold` | 3 | consecutive thrashing events that pin a unit (`uvm_perf_thrashing_threshold`) |
| `-pm-pin-ns` | 150000000 | how long a pinned unit stays pinned (`uvm_perf_thrashing_pin`) |

With `-report-all`, the driver reports `pm_faults`, `pm_notifications`,
`pm_migrations`, `pm_pages_migrated`, `pm_pages_prefetched`,
`pm_bytes_migrated`, `pm_releases`,
`pm_pins`, and the summed migration, copy, and shootdown times.

## Components

| Component | Role |
|---|---|
| MMU (Akita) | A walk of a unified page that is on another GPU and not pinned faults: the MMU answers at once with a fault response and notifies `Driver.PageMigration` from its `Notification` port. It never holds a walk. |
| TLBs (Akita) | Forward a fault response without caching it. |
| Address translators (Akita) | Hold faulted accesses and accesses to held pages in a list that uses no transaction slot; `CmdHold` acks once no access to the pages and no translation of them is in flight; `CmdRelease` translates the held accesses again. |
| RDMA engines | With `counter`, count the requests on `RDMARequestInside` (the remote L1 misses of their GPU) per migration unit of physical memory. At the threshold, the counter is cleared and the `AccessCounter` port notifies `Driver.PageMigration`. |
| Driver (`amd/driver/pagemigration.go`) | Batches faults and notifications, chooses the target (the faulting or notifying GPU), allocates frames from the Runtime, updates the page table, detects thrashing, and pins. |
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
driver tells *g*'s command processor to hold and then release the unit; on
the replay, the MMU returns the local or the pinned remote translation. The
Hold is needed because the fault notification reaches the driver directly
while the fault response still travels through *g*'s TLBs: a Release that
overtook it would not clear the fault, and the access would wait for another
Release. The Hold acks only after the response has arrived. A memory copy never runs on a
migrating unit, and a unit never migrates under a running memory copy.

## Prefetching

As in UVM, a migration on a fault also prefetches
([sarchlab/akita#591](https://github.com/sarchlab/akita/issues/591)). The
migration units of each 2 MB block form a binary tree. Going up from the
faulting unit, the driver selects the largest aligned sub-region in which more
than `-pm-prefetch-threshold` percent of the allocated units are on the
faulting GPU or migrate to it, counting the faulting unit and the units of
earlier migrations still in flight. The other units of that sub-region move
with the faulting unit, in the same hold, copy, remap, shootdown, and release.

A sequential sweep therefore faults on the first units of each sub-region
and prefetches the rest: with 4 KB units, after faults on units 0 and 1, a
fault on unit 2 also moves unit 3; the region grows as the sweep goes on, up
to the whole block.

Units that are pinned, thrash (have a thrashing event since their last
migration), migrate, or are used by a memory copy are not prefetched. With a
2 MB unit, a unit is a whole block and nothing is prefetched. Access-counter
notifications do not prefetch. UVM also maps fully resident 64 KB and 2 MB
regions with big pages after a migration; MGPUSim does not, because its TLBs
use one page size.

## Access-counter migration

With `-page-migration=counter`, the MMU raises no faults: a GPU accesses a
unified page on another GPU remotely, through its RDMA engine. When the
GPU's remote L1 misses to a unit reach the threshold, its RDMA engine
notifies the driver with the physical address and the GPU. The driver maps
the address back to the unit and, in the next batch, migrates the unit to the
notifier with the same hold, copy, remap, shootdown, and release steps. No
access waits for a notification: until the hold, the page stays mapped
remotely.

The driver ignores a notification when the unit is already on the notifier,
is migrating, or is pinned, and when the page has moved since the access was
counted. The data mover reads the old frame through `RDMADataOutside`, not
`RDMARequestInside`, so copies are not counted.

The counters see only remote accesses: the resident GPU's own use is never
counted. When two GPUs use a page about equally, the page can move back and
forth, and a GPU that rarely uses a page can slowly reach the threshold and
take it from a GPU that uses it heavily. As in UVM, thrashing detection bounds
this.

UVM counts in 2 MB regions and migrates the touched 64 KB sub-regions;
MGPUSim counts per migration unit. The counter is cleared when the
notification is sent, not after it is serviced; the two differ only when the
driver does not migrate. Faults and counters are not combined: with faults
on, a GPU's first access to a remote page faults, so only pinned pages would
be accessed remotely, and the driver ignores notifications on pinned units.

## Thrashing

A migration within the lapse of the previous one is a thrashing event. After
the threshold of consecutive events, the unit is pinned: its pages are marked
`IsPinned`, the MMU returns the remote translation, and faults on it are
answered by a release without migration; with `counter`, notifications on it
are ignored. When the pin expires, the driver clears `IsPinned` and shoots
down the unit's translations, so the next access faults again. Unlike UVM, the
faulting processor is not throttled before the pin.

## Limitations

- No big-page mapping after prefetching.
- Freed physical memory is never reused, so long runs that allocate and free
  often can run out of physical memory.
