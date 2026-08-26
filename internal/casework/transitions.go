package casework

func AllowedTransition(from, to State) bool {
	allowed := map[State][]State{
		StateDetected:   {StateTriaged},
		StateTriaged:    {StatePlanned},
		StatePlanned:    {StatePlanned, StateEvidence},
		StateEvidence:   {StatePlanned, StateHypothesis},
		StateHypothesis: {StateMitigated},
		StateMitigated:  {StateMitigated, StateVerified},
		StateVerified:   {StateClosed},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func IsTerminal(state State) bool {
	return state == StateClosed
}
