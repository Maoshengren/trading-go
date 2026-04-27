package risk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"trading-go/internal/llm"
	"trading-go/internal/logx"
	"trading-go/internal/prompts"
	"trading-go/state"
	"trading-go/tools"
	technicaltools "trading-go/tools/technical"
)

type RiskAnalystAgent struct{}
type RiskManagerAgent struct{}

const riskMarketLookbackBars = 20

type riskAnalystPayload struct {
	Symbol           string               `json:"symbol"`
	TraderDecision   state.TraderDecision `json:"trader_decision"`
	PositionSnapshot riskPositionSnapshot `json:"position_snapshot"`
	MarketData       riskMarketData       `json:"market_data"`
}

type riskPositionSnapshot struct {
	AccountMode      string              `json:"account_mode"`
	CashBalances     []riskCashBalance   `json:"cash_balances"`
	StockPositions   []riskStockPosition `json:"stock_positions"`
	MatchingPosition *riskStockPosition  `json:"matching_position,omitempty"`
}

type riskCashBalance struct {
	Currency      string  `json:"currency"`
	AvailableCash float64 `json:"available_cash"`
	TotalCash     float64 `json:"total_cash"`
	NetAssets     float64 `json:"net_assets"`
	RiskLevel     string  `json:"risk_level"`
}

type riskStockPosition struct {
	AccountChannel    string  `json:"account_channel"`
	Symbol            string  `json:"symbol"`
	SymbolName        string  `json:"symbol_name"`
	Quantity          float64 `json:"quantity"`
	AvailableQuantity float64 `json:"available_quantity"`
	Currency          string  `json:"currency"`
	CostPrice         float64 `json:"cost_price"`
	Market            string  `json:"market"`
}

type riskMarketData struct {
	Snapshot      *technicaltools.TechnicalSnapshot `json:"snapshot"`
	RecentBars    []riskMarketBar                   `json:"recent_bars"`
	CurrentPrice  float64                           `json:"current_price"`
	RecentHigh    float64                           `json:"recent_high"`
	RecentLow     float64                           `json:"recent_low"`
	AverageVolume float64                           `json:"average_volume"`
}

