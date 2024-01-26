package trace

type memCopy struct { // trace execs interface
	rawText   string
	h2d       bool
	startAddr uint64
	length    uint64
}

func (te *memCopy) Type() string {
	return "memcopy"
}

func (te *memCopy) File() string {
	return ""
}
