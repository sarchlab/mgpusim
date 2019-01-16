package trace

type Task struct {
	ID           string
	ParentTaskID string
	Type         string
	What         string
	Where        string
	Start, End   float64
	Detail       interface{}
}
