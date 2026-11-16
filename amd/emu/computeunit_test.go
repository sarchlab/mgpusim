package emu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/mgpusim/v5/amd/insts"
	"github.com/sarchlab/mgpusim/v5/amd/kernels"
	"github.com/sarchlab/mgpusim/v5/amd/protocol"
)

// endpgmDecoder always decodes an S_ENDPGM instruction.
type endpgmDecoder struct{}

func (d endpgmDecoder) Decode(buf []byte) (*insts.Inst, error) {
	inst := insts.NewInst()
	inst.FormatType = insts.SOPP
	inst.Opcode = 1 // S_ENDPGM
	inst.ByteSize = 4
	return inst, nil
}

func exampleWorkGroup(s *sim.Simulation) *kernels.WorkGroup {
	wg := kernels.NewWorkGroup(s.NewID())
	wg.SizeX, wg.SizeY, wg.SizeZ = 64, 1, 1
	wg.CodeObject = &insts.KernelCodeObject{
		KernelCodeObjectMeta: &insts.KernelCodeObjectMeta{},
	}
	wg.Packet = &kernels.HsaKernelDispatchPacket{
		WorkgroupSizeX: 64,
		WorkgroupSizeY: 1,
		WorkgroupSizeZ: 1,
		GridSizeX:      64,
		GridSizeY:      1,
		GridSizeZ:      1,
		KernelObject:   0x1000,
	}

	wf := kernels.NewWavefront(s.NewID())
	wf.WG = wg
	wf.CodeObject = wg.CodeObject
	wf.Packet = wg.Packet
	wf.InitExecMask = ^uint64(0)
	wg.Wavefronts = append(wg.Wavefronts, wf)

	return wg
}

// TestComputeUnitRunsWorkGroup dispatches a MapWGReq to the emulation CU and
// expects a WGCompletionMsg that acknowledges the request.
func TestComputeUnitRunsWorkGroup(t *testing.T) {
	registrar := sim.MakeBuilder().Build()
	t.Cleanup(func() {
		if err := registrar.Terminate(); err != nil {
			t.Error(err)
		}
	})

	decoder, storage := &recordingEndDecoder{}, &changingCodeStorage{}
	cu := MakeBuilder().
		WithSimulation(registrar).
		WithResources(Resources{
			Decoder:         decoder,
			ALU:             &mockBenchALU{},
			StorageAccessor: storage,
		}).
		Build("CU")
	cuPort := twowaybuffered.MakeBuilder().WithSimulation(registrar).Build()
	cu.BindPort(DispatchPortName, cuPort)
	endpoint := modeling.MakeBuilder[*testEndpoint](
		testEndpointSpec{Freq: timing.GHz},
	).WithSimulation(registrar).
		Build("Dispatcher")
	dispatcherPort := twowaybuffered.MakeBuilder().
		WithSimulation(registrar).
		WithSpec(twowaybuffered.Spec{IncomingCapacity: 4, OutgoingCapacity: 4}).
		Build()
	endpoint.BindPort("ToCU", dispatcherPort)
	conn := direct.NewConnection("Conn", registrar, timing.GHz)
	conn.BindPort(cuPort)
	conn.BindPort(dispatcherPort)
	if err := registrar.Initialize(); err != nil {
		t.Fatal(err)
	}

	req := messaging.Msg{ID: registrar.NewID(),
		Src: dispatcherPort.AsRemote(),
		Dst: cuPort.AsRemote(),
		Payload: protocol.MapWGReq{WorkGroup: kernels.WorkGroupSnapshot(exampleWorkGroup(registrar)),
			PID: 1}}

	dispatcherPort.Send(req)

	if err := registrar.Engine().Run(); err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	msg, ok := dispatcherPort.RetrieveIncoming()
	if !ok {
		t.Fatal("expected a WGCompletionMsg, got none")
	}

	completion := msg.Payload.(protocol.WGCompletionMsg)

	if len(completion.RspToIDs) != 1 || completion.RspToIDs[0] != req.ID {
		t.Fatalf("expected RspToIDs [%d], got %v",
			req.ID, completion.RspToIDs)
	}
	checkCodeReload(t, registrar, cu, dispatcherPort, decoder, storage)
}

type testEndpointSpec struct{ Freq timing.Freq }
type testEndpointPorts struct {
	ToCU messaging.Port `akita:"role=github.com/sarchlab/mgpusim/v5/amd/protocol.cp"`
}
type testEndpoint = ticking.Component[testEndpointSpec, struct{}, struct{}, testEndpointPorts, struct{}]

type recordingEndDecoder struct{ bytes []byte }

func (d *recordingEndDecoder) Decode(data []byte) (*insts.Inst, error) {
	d.bytes = append(d.bytes, data[0])
	return endpgmDecoder{}.Decode(data)
}

type changingCodeStorage struct{ generation byte }

func (s *changingCodeStorage) Read(pid vm.PID, address, size uint64) []byte {
	data := make([]byte, size)
	data[0] = byte(pid) + s.generation
	return data
}
func (*changingCodeStorage) Write(vm.PID, uint64, []byte) {}

func checkCodeReload(t *testing.T, registrar *sim.Simulation, cu *Comp, dispatcherPort messaging.Port,
	decoder *recordingEndDecoder, storage *changingCodeStorage) {
	t.Helper()
	// The same PC belongs to a different process, then to reloaded code in the
	// first process. Each atomic work-group must fetch current canonical bytes.
	for _, pid := range []vm.PID{2, 1} {
		if pid == 1 {
			storage.generation = 10
		}
		next := messaging.Msg{ID: registrar.NewID(), Src: dispatcherPort.AsRemote(),
			Dst:     cu.Ports.ToDispatcher.AsRemote(),
			Payload: protocol.MapWGReq{WorkGroup: kernels.WorkGroupSnapshot(exampleWorkGroup(registrar)), PID: pid}}
		cu.Middlewares.Processor.runWG(cu, next, registrar.Engine().CurrentTime())
		if err := registrar.Engine().Run(); err != nil {
			t.Fatal(err)
		}
		if _, ok := dispatcherPort.RetrieveIncoming(); !ok {
			t.Fatal("missing repeated completion")
		}
		if len(cu.Middlewares.Processor.instCache) != 0 {
			t.Fatal("decoded scratch retained after work-group")
		}
	}
	if len(decoder.bytes) != 3 || decoder.bytes[0] != 1 || decoder.bytes[1] != 2 || decoder.bytes[2] != 11 {
		t.Fatalf("stale instruction bytes across PID/code reload: %v", decoder.bytes)
	}
}
