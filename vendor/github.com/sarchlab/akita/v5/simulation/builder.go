package simulation

import (
	"github.com/rs/xid"
	"github.com/sarchlab/akita/v5/datarecording"

	"github.com/sarchlab/akita/v5/monitoring2"
	"github.com/sarchlab/akita/v5/timing"
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
	b.createIDGenerator(s)
	b.createMetaRecorder(s)
	b.createVisTracer(s)
	b.createServer(s)

	return s
}

func (b Builder) createSimulation() *Simulation {
	return &Simulation{
		id:            xid.New().String(),
		compNameIndex: make(map[string]int),
		portNameIndex: make(map[string]int),
		connNameIndex: make(map[string]int),
		entityByName:  make(map[string]int),
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
	if b.parallelEngine {
		engine := timing.NewParallelEngine()
		s.engine = engine
		s.registerEntity(engine)
	} else {
		engine := timing.NewSerialEngine()
		s.engine = engine
		s.registerEntity(engine)
	}
}

// createIDGenerator registers the process-wide ID generator as an entity so its
// counter is captured in the state snapshot.
func (b Builder) createIDGenerator(s *Simulation) {
	s.registerEntity(timing.GetIDGenerator().(Entity))
}

func (b Builder) createMetaRecorder(s *Simulation) {
	s.metaRecorder = newMetaRecorder(s.dataRecorder, s.engine)
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

	monitor := monitoring2.NewMonitor()
	if b.monitorPort != 0 {
		monitor.WithPortNumber(b.monitorPort)
	}

	monitor.RegisterEngine(s.engine)
	monitor.RegisterVisTracer(s.visTracer)
	monitor.SetTraceDBPath(s.outputPath + ".sqlite3")
	monitor.StartServer()

	s.monitor = monitor
}
