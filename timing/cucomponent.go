package timing

// A CUComponent is an element installed in the compute unit
type CUComponent interface {
	CanAcceptWave() bool
	AcceptWave(wave *Wavefront) error
	Run()
}
