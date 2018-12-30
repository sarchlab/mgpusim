package trace

type Task struct {
	ID         string
	Start, End float64
	Steps      []*Step
	Detail     interface{}
}

func (t *Task) AddStep(step *Step) {
	if len(t.Steps) == 0 || t.Start > step.Start {
		t.Start = step.Start
	}

	if len(t.Steps) == 0 || t.End < step.End {
		t.End = step.End
	}

	t.Steps = append(t.Steps, step)
}

type Step struct {
	Start, End float64
	Where      string
	What       string
	Detail     interface{}
}
