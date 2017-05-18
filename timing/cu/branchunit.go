package cu

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
)

// BranchUnit is the execution unit that is responsible for executing the
// local data share instuctions
//
// ToScheduler <=> The port that connects the BranchUnit and the Scheduler
type BranchUnit struct {
	*core.ComponentBase
}

// NewBranchUnit creates and retuns a new BranchUnit
func NewBranchUnit(name string) *BranchUnit {
	u := new(BranchUnit)
	u.ComponentBase = core.NewComponentBase(name)
	u.AddPort("ToScheduler")
	return u
}

// Recv defines the how the BranchUnit process incomming requests
func (u *BranchUnit) Recv(req core.Req) *core.Error {
	switch req := req.(type) {
	case *IssueInstReq:
		replyReq := NewInstCompletionReq(u, req.Scheduler, req.RecvTime(),
			req.Wf)
		u.GetConnection("ToScheduler").Send(replyReq)
	default:
		log.Panicf("cannot process request of type %s", reflect.TypeOf(req))
	}
	return nil
}

// Handle defines how the BranchUnit handles events
func (u *BranchUnit) Handle(evt core.Event) error {
	return nil
}
