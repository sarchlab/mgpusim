package cu

import (
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"testing"
)

func TestComponentMatchesSource(t *testing.T) {
	modelingtest.CheckTicking(t, (*Comp)(nil), DefaultSpec())
}
