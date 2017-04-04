package timing

import "gitlab.com/yaotsu/core"

// A ScalarUnit can run gcn3 scalar instructions
//
//    <=> ToScheduler Receives IssueReq from the scheduler and send the
//        same request back to the scheduler when complete
type ScalarUnit struct {
	*core.BasicComponent
}
