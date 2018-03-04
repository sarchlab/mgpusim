package timing

// MemAccessAction enumerates all the memory interaction from a compute unit
type MemAccessAction int

// The possible memory access actions
const (
	MemAccessInstFetch MemAccessAction = iota
)

// MemAccessInfo is the information that is attached to a memory access
// request. When the request returns from the memory system, the compute
// unit need the information to perform correcponding action.
type MemAccessInfo struct {
	Action MemAccessAction
	Wf     *Wavefront
}
