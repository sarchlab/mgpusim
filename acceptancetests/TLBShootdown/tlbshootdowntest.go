package main

import (
	"flag"
	"log"
	"reflect"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/benchmarks/heteromark/fir"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/samples/runner"
	"gitlab.com/akita/gcn3/timing"
	"gitlab.com/akita/mem/vm"
	"gitlab.com/akita/mem/vm/mmu"
	"gitlab.com/akita/mem/vm/tlb"
	"gitlab.com/akita/util/ca"
)

var numData = flag.Int("length", 4096, "The number of samples to filter.")

type ShootdownControlComponent struct {
	*akita.TickingComponent
	cus                 []akita.Port
	toCU                akita.Port
	vmModules           []akita.Port
	toVMModule          akita.Port
	gpuDriver           *driver.Driver
	cuRspsReceived      int
	vmRspsReceived      int
	curShootdownRequest *gcn3.ShootDownCommand
	cusDrained          bool
	vmsInvalidated      bool
}

type ShootdownReqEvent struct {
	*akita.EventBase
	req *gcn3.ShootDownCommand
}

func newShootdownReqEvent(
	time akita.VTimeInSec,
	handler akita.Handler,
	req *gcn3.ShootDownCommand,
) *ShootdownReqEvent {
	return &ShootdownReqEvent{akita.NewEventBase(time, handler), req}
}

func NewShootdownControlComponent(
	name string,
	engine akita.Engine,
) *ShootdownControlComponent {
	ctrl := new(ShootdownControlComponent)
	ctrl.TickingComponent = akita.NewTickingComponent(name, engine, 1*akita.GHz, ctrl)
	ctrl.toCU = akita.NewLimitNumReqPort(ctrl, 1)
	ctrl.toVMModule = akita.NewLimitNumReqPort(ctrl, 1)
	return ctrl

}

func (ctrl *ShootdownControlComponent) Handle(e akita.Event) error {
	switch evt := e.(type) {
	case akita.TickEvent:
		ctrl.handleTickEvent(evt)
	case *ShootdownReqEvent:
		ctrl.handleShootdownReqEvent(evt)
	default:
		log.Panicf("Component cannot handle handle event of type %s", reflect.TypeOf(e))
	}
	return nil
}

func (ctrlComp *ShootdownControlComponent) handleTickEvent(tick akita.TickEvent) {
	now := tick.Time()
	ctrlComp.NeedTick = false

	ctrlComp.parseFromCU(now)
	ctrlComp.parseFromVMUnit(now)

	if ctrlComp.NeedTick {
		ctrlComp.TickLater(now)
	}

}

func (ctrlComp *ShootdownControlComponent) handleShootdownReqEvent(evt *ShootdownReqEvent) {
	req := evt.req

	ctrlComp.curShootdownRequest = req
	now := evt.Time()

	for i := 0; i < len(ctrlComp.gpuDriver.GPUs[0].CUs); i++ {
		drainReq := gcn3.NewCUPipelineDrainReq(akita.VTimeInSec(now), ctrlComp.toCU, ctrlComp.cus[i])
		err := ctrlComp.toCU.Send(drainReq)
		if err != nil {
			log.Panicf("failed to send drain request to CU")
		}

	}

}

func (ctrlComp *ShootdownControlComponent) handleCURsp(now akita.VTimeInSec, req akita.Req) {

	ctrlComp.cuRspsReceived++

	if ctrlComp.cuRspsReceived < len(ctrlComp.gpuDriver.GPUs[0].CUs) {
		return
	}

	ctrlComp.cuRspsReceived = 0

	ctrlComp.cusDrained = ctrlComp.checkCU()

	if ctrlComp.cusDrained == false {
		log.Panicf("CUS not drained. Something went wrong")
	}

	shootdownCmd := ctrlComp.curShootdownRequest

	for i := 0; i < len(ctrlComp.vmModules); i++ {
		req := vm.NewPTEInvalidationReq(now, ctrlComp.toVMModule, ctrlComp.vmModules[i], shootdownCmd.PID, shootdownCmd.VAddr)
		err := ctrlComp.toVMModule.Send(req)
		if err != nil {
			log.Panicf("failed to send shootdown req to TLB's or MMU")
		}
	}

}

