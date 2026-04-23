package orchestrator

import (
	"trading-go/agents/analyst"
	"trading-go/agents/portfolio"
	"trading-go/agents/research"
	"trading-go/agents/risk"
	"trading-go/agents/trader"
	"trading-go/state"
)

type Workflow struct {
	Fundamental *analyst.FundamentalAnalystAgent
	Sentiment   *analyst.SentimentAnalystAgent
	News        *analyst.NewsAnalystAgent
	Technical   *analyst.TechnicalAnalystAgent
	Bull        *research.BullResearcherAgent
	Bear        *research.BearResearcherAgent
	ResearchMgr *research.ResearchManagerAgent
	Trader      *trader.TraderAgent
	RiskAnalyst *risk.RiskAnalystAgent
	RiskManager *risk.RiskManagerAgent
	Portfolio   *portfolio.PortfolioManagerAgent
}

func NewWorkflow(debateRounds int) *Workflow {
	return &Workflow{
		Fundamental: &analyst.FundamentalAnalystAgent{},
		Sentiment:   &analyst.SentimentAnalystAgent{},
		News:        &analyst.NewsAnalystAgent{},
		Technical:   &analyst.TechnicalAnalystAgent{},
		Bull:        &research.BullResearcherAgent{},
		Bear:        &research.BearResearcherAgent{},
		ResearchMgr: &research.ResearchManagerAgent{Rounds: debateRounds},
		Trader:      &trader.TraderAgent{},
		RiskAnalyst: &risk.RiskAnalystAgent{},
		RiskManager: &risk.RiskManagerAgent{},
		Portfolio:   &portfolio.PortfolioManagerAgent{},
	}
}

func (w *Workflow) Run(s *state.AgentState) error {
	// TODO: replace with Eino ADK Graph/Supervisor orchestration.
	if err := w.Fundamental.Run(s); err != nil {
		return err
	}
	if err := w.Sentiment.Run(s); err != nil {
		return err
	}
	if err := w.News.Run(s); err != nil {
		return err
	}
	if err := w.Technical.Run(s); err != nil {
		return err
	}
	if err := w.Bull.Run(s); err != nil {
		return err
	}
	if err := w.Bear.Run(s); err != nil {
		return err
	}
	if err := w.ResearchMgr.Run(s); err != nil {
		return err
	}
	if err := w.Trader.Run(s); err != nil {
		return err
	}
	if err := w.RiskAnalyst.Run(s); err != nil {
		return err
	}
	if err := w.RiskManager.Run(s); err != nil {
		return err
	}
	if err := w.Portfolio.Run(s); err != nil {
		return err
	}
	return nil
}
