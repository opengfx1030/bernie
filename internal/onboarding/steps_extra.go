package onboarding

// The optional steps of the path: the benchmark, the TunableOp profile and
// the power cap. They never hold the path: they come after it and the
// reader does them when they want to.

// benchStep is an extra: it never holds the path.
func benchStep(f *Facts) Step {
	s := Step{ID: StepBench, Title: "Measure the speed (optional)", Command: "bernie bench", Action: "Measure the speed"}
	s.State = StateTodo
	s.Summary = "A benchmark shows the tokens per second of this host."
	if run := f.live(); run != nil {
		s.Detail = run.URL
	}
	return s
}

// tuningStep is an extra: it never holds the path.
func tuningStep(f *Facts) Step {
	s := Step{ID: StepTuning, Title: "TunableOp profile (optional)", Command: "bernie tune tunableop status", Action: "Tune the kernels"}
	s.Problems = problemsOf(f.Doctor, stepChecks[StepTuning]...)
	if len(s.Problems) > 0 {
		s.State = StateTodo
		s.Summary = "The loaded rocBLAS does not match a TunableOp profile."
		return s
	}
	s.State = StateDone
	s.Summary = "The loaded rocBLAS matches a TunableOp profile."
	return s
}

// powerStep is an extra: it never holds the path.
func powerStep(f *Facts) Step {
	s := Step{ID: StepPower, Title: "Power cap (optional)", Command: "bernie tune power status", Action: "Set the power cap"}
	s.Problems = problemsOf(f.Doctor, stepChecks[StepPower]...)
	if len(s.Problems) > 0 {
		s.State = StateTodo
		s.Summary = "A GPU runs above the advised power cap."
		return s
	}
	s.State = StateDone
	s.Summary = "The power caps are within the advice."
	return s
}
