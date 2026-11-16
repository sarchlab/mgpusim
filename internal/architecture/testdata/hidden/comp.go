package hidden

import model "github.com/sarchlab/akita/v5/sim/modeling/ticking"

type Spec struct{}
type State struct{}
type Resources struct{}
type Ports struct{}
type Middlewares struct{}
type Hidden = model.Component[Spec, State, Resources, Ports, Middlewares]
