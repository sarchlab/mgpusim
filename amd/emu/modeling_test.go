package emu

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
)

func TestModelingContract(t *testing.T) {
	modelingtest.CheckWakeup(t, (*Comp)(nil), DefaultSpec())
}