type riskMarketBar struct {
	Time   string  `json:"time"`
	Close  float64 `json:"close"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Volume float64 `json:"volume"`
}

func (a *RiskAnalystAgent) Name() string { return "RiskAnalystAgent" }
func (a *RiskAnalystAgent) Description() string {
	return "Assess volatility, liquidity, and drawdown risk."
}
func (a *RiskAnalystAgent) Run(ctx context.Context, s *state.AgentState) error {
	payload, err := buildRiskAnalystPayload(ctx, s, riskAnalysisPeriod(ctx))
	if err != nil {
		return err
	}
	report, err := llm.Generate(
		ctx,
		prompts.RiskAnalystSystem,
		payload,
	)
	if err != nil {
		return err
	}
	s.RiskAssessmentReport = report

	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":  a.Name(),
		"symbol": s.Symbol,
	}).Info("generating risk assessment report")
	return nil
}

func (a *RiskManagerAgent) Name() string        { return "RiskManagerAgent" }
func (a *RiskManagerAgent) Description() string { return "Review risk report and trader decision." }
func (a *RiskManagerAgent) Run(ctx context.Context, s *state.AgentState) error {
	type review struct {
		Conclusion string `json:"conclusion"`
		Suggestion string `json:"suggestion"`
	}
	var r review
	if err := llm.GenerateJSON(
		ctx,
		prompts.RiskManagerJSONSystem,
		fmt.Sprintf("symbol=%s trader_decision=%+v risk_assessment=%s", s.Symbol, s.TraderDecision, s.RiskAssessmentReport),
		&r,
	); err != nil {
		return err
	}
	conclusion := normalizeConclusion(r.Conclusion)
	suggestion := r.Suggestion
	s.RiskReview = state.RiskReview{
		Conclusion: conclusion,
		Suggestion: suggestion,
	}

	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":      a.Name(),
		"symbol":     s.Symbol,
		"conclusion": conclusion,
	}).Info("reviewing risk decision")
	return nil
}

func normalizeConclusion(v string) string {
	switch v {
	case "approve", "modify", "reject":
		return v
	default:
		return "modify"
	}
}

func buildRiskAnalystPayload(ctx context.Context, s *state.AgentState, period string) (string, error) {
	accountSnapshot, err := tools.GetAccountSnapshot([]string{s.Symbol})
	if err != nil {
		return "", fmt.Errorf("load account snapshot: %w", err)
	}

	klines, err := tools.GetKline(s.Symbol, period)
	if err != nil {
		return "", fmt.Errorf("load market kline: %w", err)
	}
	snapshot, err := technicaltools.BuildTechnicalSnapshot(klines)
	if err != nil {
		return "", fmt.Errorf("build market snapshot: %w", err)
	}

	payload := riskAnalystPayload{
		Symbol:           s.Symbol,
		TraderDecision:   s.TraderDecision,
		PositionSnapshot: buildRiskPositionSnapshot(s.Symbol, accountSnapshot),
		MarketData:       buildRiskMarketData(klines, snapshot),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal risk analyst payload: %w", err)
	}
	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":            "RiskAnalystAgent",
		"symbol":           s.Symbol,
		"position_count":   len(payload.PositionSnapshot.StockPositions),
		"cash_balance_cnt": len(payload.PositionSnapshot.CashBalances),
		"market_bar_count": len(payload.MarketData.RecentBars),
	}).Info("built risk analyst payload")
	return string(data), nil
}

type riskPeriodKey struct{}

func WithAnalysisPeriod(ctx context.Context, period string) context.Context {
	return context.WithValue(ctx, riskPeriodKey{}, strings.ToLower(strings.TrimSpace(period)))
}

func riskAnalysisPeriod(ctx context.Context) string {
	if ctx != nil {
		if v, ok := ctx.Value(riskPeriodKey{}).(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "1d"
}

func buildRiskPositionSnapshot(symbol string, snapshot *tools.AccountSnapshot) riskPositionSnapshot {
	if snapshot == nil {
		return riskPositionSnapshot{}
	}

	result := riskPositionSnapshot{
		AccountMode:    snapshot.AccountMode,
		CashBalances:   make([]riskCashBalance, 0, len(snapshot.CashBalances)),
		StockPositions: make([]riskStockPosition, 0, len(snapshot.StockPositions)),
	}
	normalizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	for _, item := range snapshot.CashBalances {
		result.CashBalances = append(result.CashBalances, riskCashBalance{
			Currency:      item.Currency,
			AvailableCash: item.AvailableCash,
			TotalCash:     item.TotalCash,
			NetAssets:     item.NetAssets,
			RiskLevel:     item.RiskLevel,
		})
	}
	for _, item := range snapshot.StockPositions {
		position := riskStockPosition{
			AccountChannel:    item.AccountChannel,
			Symbol:            item.Symbol,
			SymbolName:        item.SymbolName,
			Quantity:          item.Quantity,
			AvailableQuantity: item.AvailableQuantity,
			Currency:          item.Currency,
			CostPrice:         item.CostPrice,
			Market:            item.Market,
		}
		result.StockPositions = append(result.StockPositions, position)
		if symbolMatchesPosition(normalizedSymbol, item.Symbol) {
			p := position
			result.MatchingPosition = &p
		}
	}
	return result
}

func buildRiskMarketData(klines []tools.KLine, snapshot *technicaltools.TechnicalSnapshot) riskMarketData {
	result := riskMarketData{
		Snapshot:   snapshot,
		RecentBars: buildRiskMarketBars(klines, riskMarketLookbackBars),
	}
	if len(klines) == 0 {
		return result
	}

	result.CurrentPrice = klines[len(klines)-1].Close
	start := len(klines) - riskMarketLookbackBars
	if start < 0 {
		start = 0
	}
	segment := klines[start:]
	result.RecentHigh = segment[0].High
	result.RecentLow = segment[0].Low
	var volumeSum float64
	for _, k := range segment {
		if k.High > result.RecentHigh {
			result.RecentHigh = k.High
		}
		if k.Low < result.RecentLow {
			result.RecentLow = k.Low
		}
		volumeSum += k.Volume
	}
	result.AverageVolume = volumeSum / float64(len(segment))
	return result
}

func buildRiskMarketBars(klines []tools.KLine, limit int) []riskMarketBar {
	if limit <= 0 || len(klines) == 0 {
		return nil
	}
	start := len(klines) - limit
	if start < 0 {
		start = 0
	}
	bars := make([]riskMarketBar, 0, len(klines)-start)
	for _, k := range klines[start:] {
		bars = append(bars, riskMarketBar{
			Time:   k.Time.UTC().Format(time.RFC3339),
			Close:  k.Close,
			High:   k.High,
			Low:    k.Low,
			Volume: k.Volume,
		})
	}
	return bars
}

func symbolMatchesPosition(requested, actual string) bool {
	requested = strings.ToUpper(strings.TrimSpace(requested))
	actual = strings.ToUpper(strings.TrimSpace(actual))
	if requested == "" || actual == "" {
		return false
	}
	if requested == actual {
		return true
	}
	if strings.Contains(requested, ".") {
		requested = strings.SplitN(requested, ".", 2)[0]
	}
	if strings.Contains(actual, ".") {
		actual = strings.SplitN(actual, ".", 2)[0]
	}
	return requested == actual
}
