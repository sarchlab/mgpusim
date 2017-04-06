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
	*core.BasicComponent

	VRegFile *RegCtrl
	SRegFile *RegCtrl
}
