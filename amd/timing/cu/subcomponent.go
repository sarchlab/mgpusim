package cu

// A SubComponent is an element installed in the compute unit
type SubComponent interface {
	CanAcceptWave() bool
	AcceptWave(wave *Wavefront)
	Run() bool
	IsIdle() bool
}
