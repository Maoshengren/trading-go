package research

import (
	"context"

	"github.com/sirupsen/logrus"

	"trading-go/internal/llm"
	"trading-go/internal/logx"
	"trading-go/internal/prompts"
	"trading-go/state"
)

type ManagerAgent struct {
	Rounds int
	bull   *BullResearcherAgent
	bear   *BearResearcherAgent
}

func NewResearchManagerAgent(rounds int, bull *BullResearcherAgent, bear *BearResearcherAgent) *ManagerAgent {
	return &ManagerAgent{
		Rounds: rounds,
		bull:   bull,
		bear:   bear,
	}
}

func (a *ManagerAgent) Name() string { return "ResearchManagerAgent" }
func (a *ManagerAgent) Description() string {
	return "Run multi-round bull/bear debate and output summary."
}
func (a *ManagerAgent) Run(ctx context.Context, s *state.AgentState) error {
	if a.Rounds <= 0 {
		a.Rounds = 2
	}
	if a.bull == nil {
		a.bull = &BullResearcherAgent{}
	}
	if a.bear == nil {
		a.bear = &BearResearcherAgent{}
	}

	s.BullArguments = nil
	s.BearArguments = nil
	s.ResearchDebateTurns = nil

	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":         a.Name(),
		"symbol":        s.Symbol,
		"debate_rounds": a.Rounds,
	}).Info("running research debate rounds")

	for round := 1; round <= a.Rounds; round++ {
		if err := a.bull.Run(ctx, s); err != nil {
			return err
		}
		if err := a.bear.Run(ctx, s); err != nil {
			return err
		}
		logx.Logger(ctx).WithFields(logrus.Fields{
			"agent":       a.Name(),
			"symbol":      s.Symbol,
			"round":       round,
			"bull_points": len(s.BullArguments),
			"bear_points": len(s.BearArguments),
		}).Info("research debate round completed")
	}

	summaryPayload, err := buildResearchSummaryPayload(s)
	if err != nil {
		return err
	}
	summary, err := llm.Generate(ctx, prompts.ResearchManagerSummarySystem, summaryPayload)
	if err != nil {
		return err
	}
	s.ResearchSummary = summary
	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":       a.Name(),
		"symbol":      s.Symbol,
		"bull_points": len(s.BullArguments),
		"bear_points": len(s.BearArguments),
		"turns":       len(s.ResearchDebateTurns),
	}).Info("research summary generated")
	return nil
}
