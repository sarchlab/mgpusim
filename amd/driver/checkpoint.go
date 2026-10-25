package driver

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
	"sort"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/mgpusim/v5/amd/driver/internal"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
)

type driverCheckpoint struct{ driver *Driver }

func (r *driverCheckpoint) Name() string { return r.driver.Name() + ".Runtime" }

type savedBuffer struct {
	Address      Ptr
	Size         uint64
	Freed, Dirty bool
}
type savedQueue struct {
	Running  bool
	GPU      int
	PID      vm.PID
	Commands []savedCommand
}
type savedContext struct {
	PID          vm.PID
	GPU          int
	PreviousPage uint64
	Dirty        bool
	Buffers      []savedBuffer
	Queues       []savedQueue
}
type savedCommand struct {
	Kind       string
	ID         uint64
	Reqs       []messaging.Msg
	Src, Dst   Ptr
	Data       []byte
	ByteSize   int
	CodeObject insts.CodeObjectValue
	Grid       [3]uint32
	Group      [3]uint16
	Packet     kernels.PacketValue
	DPacket    Ptr
	Packets    []kernels.PacketValue
	DPackets   []Ptr
}
type savedCodeAddress struct {
	Code    insts.CodeObjectValue
	Address Ptr
}
type driverRuntimeState struct {
	Allocator      internal.AllocatorState
	Contexts       []savedContext
	Requests       []messaging.Msg
	CodeAddresses  []savedCodeAddress
	SimulationTask uint64
	CopyCyclesLeft int
	CopyAwaiting   []messaging.Msg
	UnboundResults map[uint64][]byte
}

type driverConfig struct {
	Spec    Spec
	GPUs    []messaging.RemotePort
	Devices []internal.DeviceProperties
}

func (d *Driver) checkpointConfig() driverConfig {
	config := driverConfig{Spec: d.Spec, GPUs: d.GPUs}
	for _, device := range d.devices {
		if device.Type != internal.DeviceTypeUnifiedGPU {
			config.Devices = append(config.Devices, device.Properties)
		}
	}
	return config
}

func snapshotCommand(c Command) (savedCommand, error) {
	s := savedCommand{ID: c.GetID(), Reqs: c.GetReqs()}
	switch c := c.(type) {
	case *MemCopyH2DCommand:
		s.Kind = "h2d"
		s.Dst = c.Dst
		var b bytes.Buffer
		if err := binary.Write(&b, binary.LittleEndian, c.Src); err != nil {
			return s, err
		}
		s.Data = b.Bytes()
	case *MemCopyD2HCommand:
		s.Kind = "d2h"
		s.Src = c.Src
		s.Data = c.RawData
		s.ByteSize = copyByteSize(c)
		if s.ByteSize < 0 {
			return s, fmt.Errorf("driver: unsupported result buffer for command %d", c.ID)
		}
	case *LaunchKernelCommand:
		s.Kind = "kernel"
		s.CodeObject = insts.CodeObjectSnapshot(c.CodeObject)
		s.Grid = c.GridSize
		s.Group = c.WGSize
		s.Packet = kernels.PacketSnapshot(c.Packet)
		s.DPacket = c.DPacket
	case *LaunchUnifiedMultiGPUKernelCommand:
		s.Kind = "unified-kernel"
		s.CodeObject = insts.CodeObjectSnapshot(c.CodeObject)
		s.Grid = c.GridSize
		s.Group = c.WGSize
		s.DPackets = c.DPacketArray
		for _, p := range c.PacketArray {
			s.Packets = append(s.Packets, kernels.PacketSnapshot(p))
		}
	case *FlushCommand:
		s.Kind = "flush"
	case *NoopCommand:
		s.Kind = "noop"
	default:
		return s, fmt.Errorf("driver: unsupported command type %T", c)
	}
	return s, nil
}

