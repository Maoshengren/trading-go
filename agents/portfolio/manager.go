package portfolio

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"

	"trading-go/internal/llm"
	"trading-go/internal/logx"
	"trading-go/internal/prompts"
	"trading-go/state"
	"trading-go/tools"
)

type ManagerAgent struct{}

type portfolioPayload struct {
	Symbol           string                 `json:"symbol"`
	TraderDecision   state.TraderDecision   `json:"trader_decision"`
	RiskReview       state.RiskReview       `json:"risk_review"`
	RiskReport       string                 `json:"risk_report"`
	PositionSnapshot *tools.AccountSnapshot `json:"position_snapshot,omitempty"`
}

func (a *ManagerAgent) Name() string { return "PortfolioManagerAgent" }
func (a *ManagerAgent) Description() string {
	return "Approve and sign final execution decision."
}
func (a *ManagerAgent) Run(ctx context.Context, s *state.AgentState) error {
	type final struct {
		Execution            string  `json:"execution"`
		ApprovedPositionSize float64 `json:"approved_position_size"`
		Summary              string  `json:"summary"`
	}
	var f final

	payload, err := buildPortfolioPayload(ctx, s)
	if err != nil {
		return err
	}
	if err := llm.GenerateJSON(
		ctx,
		prompts.PortfolioManagerJSONSystem,
		payload,
		&f,
	); err != nil {
		return err
	}

	execution := normalizeExecution(f.Execution)
	approvedPositionSize := clampPositionSize(f.ApprovedPositionSize, execution, s.TraderDecision.PositionSize)
	s.PortfolioDecision = state.PortfolioDecision{
		Execution:            execution,
		Summary:              f.Summary,
		ApprovedPositionSize: approvedPositionSize,
	}
	s.FinalDecision = fmt.Sprintf(
		"%s | %s | sign=PortfolioManager(LLM)",
		execution,
		f.Summary,
	)

	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":                  a.Name(),
		"symbol":                 s.Symbol,
		"execution":              execution,
		"approved_position_size": approvedPositionSize,
		"risk_check":             s.RiskReview.Conclusion,
	}).Info("producing final portfolio decision")
	return nil
}

func normalizeExecution(v string) string {
	switch v {
	case "执行", "拒绝", "调整后执行":
		return v
	default:
		return "调整后执行"
	}
}

func clampPositionSize(v float64, execution string, traderPositionSize float64) float64 {
	if execution == "拒绝" {
		return 0
	}
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	if execution == "调整后执行" && traderPositionSize > 0 && v > traderPositionSize {
		return traderPositionSize
	}
	if v == 0 && execution == "执行" {
		if traderPositionSize > 0 {
			return traderPositionSize
		}
	}
	return v
}

func buildPortfolioPayload(ctx context.Context, s *state.AgentState) (string, error) {
	snapshot, err := tools.GetAccountSnapshot([]string{s.Symbol})
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("symbol", s.Symbol).Warn("failed to load account snapshot for portfolio decision, continuing without it")
	}

	payload := portfolioPayload{
		Symbol:           s.Symbol,
		TraderDecision:   s.TraderDecision,
		RiskReview:       s.RiskReview,
		RiskReport:       s.RiskAssessmentReport,
		PositionSnapshot: snapshot,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
