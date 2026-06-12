package directconnection

import "github.com/sarchlab/akita/v5/sim"

// Spec holds immutable configuration for the DirectConnection.
type Spec struct {
	Freq sim.Freq `json:"freq"`
}
