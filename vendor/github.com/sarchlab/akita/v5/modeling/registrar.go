package modeling

import (
	"github.com/sarchlab/akita/v5/naming"
	"github.com/sarchlab/akita/v5/timing"
)

// Registrar is the single build-time context a builder needs: it sources the
// engine (GetEngine) and registers the built entity. A *simulation.Simulation
// satisfies it. Builders accept it through a WithRegistrar method, replacing
// separate engine and registration steps. Each builder calls the registration
// method matching the kind it builds.
type Registrar interface {
	GetEngine() timing.Engine
	RegisterComponent(c naming.Named)
	RegisterConnection(c naming.Named)
	RegisterResource(c naming.Named)
	RegisterPort(p naming.Named)
}

// standaloneRegistrar adapts a bare engine into a Registrar whose registration
// methods are no-ops. It is the engine-only path for isolated unit tests that
// build a component without a full simulation.
type standaloneRegistrar struct {
	engine timing.Engine
}

// NewStandaloneRegistrar returns a Registrar backed only by the given engine,
// with no-op registration. Use it in isolated tests:
//
//	comp := pkg.MakeBuilder().
//		WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
//		WithSpec(spec).
//		Build("Comp")
func NewStandaloneRegistrar(engine timing.Engine) Registrar {
	return &standaloneRegistrar{engine: engine}
}

func (r *standaloneRegistrar) GetEngine() timing.Engine          { return r.engine }
func (r *standaloneRegistrar) RegisterComponent(_ naming.Named)  {}
func (r *standaloneRegistrar) RegisterConnection(_ naming.Named) {}
func (r *standaloneRegistrar) RegisterResource(_ naming.Named)   {}
func (r *standaloneRegistrar) RegisterPort(_ naming.Named)       {}
