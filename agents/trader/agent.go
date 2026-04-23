package trader

import "trading-go/state"

type TraderAgent struct{}

func (a *TraderAgent) Name() string        { return "TraderAgent" }
func (a *TraderAgent) Description() string { return "Generate structured trading decision." }
func (a *TraderAgent) Run(s *state.AgentState) error {
	s.TraderDecision = state.TraderDecision{
		Direction:    "hold",
		Strength:     5,
		Confidence:   50,
		PositionSize: 0.0,
		Reasoning:    "TODO: trader reasoning",
	}
	return nil
}
