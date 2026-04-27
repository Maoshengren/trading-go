package research

import (
	"context"

	"trading-go/internal/prompts"
	"trading-go/state"
)

type BearResearcherAgent struct{}

func (a *BearResearcherAgent) Name() string { return "BearResearcherAgent" }

func (a *BearResearcherAgent) Description() string {
	return "Construct bearish thesis from analyst reports and rebut the latest bullish point."
}

func (a *BearResearcherAgent) Run(ctx context.Context, s *state.AgentState) error {
	return runDebateTurn(ctx, s, debateTurnConfig{
		agentName:       a.Name(),
		stance:          "bear",
		systemPrompt:    prompts.BearResearcherSystem,
		ownHistory:      s.BearArguments,
		opponentHistory: s.BullArguments,
		appendArgument:  func(arg string) { s.BearArguments = append(s.BearArguments, arg) },
	})
}
