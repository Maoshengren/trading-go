package risk

import "trading-go/state"

type RiskAnalystAgent struct{}
type RiskManagerAgent struct{}

func (a *RiskAnalystAgent) Name() string        { return "RiskAnalystAgent" }
func (a *RiskAnalystAgent) Description() string { return "Assess volatility, liquidity, and drawdown risk." }
func (a *RiskAnalystAgent) Run(s *state.AgentState) error {
	s.RiskAssessmentReport = "TODO: risk assessment report"
	return nil
}

func (a *RiskManagerAgent) Name() string        { return "RiskManagerAgent" }
func (a *RiskManagerAgent) Description() string { return "Review risk report and trader decision." }
func (a *RiskManagerAgent) Run(s *state.AgentState) error {
	s.RiskReview = state.RiskReview{
		Conclusion: "approve",
		Suggestion: "TODO: risk manager suggestion",
	}
	return nil
}
