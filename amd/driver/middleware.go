package driver

// A Middleware is a pluggable element of the driver that can take care of the
// handling of certain types of commands and parts of the driver-GPU
// communication.
type Middleware interface {
	ProcessCommand(
		cmd Command,
		queue *CommandQueue,
	) (processed bool)
	Tick() (madeProgress bool)
}
