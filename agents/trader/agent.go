package trader

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sirupsen/logrus"

	"trading-go/internal/llm"
	"trading-go/internal/logx"
	"trading-go/internal/prompts"
	"trading-go/state"
	"trading-go/tools"
)

type TraderAgent struct{}

const traderExecutionHistoryLookbackDays = 90
const traderExecutionHistoryMaxItems = 20

func (a *TraderAgent) Name() string        { return "TraderAgent" }
func (a *TraderAgent) Description() string { return "Generate structured trading decision." }
func (a *TraderAgent) Run(ctx context.Context, s *state.AgentState) error {
	type decision struct {
		Direction    string  `json:"direction"`
		Strength     int     `json:"strength"`
		Confidence   int     `json:"confidence"`
		PositionSize float64 `json:"position_size"`
		Reasoning    string  `json:"reasoning"`
	}

	history, err := loadTraderExecutionHistory(s.Symbol)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Warn("failed to load trader execution history, continuing with empty history")
	}
	payload, err := buildTraderPayload(s, history)
	if err != nil {
		return err
	}

	var d decision
	if err := llm.GenerateJSON(
		ctx,
		prompts.TraderDecisionJSONSystem,
		payload,
		&d,
	); err != nil {
		return err
	}
	d.Direction = normalizeDirection(d.Direction)
	d.Strength = clampInt(d.Strength, 1, 10)
	d.Confidence = clampInt(d.Confidence, 0, 100)
	if d.PositionSize < 0 {
		d.PositionSize = 0
	}
	if d.PositionSize > 1 {
		d.PositionSize = 1
	}

	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":         a.Name(),
		"symbol":        s.Symbol,
		"direction":     d.Direction,
		"strength":      d.Strength,
		"confidence":    d.Confidence,
		"position_size": d.PositionSize,
	}).Info("generating trader decision")

	s.TraderDecision = state.TraderDecision{
		Direction:    d.Direction,
		Strength:     d.Strength,
		Confidence:   d.Confidence,
		PositionSize: d.PositionSize,
		Reasoning:    d.Reasoning,
	}
	return nil
}

func normalizeDirection(direction string) string {
	switch direction {
	case "buy", "sell", "hold":
		return direction
	default:
		return "hold"
	}
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

type traderPayload struct {
	Symbol           string                  `json:"symbol"`
	Fundamental      string                  `json:"fundamental"`
	Sentiment        string                  `json:"sentiment"`
	News             string                  `json:"news"`
	Technical        string                  `json:"technical"`
	ResearchSummary  string                  `json:"research_summary"`
	ExecutionHistory []traderExecutionRecord `json:"execution_history"`
}

type traderExecutionRecord struct {
	OrderID     string  `json:"order_id"`
	TradeID     string  `json:"trade_id"`
	Symbol      string  `json:"symbol"`
	TradeDoneAt string  `json:"trade_done_at"`
	Quantity    float64 `json:"quantity"`
	Price       float64 `json:"price"`
}

func loadTraderExecutionHistory(symbol string) ([]tools.ExecutionRecord, error) {
	endAt := time.Now().UTC()
	startAt := endAt.AddDate(0, 0, -traderExecutionHistoryLookbackDays)
	records, err := tools.GetHistoryExecutions(tools.ExecutionHistoryQuery{
		Symbol:  symbol,
		StartAt: startAt,
		EndAt:   endAt,
	})
	if err != nil {
		return nil, err
	}
	if len(records) > traderExecutionHistoryMaxItems {
		records = records[len(records)-traderExecutionHistoryMaxItems:]
	}
	return records, nil
}

func buildTraderPayload(s *state.AgentState, records []tools.ExecutionRecord) (string, error) {
	payload := traderPayload{
		Symbol:           s.Symbol,
		Fundamental:      s.FundamentalReport,
		Sentiment:        s.SentimentReport,
		News:             s.NewsReport,
		Technical:        s.TechnicalReport,
		ResearchSummary:  s.ResearchSummary,
		ExecutionHistory: buildTraderExecutionHistory(records),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildTraderExecutionHistory(records []tools.ExecutionRecord) []traderExecutionRecord {
	if len(records) == 0 {
		return nil
	}
	items := make([]traderExecutionRecord, 0, len(records))
	for _, record := range records {
		items = append(items, traderExecutionRecord{
			OrderID:     record.OrderID,
			TradeID:     record.TradeID,
			Symbol:      record.Symbol,
			TradeDoneAt: record.TradeDoneAt.UTC().Format(time.RFC3339),
			Quantity:    record.Quantity,
			Price:       record.Price,
		})
	}
	return items
}
