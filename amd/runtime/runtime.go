package runtime

import (
	"fmt"
	"io"
	"slices"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/runtime/internal"
)

// Runtime is the host API and the shared command resource. It never advances
// simulation time and retains no component, engine, or host-result pointers.
type Runtime struct {
	name      string
	config    RuntimeSpec
	storage   *mem.Storage
	codeIndex map[codeKey][]int
	pageTable *vm.PageTable
	allocator internal.MemoryAllocator
	devices   []*internal.Device
	state     runtimeState
}
type codeAddress struct {
	Queue   CommandQueue
	PID     vm.PID
	GPU     int
	Code    insts.CodeObjectValue
	Address Ptr
}
type runtimeState struct {
	SimulationTask           uint64
	TraceStarted, TraceEnded bool
	NextPID                  uint64
	NextCommand              CommandID
	Contexts                 []contextData
	Queues                   []queueData
	Events                   map[Event]eventState
	Codes                    []codeAddress
}
type runtimeCheckpoint struct {
	State     runtimeState
	Allocator internal.AllocatorState
}

// NewRuntime constructs and registers the shared resource before Driver.Build.
// spec must describe the same immutable device topology as the driver.
func NewRuntime(
	reg modeling.Simulation, name string, spec RuntimeSpec, pageTable *vm.PageTable, storage *mem.Storage,
) *Runtime {
	if reg == nil || pageTable == nil || storage == nil {
		panic("driver: runtime needs simulation, page table and storage")
	}
	r := &Runtime{name: name, config: spec, pageTable: pageTable, storage: storage,
		allocator: internal.NewMemoryAllocator(pageTable, spec.Log2PageSize),
		state:     runtimeState{Events: make(map[Event]eventState)}}
	r.config.Devices = slices.Clone(spec.Devices)
	r.addDevice(internal.DeviceTypeCPU, DeviceProperties{DRAMSize: 4 * mem.GB})
	for _, dev := range spec.Devices {
		r.addDevice(internal.DeviceTypeGPU, dev)
	}
	reg.RegisterResource(r)
	return r
}
func (r *Runtime) addDevice(kind internal.DeviceType, properties DeviceProperties) {
	d := &internal.Device{ID: len(r.devices), Type: kind,
		Properties: internal.DeviceProperties(properties),
		MemState:   internal.NewDeviceMemoryState(r.config.Log2PageSize, internal.AllocatorType(r.config.Allocator))}
	d.SetTotalMemSize(properties.DRAMSize)
	r.allocator.RegisterDevice(d)
	r.devices = append(r.devices, d)
}
func (r *Runtime) Name() string { return r.name }
func (r *Runtime) SaveCheckpoint(w io.Writer) error {
	return modeling.WriteCheckpoint(
		w,
		r.config,
		runtimeCheckpoint{State: r.state, Allocator: internal.SnapshotAllocator(r.allocator, r.devices)},
		nil,
	)
}
func (r *Runtime) LoadCheckpoint(reader io.Reader) error {
	var s runtimeCheckpoint
	if err := modeling.ReadCheckpoint(reader, r.config, &s, nil); err != nil {
		return err
	}
	allocator, devices, err := internal.RestoreAllocator(s.Allocator, r.pageTable)
	if err != nil {
		return err
	}
	r.state = s.State
	r.allocator = allocator
	r.devices = devices
	r.codeIndex = nil
	return nil
}
func (r *Runtime) context(h Context) *contextData {
	if h == 0 || uint64(h) > uint64(len(r.state.Contexts)) {
		panic("driver: invalid context handle")
	}
	return &r.state.Contexts[h-1]
}
func (r *Runtime) queue(h CommandQueue) *queueData {
	if h == 0 || uint64(h) > uint64(len(r.state.Queues)) {
		panic("driver: invalid queue handle")
	}
	return &r.state.Queues[h-1]
}

