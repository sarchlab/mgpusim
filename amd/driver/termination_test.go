package driver

import (
	"sync"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sarchlab/akita/v5/hooking"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

// Hold the engine after its command has left the queue but before the final
// event hooks have finished, just as can happen during simulation teardown.
type blockingAfterEventHook struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (h *blockingAfterEventHook) Func(ctx hooking.HookCtx) {
	if ctx.Pos != timing.HookPosAfterEvent {
		return
	}

	h.once.Do(func() {
		close(h.entered)
		<-h.release
	})
}

type simulationEndTracer struct {
	tracing.NopTracer
	simulationID uint64
	ended        chan struct{}
}

func (t *simulationEndTracer) StartTask(task tracing.TaskStart) {
	if task.Kind == "Simulation" {
		t.simulationID = task.ID
	}
}

func (t *simulationEndTracer) EndTask(task tracing.TaskEnd) {
	if task.ID == t.simulationID {
		close(t.ended)
	}
}

var _ = ginkgo.Describe("Driver termination", func() {
	ginkgo.It("should wait for trailing engine hooks before ending the simulation", func() {
		engine := timing.NewSerialEngine()
		hook := &blockingAfterEventHook{
			entered: make(chan struct{}),
			release: make(chan struct{}),
		}
		engine.AcceptHook(hook)

		driver := MakeBuilder().
			WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
			WithResources(Resources{PageTable: vm.NewPageTable(12)}).
			Build("Driver")
		port := messaging.NewPort(driver.Comp, 16, 16, "Driver.GPU")
		(&noopConn{}).PlugIn(port)
		driver.AssignPort(GPUPortName, port)

		tracer := &simulationEndTracer{ended: make(chan struct{})}
		tracing.CollectTrace(driver, tracer)
		driver.Run()

		queue := driver.CreateCommandQueue(driver.Init())
		enqueueNoopCommand(driver, queue)
		driver.DrainCommandQueue(queue)
		Eventually(hook.entered).Should(BeClosed())

		terminated := make(chan struct{})
		go func() {
			driver.Terminate()
			close(terminated)
		}()
		ginkgo.DeferCleanup(func() {
			close(hook.release)
			Eventually(terminated).Should(BeClosed())
			driver.WaitForEngineIdle()
			Expect(tracer.ended).To(BeClosed())
		})

		// An empty command queue is insufficient: the engine still owns hooks
		// whose tracers and recorders must remain alive until it has drained.
		Expect(queue.NumCommand()).To(BeZero())
		Consistently(terminated, 100*time.Millisecond).ShouldNot(BeClosed())
		Expect(tracer.ended).NotTo(BeClosed())
	})
})
