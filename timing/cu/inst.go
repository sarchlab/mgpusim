package cu

import "gitlab.com/yaotsu/gcn3/insts"
import "gitlab.com/yaotsu/core"

// Inst in the timing package is a wrapper of the insts.Inst. This Inst has
// an ID and is hookable
type Inst struct {
	*insts.Inst
	*core.HookableBase

	ID int
}
