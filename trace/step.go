package trace

type Task struct {
	Start, End float64
	Steps      []*Step
	Detail     interface{}
}

func (t *Task) AddStep(step *Step) {
	if len(t.Steps) == 0 || t.Start > step.When {
		t.Start = step.When
	}

	if len(t.Steps) == 0 || t.End < step.When {
		t.End = step.When
	}

	t.Steps = append(t.Steps, step)
}

type Step struct {
	When   float64
	Where  string
	What   string
	Detail interface{}
}
