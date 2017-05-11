package cu

// IssueDirection tells the category of an instruction
type IssueDirection int

// A list of all possible issue directions
const (
	IssueDirVALU IssueDirection = iota
	IssueDirScalar
	IssueDirVMem
	IssueDirBranch
	IssueDirLDS
	IssueDirGDS
	IssueDirInternal
)

// An IssueArbitrator decides which wavefront can issue instruction
type IssueArbitrator struct {
}