func (ctrlComp *ShootdownControlComponent) handleVMUnitRsp(now akita.VTimeInSec, req akita.Req) {

	ctrlComp.vmRspsReceived++

	if ctrlComp.vmRspsReceived < len(ctrlComp.vmModules) {
		return
	}

	shootdownCmd := ctrlComp.curShootdownRequest
	vAddr := shootdownCmd.VAddr
	PID := shootdownCmd.PID
	ctrlComp.vmRspsReceived = 0

	ctrlComp.vmsInvalidated = ctrlComp.checkVMUnits(vAddr, PID)

	//log.Printf("VM's invalidated")

	if ctrlComp.vmsInvalidated == false {
		log.Panicf("VM units did not invalidate. Something went wrong")
	}

	//Get the page table and set valid to true
	table := ctrlComp.gpuDriver.MMU.GetOrCreatePageTable(PID)

	for i := 0; i < len(vAddr); i++ {
		page := table.FindPage(vAddr[i])
		page.Valid = true

	}

	//Restart all CUs
	for i := 0; i < len(ctrlComp.gpuDriver.GPUs[0].CUs); i++ {
		cuRestartReq := gcn3.NewCUPipelineRestartReq(now, ctrlComp.toCU, ctrlComp.cus[i])
		sendErr := ctrlComp.toCU.Send(cuRestartReq)
		if sendErr != nil {
			log.Panicf("Failed to send restart request")
		}
	}

}

func (ctrlComp *ShootdownControlComponent) checkCU() bool {
	for i := 0; i < len(ctrlComp.gpuDriver.GPUs[0].CUs); i++ {
		cu := ctrlComp.gpuDriver.GPUs[0].CUs[i].(*timing.ComputeUnit)

		drainCompleted := true

		drainCompleted = drainCompleted && cu.BranchUnit.IsIdle()

		drainCompleted = drainCompleted && cu.ScalarUnit.IsIdle()

		drainCompleted = drainCompleted && cu.ScalarDecoder.IsIdle()

		for _, simdUnit := range cu.SIMDUnit {
			drainCompleted = drainCompleted && simdUnit.IsIdle()
		}

		drainCompleted = drainCompleted && cu.VectorDecoder.IsIdle()

		drainCompleted = drainCompleted && cu.LDSUnit.IsIdle()

		drainCompleted = drainCompleted && cu.LDSDecoder.IsIdle()

		drainCompleted = drainCompleted && cu.VectorMemUnit.IsIdle()

		drainCompleted = drainCompleted && cu.VectorMemDecoder.IsIdle()

		drainCompleted = drainCompleted && (len(cu.InFlightInstFetch) == 0) && (len(cu.InFlightScalarMemAccess) == 0) && (len(cu.InFlightVectorMemAccess) == 0)

		if drainCompleted == false {
			log.Panicf("CU not drained successfully")

		}

	}

	return true
}

func (ctrlComp *ShootdownControlComponent) checkPageValidity(
	tlb *tlb.TLB,
	vAddr []uint64,
	pID ca.PID,
) bool {
	invalidPage := true

	for i := 0; i < len(vAddr); i++ {
		sets := tlb.Sets
		setID := vAddr[i] / uint64(4096) % uint64(len(sets))
		set := sets[setID]
		for _, page := range set.Blocks {
			if page.PID == pID &&
				vAddr[i] >= page.VAddr &&
				vAddr[i] < page.VAddr+page.PageSize {
				invalidPage = invalidPage && !page.Valid
			}
		}

	}
	return invalidPage
}

func (ctrlComp *ShootdownControlComponent) checkVMUnits(
	vAddr []uint64,
	pID ca.PID,
) bool {

	vmInvalidated := true

	for i := 0; i < len(ctrlComp.gpuDriver.GPUs[0].L1ITLBs); i++ {
		tlb := ctrlComp.gpuDriver.GPUs[0].L1ITLBs[i]
		vmInvalidated = vmInvalidated && ctrlComp.checkPageValidity(tlb, vAddr, pID)
	}

	for i := 0; i < len(ctrlComp.gpuDriver.GPUs[0].L1VTLBs); i++ {
		tlb := ctrlComp.gpuDriver.GPUs[0].L1VTLBs[i]
		vmInvalidated = vmInvalidated && ctrlComp.checkPageValidity(tlb, vAddr, pID)
	}

	for i := 0; i < len(ctrlComp.gpuDriver.GPUs[0].L1STLBs); i++ {
		tlb := ctrlComp.gpuDriver.GPUs[0].L1STLBs[i]
		vmInvalidated = vmInvalidated && ctrlComp.checkPageValidity(tlb, vAddr, pID)
	}

	for i := 0; i < len(ctrlComp.gpuDriver.GPUs[0].L2TLBs); i++ {
		tlb := ctrlComp.gpuDriver.GPUs[0].L2TLBs[i]
		vmInvalidated = vmInvalidated && ctrlComp.checkPageValidity(tlb, vAddr, pID)
	}

	table := ctrlComp.gpuDriver.MMU.GetOrCreatePageTable(pID)

	for i := 0; i < len(vAddr); i++ {
		page := table.FindPage(vAddr[i])
		vmInvalidated = vmInvalidated && !page.Valid

	}

	return vmInvalidated

}

