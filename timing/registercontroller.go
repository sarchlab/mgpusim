package timing

import (
	"gitlab.com/yaotsu/core"
)

// A RegisterController is a Yaotsu component that is responsible for the
// timing of reading and writing registers
type RegisterController struct {
	*core.BasicComponent
}
