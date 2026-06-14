package simulation

import (
	"github.com/sarchlab/akita/v5/datarecording"

	"github.com/sarchlab/akita/v5/monitoring2"
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type Simulation struct {
	id           string
	outputPath   string
	engine       timing.Engine
	dataRecorder datarecording.DataRecorder
	visTracer    *tracing.DBTracer
	metaRecorder *metaRecorder
	monitor      *monitoring2.Monitor

	components    []Component
	compNameIndex map[string]int
	ports         []Port
	portNameIndex map[string]int
	connections   []Connection
	connNameIndex map[string]int
	resources     []Resource

	// entities is the single, flat inventory of every registered runtime object
	// (components, ports, connections, resources, the engine, and the ID
	// generator), each held as the Entity it satisfies. entityByName resolves a
	// globally unique name to its index. Together they make the inventory a
	// complete state snapshot: serializing every entity's state captures
	// everything needed to recover the simulation. The engine is additionally
	// held in the engine field for direct typed access (see GetEngine).
	entities     []Entity
	entityByName map[string]int
}

// ID returns the ID of the simulation. An ID is a UUID that is generated when
// the simulation is created.
func (s *Simulation) ID() string {
	return s.id
}

// GetEngine returns the engine used in the simulation.
func (s *Simulation) GetEngine() timing.Engine {
	return s.engine
}

// GetDataRecorder returns the data recorder used in the simulation.
func (s *Simulation) GetDataRecorder() datarecording.DataRecorder {
	return s.dataRecorder
}

// GetMonitor returns the live monitor attached to the simulation, if enabled.
func (s *Simulation) GetMonitor() *monitoring2.Monitor {
	return s.monitor
}

// GetVisTracer returns the tracer used in the simulation.
func (s *Simulation) GetVisTracer() *tracing.DBTracer {
	return s.visTracer
}

// Components returns a copy of the registered components, in registration
// order.
func (s *Simulation) Components() []Component {
	return append([]Component(nil), s.components...)
}

// registerEntity records a live entity in the single, flat inventory. It is the
// only cross-kind uniqueness check — names must be globally unique across all
// kinds, which is what makes the inventory a well-defined state snapshot. The
// typed Register methods pass the concrete object, which satisfies Entity, so
// the inventory holds the live entity itself.
func (s *Simulation) registerEntity(e Entity) {
	name := e.Name()
	if name == "" {
		panic("entity name cannot be empty")
	}

	if s.entityByName == nil {
		s.entityByName = make(map[string]int)
	}

	if _, found := s.entityByName[name]; found {
		panic("entity " + name + " already registered")
	}

	s.entities = append(s.entities, e)
	s.entityByName[name] = len(s.entities) - 1
}

// RegisterComponent registers a component with the simulation. It accepts any
// named object so that component builders can register through the
// modeling.Registrar interface without importing this package.
func (s *Simulation) RegisterComponent(c naming.Named) {
	compName := c.Name()
	s.registerEntity(c)

	s.components = append(s.components, c)
	s.compNameIndex[compName] = len(s.components) - 1

	if hookable, ok := c.(tracing.NamedHookable); ok {
		tracing.CollectTrace(hookable, s.visTracer)
	}

	if s.monitor != nil {
		s.monitor.RegisterComponent(c)
	}
}

// RegisterPort registers a port with the simulation so it can be resolved by
// name and monitored. Port builders call this through the modeling.Registrar
// interface, mirroring RegisterComponent — a component is registered when it is
// built, and each of its ports is registered when the port is built.
func (s *Simulation) RegisterPort(p naming.Named) {
	port, ok := p.(Port)
	if !ok {
		panic("simulation: RegisterPort requires a messaging.Port, got " +
			p.Name())
	}

	if _, dup := s.portNameIndex[port.Name()]; dup {
		panic("simulation: port " + port.Name() + " already registered " +
			"(duplicate port name) — port names must be globally unique; " +
			"use hierarchical names like \"ComponentName.PortName\"")
	}

	s.registerPort(port)

	if s.monitor != nil {
		s.monitor.RegisterPort(port)
	}
}

// registerPort registers a port with the simulation.
func (s *Simulation) registerPort(p Port) {
	portName := p.Name()
	s.registerEntity(p)

	s.ports = append(s.ports, p)
	s.portNameIndex[portName] = len(s.ports) - 1
}

// RegisterConnection registers a connection with the simulation runtime
// inventory. Setup code still owns topology construction and PlugIn calls, but
// registered connections are tracked as runtime entities in the global state
// manager.
func (s *Simulation) RegisterConnection(c naming.Named) {
	connName := c.Name()
	s.registerEntity(c)

	s.connections = append(s.connections, c)
	s.connNameIndex[connName] = len(s.connections) - 1
}

// Connections returns a copy of the registered connections, in registration
// order.
func (s *Simulation) Connections() []Connection {
	return append([]Connection(nil), s.connections...)
}

// RegisterResource registers non-timing program state that can be referenced by
// multiple components and reached by name through the global state manager. The
// simulation owns the resource; components hold references to it. Setup
// constructs and registers each shared resource once under a canonical name.
func (s *Simulation) RegisterResource(r naming.Named) {
	if r == nil {
		panic("resource cannot be nil")
	}

	s.registerEntity(r)
	s.resources = append(s.resources, r)
}

// Resources returns a copy of the registered shared-state resources, in
// registration order.
func (s *Simulation) Resources() []Resource {
	return append([]Resource(nil), s.resources...)
}

// GetComponentByName returns the component with the given name.
func (s *Simulation) GetComponentByName(name string) Component {
	idx, found := s.compNameIndex[name]
	if !found {
		panic("component " + name + " not registered")
	}

	return s.components[idx]
}

// GetPortByName returns the port with the given name. Ports are registered
// either when their component is registered (legacy components that create
// ports in Build) or when the port is built (via a port builder that calls
// RegisterPort).
func (s *Simulation) GetPortByName(name string) Port {
	idx, found := s.portNameIndex[name]
	if !found {
		panic("port " + name + " not registered")
	}

	return s.ports[idx]
}

// Terminate terminates the simulation.
func (s *Simulation) Terminate() {
	if s.monitor != nil {
		s.monitor.StopServer()
	}

	if s.visTracer != nil {
		s.visTracer.Terminate()
	}

	if s.metaRecorder != nil {
		s.metaRecorder.End()
	}

	s.dataRecorder.Close()
}
