package trace

type Kernel struct { // trace execs interface
	rawText    string
	fileName   string
	filePath   string
	traceGroup *traceGroupReader
}

func (te *Kernel) Type() string {
	return "kernel"
}

func (te *Kernel) File() string {
	return te.fileName
}
