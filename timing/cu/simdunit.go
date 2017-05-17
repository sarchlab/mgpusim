package cu

import (
	"gitlab.com/yaotsu/core"
)

// SIMDUnit is a unit that can execute vector instructions
//
//    <=> ToScheduler
//    <=> ToVReg
//    <=> ToSReg
type SIMDUnit struct {
	*core.ComponentBase

	VRegFile *RegCtrl
	SRegFile *RegCtrl
}

// NewSIMDUnit returns a newly created SIMDUnit
func NewSIMDUnit(name string) *SIMDUnit {
	u := new(SIMDUnit)
	u.ComponentBase = core.NewComponentBase(name)
	return u
}

// Recv defines how an SIMDUnit processes incomming request
func (u *SIMDUnit) Recv(req core.Req) *core.Error {
	return nil
}

// Handle defines how an SIMDUnit handles events
func (u *SIMDUnit) Handle(evt core.Event) error {
	return nil
}
