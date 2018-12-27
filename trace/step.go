package trace

type Step struct {
	TaskID string
	When   float64
	Where  string
	What   string
	Detail interface{}
}
