package timing

import (
	"gitlab.com/yaotsu/core"
)

// A Dispatcher is a component that can dispatch workgroups and wavefronts
// to compute units.
type Dispatcher struct {
	*core.BasicComponent
}
