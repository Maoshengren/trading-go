package research

import (
	"context"

	"trading-go/internal/prompts"
	"trading-go/state"
)

type BullResearcherAgent struct{}

func (a *BullResearcherAgent) Name() string { return "BullResearcherAgent" }

func (a *BullResearcherAgent) Description() string {
	return "Construct bullish thesis from analyst reports and rebut the latest bearish point."
}

func (a *BullResearcherAgent) Run(ctx context.Context, s *state.AgentState) error {
	return runDebateTurn(ctx, s, debateTurnConfig{
		agentName:       a.Name(),
		stance:          "bull",
		systemPrompt:    prompts.BullResearcherSystem,
		ownHistory:      s.BullArguments,
		opponentHistory: s.BearArguments,
		appendArgument:  func(arg string) { s.BullArguments = append(s.BullArguments, arg) },
	})
}
