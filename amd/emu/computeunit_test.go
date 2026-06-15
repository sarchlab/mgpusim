package emu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/directconnection"
	"github.com/sarchlab/akita/v5/timing"
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

// zeroStorageAccessor returns zero bytes for every read.
type zeroStorageAccessor struct{}

func (a zeroStorageAccessor) Read(pid vm.PID, vAddr, byteSize uint64) []byte {
	return make([]byte, byteSize)
}

func (a zeroStorageAccessor) Write(pid vm.PID, vAddr uint64, data []byte) {}

func exampleWorkGroup() *kernels.WorkGroup {
	wg := kernels.NewWorkGroup()
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

	wf := kernels.NewWavefront()
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
	engine := timing.NewSerialEngine()
	registrar := modeling.NewStandaloneRegistrar(engine)

	cu := MakeBuilder().
		WithRegistrar(registrar).
		WithResources(Resources{
			Decoder:         endpgmDecoder{},
			ALU:             &mockBenchALU{},
			StorageAccessor: zeroStorageAccessor{},
		}).
		Build("CU")
	cuPort := messaging.NewPort(cu, 1, 1, "CU.ToDispatcher")
	cu.AssignPort(DispatchPortName, cuPort)

	dispatcherPort := messaging.NewPort(nil, 4, 4, "Dispatcher.ToCU")

	conn := directconnection.MakeBuilder().
		WithRegistrar(registrar).
		Build("Conn")
	conn.PlugIn(cuPort)
	conn.PlugIn(dispatcherPort)

	req := protocol.MapWGReq{
		MsgMeta: messaging.MsgMeta{
			ID:  timing.GetIDGenerator().Generate(),
			Src: dispatcherPort.AsRemote(),
			Dst: cuPort.AsRemote(),
		},
		WorkGroup: exampleWorkGroup(),
		PID:       1,
	}
	dispatcherPort.Send(req)

	if err := engine.Run(); err != nil {
		t.Fatalf("engine run failed: %v", err)
	}

	msg := dispatcherPort.RetrieveIncoming()
	if msg == nil {
		t.Fatal("expected a WGCompletionMsg, got none")
	}

	completion, ok := msg.(protocol.WGCompletionMsg)
	if !ok {
		t.Fatalf("expected WGCompletionMsg, got %T", msg)
	}

	if len(completion.RspToIDs) != 1 || completion.RspToIDs[0] != req.ID {
		t.Fatalf("expected RspToIDs [%d], got %v",
			req.ID, completion.RspToIDs)
	}
}
