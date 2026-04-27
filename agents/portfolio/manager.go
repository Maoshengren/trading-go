package portfolio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

type final struct {
	Execution             string  `json:"execution"`
	Action                string  `json:"action"`
	Direction             string  `json:"direction"`
	ApprovedPositionSize  float64 `json:"approved_position_size"`
	ApprovedPositionRatio float64 `json:"approved_position_ratio"`
	TargetPositionQty     float64 `json:"target_position_qty"`
	Summary               string  `json:"summary"`
}

func (a *ManagerAgent) Name() string { return "PortfolioManagerAgent" }
func (a *ManagerAgent) Description() string {
	return "Approve and sign final execution decision."
}
func (a *ManagerAgent) Run(ctx context.Context, s *state.AgentState) error {
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
	action := normalizeAction(f.Action, execution)
	direction := normalizePortfolioDirection(f.Direction, s.TraderDecision.Direction, action, f.Summary)
	approvedPositionRatio := clampPositionRatio(f.ApprovedPositionRatio, execution, s.TraderDecision.PositionSize)
	targetPositionQty := clampNonNegative(f.TargetPositionQty)
	approvedPositionSize := normalizeApprovedPositionSize(f.ApprovedPositionSize, execution, action, approvedPositionRatio, targetPositionQty, s.TraderDecision.PositionSize)
	s.PortfolioDecision = state.PortfolioDecision{
		Execution:             execution,
		Action:                action,
		Direction:             direction,
		Summary:               f.Summary,
		ApprovedPositionSize:  approvedPositionSize,
		ApprovedPositionRatio: approvedPositionRatio,
		TargetPositionQty:     targetPositionQty,
	}
	s.FinalDecision = fmt.Sprintf(
		"%s | action=%s | direction=%s | approved_position_size=%.4f | approved_position_ratio=%.4f | target_position_qty=%.8f | %s | sign=PortfolioManager(LLM)",
		execution,
		action,
		direction,
		approvedPositionSize,
		approvedPositionRatio,
		targetPositionQty,
		f.Summary,
	)

	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":                  a.Name(),
		"symbol":                 s.Symbol,
		"execution":              execution,
		"action":                 action,
		"direction":              direction,
		"approved_position_size": approvedPositionSize,
		"target_position_qty":    targetPositionQty,
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

func normalizeAction(v, execution string) string {
	if execution == "拒绝" {
		return "reject"
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "open", "increase", "reduce", "close", "hold", "reject":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		if execution == "拒绝" {
			return "reject"
		}
		return "hold"
	}
}

func normalizePortfolioDirection(v, traderDirection, action, summary string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "buy", "sell", "hold":
		return strings.ToLower(strings.TrimSpace(v))
	}
	if action == "reject" || action == "hold" {
		return "hold"
	}
	switch strings.ToLower(strings.TrimSpace(traderDirection)) {
	case "buy", "sell":
		return strings.ToLower(strings.TrimSpace(traderDirection))
	}
	lowerSummary := strings.ToLower(summary)
	switch {
	case strings.Contains(summary, "空头") && (action == "reduce" || action == "close"):
		return "buy"
	case strings.Contains(summary, "多头") && (action == "reduce" || action == "close"):
		return "sell"
	case strings.Contains(lowerSummary, "short") && (action == "reduce" || action == "close"):
		return "buy"
	case strings.Contains(lowerSummary, "long") && (action == "reduce" || action == "close"):
		return "sell"
	default:
		return "hold"
	}
}

func normalizeApprovedPositionSize(v float64, execution, action string, ratio, targetQty, traderPositionSize float64) float64 {
	if execution == "拒绝" {
		return 0
	}
	if action == "reduce" || action == "close" {
		if targetQty > 0 {
			return targetQty
		}
	}
	if v > 0 {
		return clampNonNegative(v)
	}
	return clampPositionRatio(ratio, execution, traderPositionSize)
}

func clampPositionRatio(v float64, execution string, traderPositionSize float64) float64 {
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

func clampNonNegative(v float64) float64 {
	if v < 0 {
		return 0
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
