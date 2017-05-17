package cu

import "gitlab.com/yaotsu/core"

// LDSUnit is the execution unit that is responsible for executing the
// local data share instuctions
//
// ToScheduler <=> The port that is used to send InstCompletionReq
//
// FromDecoder <=> The port that is used to receive IssueInstReq from decoder
type LDSUnit struct {
	*core.ComponentBase
}

// NewLDSUnit creates and retuns a new LDSUnit
func NewLDSUnit(name string) *LDSUnit {
	u := new(LDSUnit)
	u.ComponentBase = core.NewComponentBase(name)
	u.AddPort("ToScheduler")
	u.AddPort("FromDecoder")
	return u
}

// Recv defines the how the LDSUnit process incomming requests
func (u *LDSUnit) Recv(req core.Req) *core.Error {
	return nil
}

// Handle defines how the LDSUnit handles events
func (u *LDSUnit) Handle(evt core.Event) error {
	return nil
}
