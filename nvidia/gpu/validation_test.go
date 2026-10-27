package gpu

import (
	"testing"
)

func TestRejectInvalidPeers(t *testing.T) {
	for _, spec := range []Spec{
		{ToDriver: "driver", SMs: []SMPeer{{Name: "sm", Port: "port"}, {Name: "sm", Port: "port2"}}},
		{ToDriver: "driver", SMs: []SMPeer{{Name: "sm", Port: "port"}, {Name: "sm2", Port: "port"}}},
		{ToDriver: "driver", SMs: []SMPeer{{Name: "sm"}}},
		{},
	} {
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
