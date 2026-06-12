package simulation

import (
	"github.com/rs/xid"
	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/monitoring"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

// Builder can be used to build a simulation.
type Builder struct {
	parallelEngine    bool
	monitorOn         bool
	monitorPort       int
	outputFileName    string
	visTracingOnStart bool
}

// MakeBuilder creates a new builder.
func MakeBuilder() Builder {
	return Builder{
		parallelEngine: false,
		monitorOn:      true,
	}
}

// WithParallelEngine sets the simulation to use a parallel engine.
func (b Builder) WithParallelEngine() Builder {
	b.parallelEngine = true
	return b
}

// WithoutMonitoring sets the simulation to not use monitoring.
func (b Builder) WithoutMonitoring() Builder {
	b.monitorOn = false
	return b
}

// WithOutputFileName sets the custom output file name for the data recorder.
func (b Builder) WithOutputFileName(filename string) Builder {
	b.outputFileName = filename
	return b
}

// WithMonitorPort sets the port number for the monitoring server.
func (b Builder) WithMonitorPort(port int) Builder {
	b.monitorPort = port
	return b
}

// WithVisTracingOnStart enables visual tracing from the start of the simulation.
func (b Builder) WithVisTracingOnStart() Builder {
	b.visTracingOnStart = true
	return b
}

func (b Builder) parametersMustBeValid() {
	if !b.monitorOn && b.monitorPort != 0 {
		panic("monitor port cannot be set when monitoring is disabled")
	}
}

// Build builds the simulation.
func (b Builder) Build() *Simulation {
	b.parametersMustBeValid()

	s := b.createSimulation()

	b.createDataRecorder(s)
	b.createEngine(s)
	b.createVisTracer(s)
	b.createServer(s)

	return s
}

func (b Builder) createSimulation() *Simulation {
	return &Simulation{
		id:            xid.New().String(),
		compNameIndex: make(map[string]int),
		portNameIndex: make(map[string]int),
	}
}

func (b Builder) createDataRecorder(s *Simulation) {
	outputPath := b.outputFileName
	if outputPath == "" {
		outputPath = "akita_sim_" + s.id
	}
	s.outputPath = outputPath
	s.dataRecorder = datarecording.NewDataRecorder(outputPath)
}

func (b Builder) createEngine(s *Simulation) {
	s.engine = sim.NewSerialEngine()
	if b.parallelEngine {
		s.engine = sim.NewParallelEngine()
	}
}

func (b Builder) createVisTracer(s *Simulation) {
	s.visTracer = tracing.NewDBTracer(s.engine, s.dataRecorder)

	if b.visTracingOnStart {
		s.visTracer.StartTracing()
	}
}

func (b Builder) createServer(s *Simulation) {
	if !b.monitorOn {
		return
	}

	monitor := monitoring.NewMonitor()

	if b.monitorPort > 0 {
		monitor.WithPortNumber(b.monitorPort)
	}

	monitor.RegisterEngine(s.engine)
	monitor.RegisterVisTracer(s.visTracer)
	monitor.SetTraceDBPath(s.outputPath + ".sqlite3")
	monitor.StartServer()

	s.monitor = monitor
}
