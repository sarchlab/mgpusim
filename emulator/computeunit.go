package emulator

import (
	"fmt"
	"log"
	"reflect"

	"gitlab.com/yaotsu/core/conn"
	"gitlab.com/yaotsu/core/event"
	"gitlab.com/yaotsu/gcn3/disasm"
	"gitlab.com/yaotsu/mem"
)

// A MapWorkGroupReq is a request sent from a dispatcher to a compute unit
// to request the compute unit to execute a workgroup.
type MapWorkGroupReq struct {
	*conn.BasicRequest

	WG      *WorkGroup
	IsReply bool
	Succeed bool
}

// NewMapWorkGroupReq returns a new MapWorkGroupReq
func NewMapWorkGroupReq() *MapWorkGroupReq {
	r := new(MapWorkGroupReq)
	r.BasicRequest = conn.NewBasicRequest()

	return r
}

// MapWorkGroupReqFactory is the factory that creates MapWorkGroupReq
type MapWorkGroupReqFactory interface {
	Create() *MapWorkGroupReq
}

type mapWorkGroupReqFactoryImpl struct {
}

func (f *mapWorkGroupReqFactoryImpl) Create() *MapWorkGroupReq {
	return NewMapWorkGroupReq()
}

// NewMapWorkGroupReqFactory returns the default factory for the
// MapWorkGroupReq
func NewMapWorkGroupReqFactory() MapWorkGroupReqFactory {
	return &mapWorkGroupReqFactoryImpl{}
}

// A ComputeUnit is the unit that can execute workgroups.
//
// A ComputeUnit is a Yaotsu component
//   ToDispatcher <=> Receive the dispatch request and respond with the
//                    Completion signal
type ComputeUnit struct {
	*conn.BasicComponent

	WorkGroup *WorkGroup

	VgprStorage *mem.Storage // Should be 1 MB
	SgprStorage *mem.Storage // Should be 102 * 16 * 4 Bytes

	VgprPerWorkItem      int
	SgprPerWavefront     int
	WorkItemPerWavefront int
	MaxNumOfWIs          int
}

// NewComputeUnit creates a ComputeUnit
func NewComputeUnit(name string) *ComputeUnit {
	cu := new(ComputeUnit)
	cu.BasicComponent = conn.NewBasicComponent(name)

	cu.VgprStorage = mem.NewStorage(1 * mem.MB)
	cu.SgprStorage = mem.NewStorage(102 * 64)

	cu.VgprPerWorkItem = 256
	cu.SgprPerWavefront = 102
	cu.WorkItemPerWavefront = 64
	cu.MaxNumOfWIs = 1024

	cu.AddPort("ToDispatcher")
	return cu
}

// vgprToAddress converts a VGPR to the address in the vector register file
func (cu *ComputeUnit) vgprToAddress(reg *disasm.Reg, wiFlattenedId int) int {
	wiInWgId := wiFlattenedId % cu.MaxNumOfWIs
	return wiInWgId*cu.VgprPerWorkItem + reg.RegIndex()

	log.Panic("Registers other othan SGPR and VGPR are not supported yet")
	return 0
}

// sgprToAddress converts a SGPR to the address in the scalar register file
func (cu *ComputeUnit) sgprToAddress(reg *disasm.Reg, wiFlattenedId int) int {
	wfId := wiFlattenedId / cu.WorkItemPerWavefront
	return wfId*cu.SgprPerWavefront + reg.RegIndex()
}

// WriteRegister updates the value in the register file
func (cu *ComputeUnit) WriteRegister(reg *disasm.Reg,
	wiFlattenedId int, data []byte) {
	if reg.IsVReg() {
		addr := cu.vgprToAddress(reg, wiFlattenedId)
		err := cu.VgprStorage.Write(uint64(addr), data)
		if err != nil {
			log.Panic(err)
		}
	} else if reg.IsSReg() {
		addr := cu.sgprToAddress(reg, wiFlattenedId)
		err := cu.SgprStorage.Write(uint64(addr), data)
		if err != nil {
			log.Panic(err)
		}
	} else {
		log.Panic("Only VGPRs and SGPRs are supported")
	}
}

