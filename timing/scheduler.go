package timing

// A Scheduler is the controlling unit of a compute unit. It decides which
// wavefront to fetch and to issue.
type Scheduler interface {
	// DoFetch will fetch instructions for wavefronts.
	DoFetch()

	// DoIssue will issue wavefronts to the decoding units.
	DoIssue()
}

// A DefaultScheduler simulates the scheduler that is in a AMD Fiji GPU.
type DefaultScheduler struct {
	cu *ComputeUnit
}

// NewDefaultScheduler creates and returns a new DefaultScheduler
func NewDefaultScheduler(cu *ComputeUnit) *DefaultScheduler {
	s := new(DefaultScheduler)

	s.cu = cu

	return s
}
