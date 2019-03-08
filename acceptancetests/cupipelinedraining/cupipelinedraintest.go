package main

import (
	"flag"
	"log"
	"reflect"

	"net/http"
	_ "net/http/pprof"

	"gitlab.com/akita/akita"
	"gitlab.com/akita/gcn3"
	"gitlab.com/akita/gcn3/benchmarks/heteromark/fir"
	"gitlab.com/akita/gcn3/driver"
	"gitlab.com/akita/gcn3/samples/runner"
	"gitlab.com/akita/gcn3/timing"
)

var numData = flag.Int("length", 4096, "The number of samples to filter.")

type ControlComponent struct {
	*akita.TickingComponent
	cus            []akita.Port
	toCU           akita.Port
	gpuDriver      *driver.Driver
	cuRspsReceived int
}

type cuPipelineDrainReqEvent struct {
	*akita.EventBase
	req *gcn3.CUPipelineDrainReq
}

func NewControlComponent(
	name string,
	engine akita.Engine,
) *ControlComponent {
	ctrlComponent := new(ControlComponent)
	ctrlComponent.TickingComponent = akita.NewTickingComponent(name, engine, 1*akita.GHz, ctrlComponent)
	ctrlComponent.toCU = akita.NewLimitNumReqPort(ctrlComponent, 1)
	return ctrlComponent

}

func (ctrl *ControlComponent) Handle(e akita.Event) error {
	switch evt := e.(type) {
	case akita.TickEvent:
		ctrl.handleTickEvent(evt)
	case *cuPipelineDrainReqEvent:
		ctrl.handleCUPipelineDrain(evt)
	default:
		log.Panicf("Component cannot handle handle event of type %s", reflect.TypeOf(e))
	}
	return nil
}

func (ctrlComp *ControlComponent) handleTickEvent(tick akita.TickEvent) {
	now := tick.Time()
	ctrlComp.NeedTick = false

	ctrlComp.parseFromCU(now)

	if ctrlComp.NeedTick {
		ctrlComp.TickLater(now)
	}

}

func (ctrlComp *ControlComponent) handleCUPipelineDrain(evt *cuPipelineDrainReqEvent) {
	req := evt.req
	sendErr := ctrlComp.toCU.Send(req)
	if sendErr != nil {
		log.Panicf("Unable to send drain request to CU")
	}

}

func (ctrlComp *ControlComponent) checkCU(now akita.VTimeInSec, req akita.Req) {

	ctrlComp.cuRspsReceived++

	if ctrlComp.cuRspsReceived < len(ctrlComp.gpuDriver.GPUs[0].CUs) {
		return
	}

	ctrlComp.cuRspsReceived = 0

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

		cuRestartReq := gcn3.NewCUPipelineRestartReq(now, ctrlComp.toCU, ctrlComp.cus[i])
		sendErr := ctrlComp.toCU.Send(cuRestartReq)

		if sendErr != nil {
			log.Panicf("Failed to send restart request")
		}

	}

}

func (ctrlComp *ControlComponent) parseFromCU(now akita.VTimeInSec) {
	cuReq := ctrlComp.toCU.Retrieve(now)

	if cuReq == nil {
		return
	}

	switch req := cuReq.(type) {
	case *gcn3.CUPipelineDrainRsp:
		ctrlComp.checkCU(now, req)
		return
	default:
		log.Panicf("Received an unsupported request type %s from CU \n", reflect.TypeOf(cuReq))
	}

}

func newCUPipelineDrainReqEvent(
	time akita.VTimeInSec,
	handler akita.Handler,
	req *gcn3.CUPipelineDrainReq,
) *cuPipelineDrainReqEvent {
	return &cuPipelineDrainReqEvent{akita.NewEventBase(time, handler), req}
}

func main() {
	flag.Parse()

	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	r := runner.Runner{}
	r.Init()
	// r.Engine, r.GPUDriver = platform.BuildNR9NanoPlatform(1)

	benchmark := fir.NewBenchmark(r.GPUDriver)
	benchmark.Length = *numData
	r.AddBenchmark(benchmark)

	ctrlComponent := NewControlComponent("ctrl", r.Engine)

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

	r.KernelTimeCounter = driver.NewKernelTimeCounter()
	r.GPUDriver.AcceptHook(r.KernelTimeCounter)

	drainTime := 0.000001637000000

	for i := 0; i < len(r.GPUDriver.GPUs[0].CUs); i++ {
		drainReq := gcn3.NewCUPipelineDrainReq(akita.VTimeInSec(drainTime), ctrlComponent.toCU, ctrlComponent.cus[i])
		drainEvent := newCUPipelineDrainReqEvent(akita.VTimeInSec(drainTime), ctrlComponent, drainReq)
		r.Engine.Schedule(drainEvent)
	}

	r.Run()
}
