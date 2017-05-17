package cu

import "gitlab.com/yaotsu/core"

// ScalarUnit is the execution unit that is responsible for executing the
// local data share instuctions
//
// ToScheduler <=>
//
// FromDecoder <=>
type ScalarUnit struct {
	*core.ComponentBase
}

// NewScalarUnit creates and retuns a new ScalarUnit
func NewScalarUnit(name string) *ScalarUnit {
	u := new(ScalarUnit)
	u.ComponentBase = core.NewComponentBase(name)
	u.AddPort("ToScheduler")
	u.AddPort("FromDecoder")
	return u
}

// Recv defines the how the ScalarUnit process incomming requests
func (u *ScalarUnit) Recv(req core.Req) *core.Error {
	return nil
}

// Handle defines how the ScalarUnit handles events
func (u *ScalarUnit) Handle(evt core.Event) error {
	return nil
}