func (s savedCommand) restore() (Command, error) {
	switch s.Kind {
	case "h2d":
		return &MemCopyH2DCommand{ID: s.ID, Dst: s.Dst, Src: s.Data, Reqs: s.Reqs}, nil
	case "d2h":
		return &MemCopyD2HCommand{ID: s.ID, Src: s.Src, RawData: s.Data, ByteSize: s.ByteSize, Reqs: s.Reqs}, nil
	case "kernel":
		return &LaunchKernelCommand{
			ID:         s.ID,
			CodeObject: s.CodeObject.Restore(),
			GridSize:   s.Grid,
			WGSize:     s.Group,
			Packet:     s.Packet.Restore(),
			DPacket:    s.DPacket,
			Reqs:       s.Reqs,
		}, nil
	case "unified-kernel":
		c := &LaunchUnifiedMultiGPUKernelCommand{
			ID:           s.ID,
			CodeObject:   s.CodeObject.Restore(),
			GridSize:     s.Grid,
			WGSize:       s.Group,
			DPacketArray: s.DPackets,
			Reqs:         s.Reqs,
		}
		for _, p := range s.Packets {
			c.PacketArray = append(c.PacketArray, p.Restore())
		}
		return c, nil
	case "flush":
		return &FlushCommand{ID: s.ID, Reqs: s.Reqs}, nil
	case "noop":
		return &NoopCommand{ID: s.ID}, nil
	default:
		return nil, fmt.Errorf("driver: unknown saved command %q", s.Kind)
	}
}

func (r *driverCheckpoint) SaveCheckpoint(w io.Writer) error {
	d := r.driver
	d.contextMutex.Lock()
	defer d.contextMutex.Unlock()
	s := driverRuntimeState{
		Allocator:      internal.SnapshotAllocator(d.memAllocator, d.devices),
		Requests:       d.requestsToSend,
		SimulationTask: d.simulationID,
		UnboundResults: d.unboundResults,
	}
	for _, ctx := range d.contexts {
		c := savedContext{PID: ctx.pid, GPU: ctx.currentGPUID, PreviousPage: ctx.prevPageVAddr, Dirty: ctx.l2Dirty}
		for _, b := range ctx.buffers {
			c.Buffers = append(c.Buffers, savedBuffer{b.vAddr, b.size, b.freed, b.l2Dirty})
		}
		for _, q := range ctx.queues {
			v := savedQueue{Running: q.IsRunning, GPU: q.GPUID, PID: q.PID}
			for _, cmd := range q.commands {
				saved, err := snapshotCommand(cmd)
				if err != nil {
					return err
				}
				v.Commands = append(v.Commands, saved)
			}
			c.Queues = append(c.Queues, v)
		}
		s.Contexts = append(s.Contexts, c)
	}
	for code, addr := range d.codeObjGPUAddrs {
		s.CodeAddresses = append(s.CodeAddresses, savedCodeAddress{insts.CodeObjectSnapshot(code), addr})
	}
	sort.Slice(s.CodeAddresses, func(i, j int) bool { return s.CodeAddresses[i].Address < s.CodeAddresses[j].Address })
	for _, mw := range d.middlewares {
		if m, ok := mw.(*defaultMemoryCopyMiddleware); ok {
			s.CopyCyclesLeft = m.cyclesLeft
			s.CopyAwaiting = m.awaitingReqs
		}
	}
	return modeling.WriteCheckpoint(w, d.checkpointConfig(), s, nil)
}

func (r *driverCheckpoint) LoadCheckpoint(reader io.Reader) error {
	d := r.driver
	var s driverRuntimeState
	if err := modeling.ReadCheckpoint(reader, d.checkpointConfig(), &s, nil); err != nil {
		return err
	}
	allocator, devices, err := internal.RestoreAllocator(s.Allocator, d.Resources.PageTable)
	if err != nil {
		return err
	}
	contexts := make([]*Context, 0, len(s.Contexts))
	for _, v := range s.Contexts {
		c := &Context{pid: v.PID, currentGPUID: v.GPU, prevPageVAddr: v.PreviousPage, l2Dirty: v.Dirty}
		for _, b := range v.Buffers {
			c.buffers = append(c.buffers, &buffer{vAddr: b.Address, size: b.Size, freed: b.Freed, l2Dirty: b.Dirty})
		}
		for _, sq := range v.Queues {
			q := &CommandQueue{IsRunning: sq.Running, GPUID: sq.GPU, PID: sq.PID, Context: c}
			for _, sc := range sq.Commands {
				cmd, err := sc.restore()
				if err != nil {
					return err
				}
				q.commands = append(q.commands, cmd)
			}
			c.queues = append(c.queues, q)
		}
		contexts = append(contexts, c)
	}
	d.memAllocator = allocator
	d.devices = devices
	d.contexts = contexts
	dist := newDistributorImpl(allocator)
	dist.pageSizeAsPowerOf2 = d.Log2PageSize
	d.distributor = dist
	d.requestsToSend = s.Requests
	d.simulationID = s.SimulationTask
	d.unboundResults = s.UnboundResults
	if d.unboundResults == nil {
		d.unboundResults = make(map[uint64][]byte)
	}
	d.codeObjGPUAddrs = make(map[*insts.KernelCodeObject]Ptr)
	for _, v := range s.CodeAddresses {
		d.codeObjGPUAddrs[v.Code.Restore()] = v.Address
	}
	for _, mw := range d.middlewares {
		if m, ok := mw.(*defaultMemoryCopyMiddleware); ok {
			m.cyclesLeft = s.CopyCyclesLeft
			m.awaitingReqs = s.CopyAwaiting
		}
	}
	return nil
}

