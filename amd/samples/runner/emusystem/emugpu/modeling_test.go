package emugpu

import (
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"testing"
)

func TestComponentModel(t *testing.T) {
	modelingtest.CheckTicking(t, (*Comp)(nil), DefaultSpec())
}
