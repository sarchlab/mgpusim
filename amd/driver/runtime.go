package driver

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"slices"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/driver/internal"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
)

// Runtime is the host API and the shared command resource. It never advances
// simulation time and retains no component, engine, or host-result pointers.
type Runtime struct {
	name      string
	config    Spec
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
	Results                  map[CommandID][]byte
	Events                   map[Event]timing.VTimeInPicoSec
	Codes                    []codeAddress
}
type runtimeCheckpoint struct {
	State     runtimeState
	Allocator internal.AllocatorState
}

// NewRuntime constructs and registers the shared resource before Driver.Build.
// spec must describe the same immutable device topology as the driver.
func NewRuntime(reg modeling.Simulation, name string, spec Spec, pageTable *vm.PageTable) *Runtime {
	if reg == nil || pageTable == nil {
		panic("driver: runtime needs simulation and page table")
	}
	r := &Runtime{name: name, config: spec, pageTable: pageTable,
		allocator: internal.NewMemoryAllocator(pageTable, spec.Log2PageSize),
		state:     runtimeState{Results: make(map[CommandID][]byte), Events: make(map[Event]timing.VTimeInPicoSec)}}
	r.config.GPUs = slices.Clone(spec.GPUs)
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
		runtimeCheckpoint{r.state, internal.SnapshotAllocator(r.allocator, r.devices)},
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
func (r *Runtime) GetNumGPUs() int { return len(r.config.GPUs) }
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
		if b.Freed {
			return fmt.Errorf("runtime: allocation %d already freed", ptr)
		}
		r.allocator.Free(c.PID, uint64(ptr), b.Size)
		b.Freed = true
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
func (r *Runtime) EnqueueMemCopyH2D(q CommandQueue, dst Ptr, src any) CommandID {
	var b bytes.Buffer
	if err := binary.Write(&b, binary.LittleEndian, src); err != nil {
		panic(err)
	}
	return r.enqueue(q, Command{Kind: copyH2D, Dst: dst, Data: b.Bytes()})
}

// EnqueueMemCopyD2H retains bytes in Runtime until ReadResult consumes them.
func (r *Runtime) EnqueueMemCopyD2H(q CommandQueue, src Ptr, size int) CommandID {
	if size < 0 {
		panic("runtime: negative readback size")
	}
	return r.enqueue(q, Command{Kind: copyD2H, Src: src, ByteSize: size})
}

// ReadResult copies a completed readback to host memory and consumes it only
// after successful decoding. It never waits or runs simulation.
func (r *Runtime) ReadResult(id CommandID, dst any) error {
	data, ok := r.state.Results[id]
	if !ok {
		return fmt.Errorf("runtime: result %d is not available", id)
	}
	if binary.Size(dst) != len(data) {
		return fmt.Errorf("runtime: result %d size mismatch", id)
	}
	if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, dst); err != nil {
		return err
	}
	delete(r.state.Results, id)
	return nil
}
func (r *Runtime) EnqueueRecordEvent(q CommandQueue) Event {
	return Event(r.enqueue(q, Command{Kind: recordEvent}))
}
func (r *Runtime) EventTime(e Event) (timing.VTimeInPicoSec, error) {
	t, ok := r.state.Events[e]
	if !ok {
		return 0, fmt.Errorf("runtime: event %d has not triggered", e)
	}
	return t, nil
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
func (r *Runtime) take(q CommandQueue) (Command, bool) {
	queue := r.queue(q)
	if len(queue.Pending) == 0 {
		return Command{}, false
	}
	c := queue.Pending[0]
	queue.Pending[0] = Command{}
	queue.Pending = queue.Pending[1:]
	return c, true
}
func (r *Runtime) complete(q CommandQueue, c Command, now timing.VTimeInPicoSec) {
	queue := r.queue(q)
	if queue.Unfinished == 0 {
		panic("runtime: duplicate completion")
	}
	queue.Unfinished--
	if c.Kind == copyD2H {
		r.state.Results[c.ID] = c.Data
	}
	if c.Kind == recordEvent {
		r.state.Events[Event(c.ID)] = now
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
