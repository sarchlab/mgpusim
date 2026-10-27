package smsp

import (
	"testing"
)

func TestRejectInvalidPeers(t *testing.T) {
	for _, spec := range []Spec{{}, {ToSM: "sm"}, {ToVectorMem: "memory"}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("invalid Spec accepted: %+v", spec)
				}
			}()
			c := &Comp{}
			c.Spec = spec
			c.State.Initialize(c)
		}()
	}
}