// Init creates an address space and returns its stable handle.
func (r *Runtime) Init() Context {
	r.state.NextPID++
	r.state.Contexts = append(r.state.Contexts, contextData{PID: vm.PID(r.state.NextPID), GPU: 1})
	return Context(len(r.state.Contexts))
}
func (r *Runtime) InitWithExistingPID(h Context) Context {
	pid := r.context(h).PID
	r.state.Contexts = append(r.state.Contexts, contextData{PID: pid, GPU: 1})
	return Context(len(r.state.Contexts))
}
func (r *Runtime) GetNumGPUs() int { return len(r.config.Devices) }
func (r *Runtime) SelectGPU(h Context, gpu int) {
	if gpu < 1 || gpu >= len(r.devices) {
		panic(fmt.Sprintf("GPU %d is not available", gpu))
	}
	r.context(h).GPU = gpu
}
func (r *Runtime) CreateUnifiedGPU(_ Context, gpuIDs []int) int {
	if len(gpuIDs) == 0 {
		panic("must unify at least 1 GPU")
	}
	dev := &internal.Device{ID: len(r.devices), Type: internal.DeviceTypeUnifiedGPU, UnifiedGPUIDs: slices.Clone(gpuIDs),
		MemState: internal.NewDeviceMemoryState(r.config.Log2PageSize, internal.AllocatorType(r.config.Allocator))}
	for _, id := range gpuIDs {
		if id < 1 || id >= len(r.devices) || r.devices[id].Type != internal.DeviceTypeGPU {
			panic("can only unify GPUs")
		}
		dev.ActualGPUs = append(dev.ActualGPUs, r.devices[id])
	}
	r.devices = append(r.devices, dev)
	r.allocator.RegisterDevice(dev)
	return dev.ID
}
func (r *Runtime) CreateCommandQueue(h Context) CommandQueue {
	gpu := r.context(h).GPU
	r.state.Queues = append(r.state.Queues, queueData{Context: h, GPU: gpu})
	return CommandQueue(len(r.state.Queues))
}
func (r *Runtime) AllocateMemory(h Context, size uint64) Ptr {
	c := r.context(h)
	ptr := Ptr(r.allocator.Allocate(c.PID, size, c.GPU))
	c.Buffers = append(c.Buffers, buffer{Address: ptr, Size: size})
	return ptr
}
func (r *Runtime) AllocateUnifiedMemory(h Context, size uint64) Ptr {
	c := r.context(h)
	ptr := Ptr(r.allocator.AllocateUnified(c.PID, size))
	c.Buffers = append(c.Buffers, buffer{Address: ptr, Size: size})
	return ptr
}

// FreeMemory releases storage immediately. Call only after the batch using it
// has globally completed. Deferred benchmark frees belong to its continuation.
func (r *Runtime) FreeMemory(h Context, ptr Ptr) error {
	c := r.context(h)
	for i := range c.Buffers {
		b := &c.Buffers[i]
		if b.Address != ptr {
			continue
		}
		r.allocator.Free(c.PID, uint64(ptr), b.Size)
		c.Buffers = append(c.Buffers[:i], c.Buffers[i+1:]...)
		return nil
	}
	return fmt.Errorf("runtime: unknown allocation %d", ptr)
}
func (r *Runtime) Remap(h Context, addr, size uint64, device int) {
	r.allocator.Remap(r.context(h).PID, addr, size, device)
}
func (r *Runtime) Distribute(h Context, addr Ptr, size uint64, gpus []int) []uint64 {
	if len(gpus) == 1 {
		return []uint64{size}
	}
	d := distributorImpl{memAllocator: r.allocator, pageSizeAsPowerOf2: r.config.Log2PageSize}
	return d.Distribute(r.context(h), uint64(addr), size, gpus)
}
func (r *Runtime) enqueue(q CommandQueue, c Command) CommandID {
	r.state.NextCommand++
	c.ID = r.state.NextCommand
	queue := r.queue(q)
	queue.Pending = append(queue.Pending, c)
	queue.Unfinished++
	return c.ID
}

