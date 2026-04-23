package portfolio

import "trading-go/state"

type PortfolioManagerAgent struct{}

func (a *PortfolioManagerAgent) Name() string        { return "PortfolioManagerAgent" }
func (a *PortfolioManagerAgent) Description() string { return "Approve and sign final execution decision." }
func (a *PortfolioManagerAgent) Run(s *state.AgentState) error {
	s.FinalDecision = "TODO: final portfolio decision"
	return nil
}