// ReadRegsiter returns the register value in the register file
func (cu *ComputeUnit) ReadRegister(reg *disasm.Reg,
	wiFlattenedId int, byteSize int) []byte {
	if reg.IsVReg() {
		addr := cu.vgprToAddress(reg, wiFlattenedId)
		data, err := cu.VgprStorage.Read(uint64(addr), uint64(byteSize))
		if err != nil {
			log.Panic(err)
		}
		return data
	}

	if reg.IsSReg() {
		addr := cu.sgprToAddress(reg, wiFlattenedId)
		data, err := cu.SgprStorage.Read(uint64(addr), uint64(byteSize))
		if err != nil {
			log.Panic(err)
		}
		return data
	}

	log.Panic("Only VGPRs and SGPRs are supported")
	return nil
}

func (cu *ComputeUnit) initializeRegisters() {
	count := 0
	if cu.WorkGroup.Grid.CodeObject.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Panic("Initializing register PrivateSegmentWaveByteOffset is not supported")
		count += 4
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprDispatchPtr() {
		// reg := disasm.SReg(count)

		log.Println("Initializing register DispatchPtr is not supported")
		count += 2
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprQueuePtr() {
		log.Println("Initializing register QueuePtr is not supported")
		count += 2
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprKernelArgSegmentPtr() {
		log.Println("Initializing register KernargSegmentPtr is not supported")
		count += 2
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprDispatchId() {
		log.Println("Initializing register DispatchId is not supported")
		count += 2
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprFlatScratchInit() {
		log.Println("Initializing register FlatScratchInit is not supported")
		count += 2
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprPrivateSegementSize() {
		log.Println("Initializing register PrivateSegementSize is not supported")
		count += 1
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprGridWorkGroupCountX() {
		log.Println("Initializing register GridWorkGroupCountX is not supported")
		count += 1
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprGridWorkGroupCountY() {
		log.Println("Initializing register GridWorkGroupCountY is not supported")
		count += 1
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprGridWorkGroupCountZ() {
		log.Println("Initializing register GridWorkGroupCountZ is not supported")
		count += 1
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprWorkGroupIdX() {
		log.Println("Initializing register GridWorkGroupIdX is not supported")
		count += 1
	}
	if cu.WorkGroup.Grid.CodeObject.EnableSgprWorkGroupIdY() {
		log.Println("Initializing register GridWorkGroupIdY is not supported")
		count += 1
	}
	if cu.WorkGroup.Grid.CodeObject.EnableSgprWorkGroupIdZ() {
		log.Println("Initializing register GridWorkGroupIdZ is not supported")
		count += 1
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprWorkGroupInfo() {
		log.Println("Initializing register GridWorkGroupInfo is not supported")
		count += 1
	}

	if cu.WorkGroup.Grid.CodeObject.EnableSgprPrivateSegmentWaveByteOffset() {
		log.Println("Initializing register PrivateSegmentWaveByteOffset is not supported")
		count += 1
	}

	log.Println("Initializing register WorkItemIdX is not supported")

	if cu.WorkGroup.Grid.CodeObject.EnableVgprWorkItemId() > 0 {
		log.Println("Initializing register WorkItemIdY is not supported")
	}
	if cu.WorkGroup.Grid.CodeObject.EnableVgprWorkItemId() > 1 {
		log.Println("Initializing register WorkItemIdZ is not supported")
	}

	log.Printf("Done initialize registers\n")
}

func (cu *ComputeUnit) startExecution() {
}

func (cu *ComputeUnit) handleMapWorkGroupReq(req *MapWorkGroupReq) *conn.Error {
	if cu.WorkGroup != nil {
		req.SwapSrcAndDst()
		req.IsReply = true
		req.Succeed = false
		cu.GetConnection("ToDispatcher").Send(req)
		return nil
	}

	cu.WorkGroup = req.WG
	cu.initializeRegisters()
	cu.startExecution()

	return nil
}

// Receive processes the incomming requests
func (cu *ComputeUnit) Receive(req conn.Request) *conn.Error {
	switch req := req.(type) {
	case *MapWorkGroupReq:
		return cu.handleMapWorkGroupReq(req)
	default:
		return conn.NewError(
			fmt.Sprintf("cannot process request %s", reflect.TypeOf(req)), false, 0)
	}
}

// Handle processes the events that is scheduled for the CommandProcessor
func (cu *ComputeUnit) Handle(e event.Event) error {
	return nil
}