func (ctrlComp *ShootdownControlComponent) parseFromCU(now akita.VTimeInSec) {
	cuReq := ctrlComp.toCU.Retrieve(now)

	if cuReq == nil {
		return
	}

	switch req := cuReq.(type) {
	case *gcn3.CUPipelineDrainRsp:
		ctrlComp.handleCURsp(now, req)
		return
	default:
		log.Panicf("Received an unsupported request type %s from CU \n", reflect.TypeOf(cuReq))
	}

	ctrlComp.NeedTick = true

}

func (ctrlComp *ShootdownControlComponent) parseFromVMUnit(now akita.VTimeInSec) {
	vmReq := ctrlComp.toVMModule.Retrieve(now)

	if vmReq == nil {
		return
	}

	switch req := vmReq.(type) {
	case *vm.InvalidationCompleteRsp:
		ctrlComp.handleVMUnitRsp(now, req)
		return
	default:
		log.Panicf("Received an unsupported request type %s from CU \n", reflect.TypeOf(vmReq))
	}

	ctrlComp.NeedTick = true

}

func main() {
	flag.Parse()
	r := runner.Runner{}
	r.Init()

	benchmark := fir.NewBenchmark(r.GPUDriver)
	benchmark.Length = *numData
	r.AddBenchmark(benchmark)

	ctrlComponent := NewShootdownControlComponent("ctrl", r.Engine)

	ctrlComponent.gpuDriver = r.GPUDriver

	for i := 0; i < len(r.GPUDriver.GPUs[0].CUs); i++ {
		ctrlComponent.cus = append(ctrlComponent.cus, akita.NewLimitNumReqPort(ctrlComponent, 1))
		r.GPUDriver.GPUs[0].InternalConnection.PlugIn(ctrlComponent.cus[i])
	}

	r.GPUDriver.GPUs[0].InternalConnection.PlugIn(ctrlComponent.toCU)

	for i := 0; i < len(r.GPUDriver.GPUs[0].CUs); i++ {
		r.GPUDriver.GPUs[0].CUs[i].(*timing.ComputeUnit).CP = ctrlComponent.toCU
	}

	for i := 0; i < len(r.GPUDriver.GPUs[0].CUs); i++ {
		ctrlComponent.cus[i] = r.GPUDriver.GPUs[0].CUs[i].(*timing.ComputeUnit).ToCP
	}

	l1VTLBCount := len(r.GPUDriver.GPUs[0].L1VTLBs)
	l1STLBCount := len(r.GPUDriver.GPUs[0].L1STLBs)
	l1ITLBCount := len(r.GPUDriver.GPUs[0].L1ITLBs)
	l2TLBCount := len(r.GPUDriver.GPUs[0].L2TLBs)
	mmuCount := 1

	totalVMUnits := l1VTLBCount + l1STLBCount + l1ITLBCount + mmuCount + l2TLBCount

	for i := 0; i < totalVMUnits; i++ {
		ctrlComponent.vmModules = append(ctrlComponent.vmModules, akita.NewLimitNumReqPort(ctrlComponent, 1))
		r.GPUDriver.GPUs[0].InternalConnection.PlugIn(ctrlComponent.vmModules[i])
	}

	r.GPUDriver.GPUs[0].InternalConnection.PlugIn(ctrlComponent.toVMModule)

	currentVMCount := 0

	for i := 0; i < l1VTLBCount; i++ {
		ctrlComponent.vmModules[currentVMCount] =
			r.GPUDriver.GPUs[0].L1VTLBs[i].ControlPort
		currentVMCount++
	}

	for i := 0; i < l1STLBCount; i++ {
		ctrlComponent.vmModules[currentVMCount] =
			r.GPUDriver.GPUs[0].L1STLBs[i].ControlPort
		currentVMCount++
	}

	for i := 0; i < l1ITLBCount; i++ {
		ctrlComponent.vmModules[currentVMCount] =
			r.GPUDriver.GPUs[0].L1ITLBs[i].ControlPort
		currentVMCount++
	}

	for i := 0; i < l2TLBCount; i++ {
		ctrlComponent.vmModules[currentVMCount] =
			r.GPUDriver.GPUs[0].L2TLBs[i].ControlPort
		currentVMCount++
	}

	ctrlComponent.vmModules[currentVMCount] =
		r.GPUDriver.MMU.(*mmu.MMUImpl).MigrationPort

	r.KernelTimeCounter = driver.NewKernelTimeCounter()
	r.GPUDriver.AcceptHook(r.KernelTimeCounter)

	vAddr := []uint64{4294967296, 4295004160}
	shootDownTime := 0.000001637000000

	shootdownReq := gcn3.NewShootdownCommand(akita.VTimeInSec(shootDownTime), nil, nil, vAddr, 1)
	shootdownEvent := newShootdownReqEvent(akita.VTimeInSec(shootDownTime), ctrlComponent, shootdownReq)
	r.Engine.Schedule(shootdownEvent)

	r.Run()

}
