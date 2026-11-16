package valid

import model "github.com/sarchlab/akita/v5/sim/modeling/ticking"

type Spec struct{}
type State struct{}
type Resources struct{}
type Ports struct{}
type Middlewares struct{}
type Comp = model.Component[Spec, State, Resources, Ports, Middlewares]
