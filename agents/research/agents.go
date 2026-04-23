package research

import (
	"fmt"

	"trading-go/state"
)

type BullResearcherAgent struct{}
type BearResearcherAgent struct{}
type ResearchManagerAgent struct {
	Rounds int
}

func (a *BullResearcherAgent) Name() string        { return "BullResearcherAgent" }
func (a *BullResearcherAgent) Description() string { return "Construct bullish thesis from analyst reports." }
func (a *BullResearcherAgent) Run(s *state.AgentState) error {
	s.BullArguments = append(s.BullArguments, "TODO: bullish argument")
	return nil
}

func (a *BearResearcherAgent) Name() string        { return "BearResearcherAgent" }
func (a *BearResearcherAgent) Description() string { return "Construct bearish thesis from analyst reports." }
func (a *BearResearcherAgent) Run(s *state.AgentState) error {
	s.BearArguments = append(s.BearArguments, "TODO: bearish argument")
	return nil
}

func (a *ResearchManagerAgent) Name() string        { return "ResearchManagerAgent" }
func (a *ResearchManagerAgent) Description() string { return "Run debate rounds and output summary." }
func (a *ResearchManagerAgent) Run(s *state.AgentState) error {
	s.ResearchSummary = fmt.Sprintf("TODO: debate summary after %d rounds", a.Rounds)
	return nil
}