// ResultBuffer describes a submitted device-to-host copy whose host buffer has
// not been rebound after restoration. Complete results remain available until
// BindResultBuffer delivers them.
type ResultBuffer struct {
	CommandID uint64
	ByteSize  int
	Complete  bool
}

// ResultBuffers lists unbound copies in command-ID order. Call while the engine
// is idle and no host goroutine is changing command queues.
func (d *Driver) ResultBuffers() []ResultBuffer {
	var out []ResultBuffer
	for _, ctx := range d.contexts {
		for _, q := range ctx.queues {
			for _, c := range q.commands {
				if c, ok := c.(*MemCopyD2HCommand); ok && c.Dst == nil {
					out = append(out, ResultBuffer{c.ID, copyByteSize(c), false})
				}
			}
		}
	}
	for id, data := range d.unboundResults {
		out = append(out, ResultBuffer{id, len(data), true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CommandID < out[j].CommandID })
	return out
}

// BindResultBuffer attaches a host destination to a restored copy. Its binary
// size must match the submitted copy. A copy that has already completed is
// delivered immediately. Call while the engine and host submission are idle.
func (d *Driver) BindResultBuffer(id uint64, dst any) error {
	if data, ok := d.unboundResults[id]; ok {
		if err := validateResultBuffer(dst, len(data)); err != nil {
			return err
		}
		if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, dst); err != nil {
			return err
		}
		delete(d.unboundResults, id)
		return nil
	}
	for _, ctx := range d.contexts {
		for _, q := range ctx.queues {
			for _, c := range q.commands {
				if c, ok := c.(*MemCopyD2HCommand); ok && c.ID == id {
					if err := validateResultBuffer(dst, copyByteSize(c)); err != nil {
						return err
					}
					c.Dst = dst
					return nil
				}
			}
		}
	}
	return fmt.Errorf("driver: unknown result command %d", id)
}

func validateResultBuffer(dst any, size int) error {
	if dst == nil {
		return fmt.Errorf("driver: nil result destination")
	}
	if binary.Size(dst) != size {
		return fmt.Errorf("driver: result buffer size mismatch")
	}
	// Validate writability without changing the caller's pending output.
	value := reflect.ValueOf(dst)
	var probe any
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return fmt.Errorf("driver: nil result destination")
		}
		probe = reflect.New(value.Type().Elem()).Interface()
		if value.Elem().Kind() == reflect.Slice {
			reflect.ValueOf(probe).
				Elem().
				Set(reflect.MakeSlice(value.Elem().Type(), value.Elem().Len(), value.Elem().Len()))
		}
	case reflect.Slice:
		probe = reflect.MakeSlice(value.Type(), value.Len(), value.Len()).Interface()
	default:
		return fmt.Errorf("driver: result destination must be a slice or pointer")
	}
	return binary.Read(bytes.NewReader(make([]byte, size)), binary.LittleEndian, probe)
}

func copyByteSize(c *MemCopyD2HCommand) int {
	if c.Dst != nil {
		return binary.Size(c.Dst)
	}
	return c.ByteSize
}
func (d *Driver) deliverResult(c *MemCopyD2HCommand) {
	if c.Dst == nil {
		d.unboundResults[c.ID] = c.RawData
		return
	}
	if err := binary.Read(bytes.NewReader(c.RawData), binary.LittleEndian, c.Dst); err != nil {
		panic(err)
	}
}