// EnqueueMemCopyH2D reads CPU storage when the command executes. The caller
// must keep its source allocated and unchanged until the stream completes.
func (r *Runtime) EnqueueMemCopyH2D(q CommandQueue, dst, src Ptr, size int) CommandID {
	if size < 0 {
		panic("runtime: negative copy size")
	}
	return r.enqueue(q, Command{Kind: CopyH2D, Dst: dst, Src: src, ByteSize: size})
}

// EnqueueMemCopyD2H writes CPU storage. Read/reuse/free the destination only
// after completion; Runtime does not track host buffer occupancy.
func (r *Runtime) EnqueueMemCopyD2H(q CommandQueue, dst, src Ptr, size int) CommandID {
	if size < 0 {
		panic("runtime: negative copy size")
	}
	return r.enqueue(q, Command{Kind: CopyD2H, Dst: dst, Src: src, ByteSize: size})
}
func (r *Runtime) EnqueueRecordEvent(q CommandQueue) Event {
	e := Event(r.enqueue(q, Command{Kind: RecordEvent}))
	r.state.Events[e] = eventState{}
	return e
}
func (r *Runtime) EventTime(e Event) (timing.VTimeInPicoSec, error) {
	t, ok := r.state.Events[e]
	if !ok || !t.Triggered {
		return 0, fmt.Errorf("runtime: event %d is released or has not triggered", e)
	}
	return t.Time, nil
}

// Unfinished returns a snapshot in queue-registration order, including work
// whose ownership has already moved into Driver.State.
func (r *Runtime) Unfinished() []uint64 {
	out := make([]uint64, len(r.state.Queues))
	for i := range out {
		out[i] = r.state.Queues[i].Unfinished
	}
	return out
}

// TakeCommand transfers the next command to Driver execution state.
func (r *Runtime) TakeCommand(q CommandQueue) (Command, bool) {
	queue := r.queue(q)
	if len(queue.Pending) == 0 {
		return Command{}, false
	}
	c := queue.Pending[0]
	queue.Pending[0] = Command{}
	queue.Pending = queue.Pending[1:]
	return c, true
}

// CompleteCommand releases Driver-owned work and any internal staging allocations.
func (r *Runtime) CompleteCommand(q CommandQueue, c Command, now timing.VTimeInPicoSec) {
	queue := r.queue(q)
	if queue.Unfinished == 0 {
		panic("runtime: duplicate completion")
	}
	queue.Unfinished--
	for _, ptr := range c.Owned {
		if err := r.FreeMemory(queue.Context, ptr); err != nil {
			panic(err)
		}
	}
	if c.Kind == RecordEvent {
		if _, live := r.state.Events[Event(c.ID)]; live {
			r.state.Events[Event(c.ID)] = eventState{Triggered: true, Time: now}
		}
	}
}

// BeginSimulationTrace records the host run's task handle in the shared resource.
// The owner and tracker are borrowed for this call and never retained.
func (r *Runtime) BeginSimulationTrace(tracker *tasks.Tracker, owner tasks.Owner) {
	if r.state.TraceStarted {
		return
	}
	r.state.TraceStarted = true
	if tracker != nil {
		r.state.SimulationTask = tracker.StartTask(owner, tasks.StartInfo{Kind: "Simulation", What: "Simulation"})
	}
}

// EndSimulationTrace completes the host run's trace once all workloads end.
func (r *Runtime) EndSimulationTrace(tracker *tasks.Tracker) {
	if r.state.TraceEnded {
		return
	}
	if tracker != nil && r.state.SimulationTask != 0 {
		tracker.EndTask(r.state.SimulationTask)
	}
	r.state.TraceEnded = true
}

type eventState struct {
	Triggered bool
	Time      timing.VTimeInPicoSec
}

// ReleaseEvent invalidates a handle, including a pending marker. It does not
// cancel stream work. Completion cannot recreate a released timestamp.
func (r *Runtime) ReleaseEvent(e Event) error {
	if _, ok := r.state.Events[e]; !ok {
		return fmt.Errorf("runtime: unknown or released event %d", e)
	}
	delete(r.state.Events, e)
	return nil
}
