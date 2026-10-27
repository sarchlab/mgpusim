package driver

import (
	"testing"
)

func TestRejectInvalidPeers(t *testing.T) {
	for _, spec := range []Spec{
		{Devices: []DevicePeer{{Name: "gpu"}}},
		{Devices: []DevicePeer{{Port: "port"}}},
		{Devices: []DevicePeer{{Name: "gpu", Port: "port"}, {Name: "gpu", Port: "port2"}}},
		{Devices: []DevicePeer{{Name: "gpu", Port: "port"}, {Name: "gpu2", Port: "port"}}},
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
