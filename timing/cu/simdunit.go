package cu

import (
	"log"
	"reflect"

	"gitlab.com/yaotsu/core"
)

// SIMDUnit is a unit that can execute vector instructions
//
// FromDecoder <=>
//
// ToScheduler <=>
//
// ToVReg <=>
//
// ToSReg <=>
type SIMDUnit struct {
	*core.ComponentBase

	VRegFile *RegCtrl
	SRegFile *RegCtrl
}

// NewSIMDUnit returns a newly created SIMDUnit
func NewSIMDUnit(name string) *SIMDUnit {
	u := new(SIMDUnit)
	u.ComponentBase = core.NewComponentBase(name)
	u.AddPort("FromDecoder")
	u.AddPort("ToScheduler")
	u.AddPort("ToVReg")
	u.AddPort("ToSReg")
	return u
}

// Recv defines how an SIMDUnit processes incomming request
func (u *SIMDUnit) Recv(req core.Req) *core.Error {
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

// Handle defines how an SIMDUnit handles events
func (u *SIMDUnit) Handle(evt core.Event) error {
	return nil
}
