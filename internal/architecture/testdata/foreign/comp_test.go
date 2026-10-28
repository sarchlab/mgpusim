package foreign

import (
	"testing"

	checks "github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	other "github.com/sarchlab/mgpusim/v5/internal/architecture/testdata/valid"
)

func TestAnotherComponent(t *testing.T) {
	checks.CheckTicking(t, (*other.Comp)(nil), other.Spec{})
}
