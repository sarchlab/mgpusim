package directconnection

// State holds mutable runtime state for the DirectConnection.
type State struct {
	NextPortID int `json:"next_port_id"`
}
