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
