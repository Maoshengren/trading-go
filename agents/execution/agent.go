package execution

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/sirupsen/logrus"

	"trading-go/internal/logx"
	"trading-go/state"
	"trading-go/tools"
)

type Agent struct {
	Enabled                 bool
	DryRun                  bool
	OrderType               string
	TimeInForce             string
	Leverage                int
	PricePeriod             string
	ProtectiveOrdersEnabled bool
	TakeProfitPercent       float64
	StopLossPercent         float64
}

func (a *Agent) Name() string { return "ExecutionAgent" }

func (a *Agent) Description() string {
	return "Execute approved orders or produce a dry-run execution plan."
}

func (a *Agent) Run(ctx context.Context, s *state.AgentState) error {
	result, err := a.buildExecutionResult(ctx, s)
	if err != nil {
		return err
	}
	s.ExecutionResult = *result
	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":      a.Name(),
		"symbol":     result.Symbol,
		"status":     result.Status,
		"dry_run":    result.DryRun,
		"side":       result.Side,
		"quantity":   result.Quantity,
		"reference":  result.ReferencePrice,
		"order_type": result.OrderType,
		"leverage":   result.Leverage,
		"order_id":   result.OrderID,
		"tp_order":   result.TakeProfitOrderID,
		"sl_order":   result.StopLossOrderID,
	}).Info("execution agent completed")
	return nil
}

func (a *Agent) buildExecutionResult(ctx context.Context, s *state.AgentState) (*state.ExecutionResult, error) {
	cfg := a.normalized()
	result := &state.ExecutionResult{
		Status:      "skipped",
		DryRun:      cfg.DryRun,
		Symbol:      s.Symbol,
		OrderType:   strings.ToLower(cfg.OrderType),
		TimeInForce: cfg.TimeInForce,
		Leverage:    cfg.Leverage,
	}

	if !cfg.Enabled {
		result.Reason = "execution disabled by config"
		return result, nil
	}

	verdict := parseFinalExecutionVerdict(s.FinalDecision)
	if verdict == "reject" {
		result.Reason = "portfolio decision rejected execution"
		return result, nil
	}

	side, hasApprovedSide := mapDirectionToSide(firstNonEmpty(s.PortfolioDecision.Direction, s.TraderDecision.Direction))
	portfolioReduction := shouldReduceFromPortfolio(s)
	if !hasApprovedSide && !portfolioReduction {
		result.Reason = "trader direction is hold or unsupported"
		return result, nil
	}

	positionFactor := approvedExecutionSize(s.PortfolioDecision)
	if positionFactor <= 0 {
		result.Reason = "approved position size is zero"
		return result, nil
	}

	accountSnapshot, err := tools.GetAccountSnapshot([]string{s.Symbol})
	if err != nil {
		return nil, fmt.Errorf("load account snapshot for execution: %w", err)
	}
	klines, err := tools.GetKline(s.Symbol, cfg.PricePeriod)
	if err != nil {
		return nil, fmt.Errorf("load execution price kline: %w", err)
	}
	if len(klines) == 0 {
		return nil, fmt.Errorf("execution price kline is empty")
	}
	refPrice := klines[len(klines)-1].Close
	if refPrice <= 0 {
		return nil, fmt.Errorf("invalid execution reference price")
	}
	result.ReferencePrice = refPrice

	currentPos, hasPosition := matchingPosition(s.Symbol, accountSnapshot)
	if !hasApprovedSide {
		if portfolioReduction && hasPosition {
			var ok bool
			side, ok = sideForReducingPosition(currentPos)
			if ok {
				result.Reason = "portfolio requested reduction while trader direction is hold; inferred reduce side from current position"
			}
		}
		if side == "" {
			result.Reason = "portfolio requested reduction but no matching position was found"
			return result, nil
		}
	}
	result.Side = side

	if hasPosition {
		reduceQty, reduceSide, reduce, positionSide := reductionPlan(side, currentPos, positionFactor, refPrice, accountSnapshot)
		if reduce {
			if reduceQty <= 0 {
				result.Reason = "reduction quantity is zero"
				return result, nil
			}
			result.Side = reduceSide
			result.Quantity = reduceQty
			result.ReduceOnly = true
			result.PositionSide = positionSide

			if cfg.DryRun {
				result.Status = "planned"
				result.Reason = fmt.Sprintf("dry-run reduction plan generated for existing position qty=%.8f approved_position_size=%.4f", currentPos.Quantity, positionFactor)
				return result, nil
			}

			if cfg.Leverage > 1 {
				if _, err := tools.SetLeverage(tools.LeverageRequest{
					Symbol:   s.Symbol,
					Leverage: cfg.Leverage,
				}); err != nil {
					return nil, fmt.Errorf("set leverage before reduction: %w", err)
				}
			}

			submission, err := tools.SubmitOrder(tools.OrderRequest{
				Symbol:       s.Symbol,
				OrderType:    cfg.OrderType,
				Side:         reduceSide,
				Quantity:     reduceQty,
				TimeInForce:  cfg.TimeInForce,
				ReduceOnly:   true,
				PositionSide: positionSide,
			})
			if err != nil {
				return nil, fmt.Errorf("submit reduction order: %w", err)
			}

			result.Status = "submitted"
			result.OrderID = submission.OrderID
			result.Reason = fmt.Sprintf("submitted reduce-only order against existing position qty=%.8f approved_position_size=%.4f", currentPos.Quantity, positionFactor)
			return result, nil
		}
	}

	availableCash, err := executionAvailableCash(s.Symbol, accountSnapshot)
	if err != nil {
		result.Reason = err.Error()
		return result, nil
	}

	notional := availableCash * positionFactor * float64(cfg.Leverage)
	qty := notional / refPrice
	if qty <= 0 {
		result.Reason = "calculated order quantity is zero"
		return result, nil
	}
	result.Quantity = qty
	result.PositionSide = "BOTH"
	applyProtectivePrices(result, cfg, side, refPrice)

	if cfg.DryRun {
		result.Status = "planned"
		result.Reason = fmt.Sprintf("dry-run execution plan generated from portfolio verdict=%s approved_position_size=%.4f", verdict, positionFactor)
		if cfg.ProtectiveOrdersEnabled && protectivePricesReady(result) {
			result.ProtectionStatus = "planned"
			result.ProtectionReason = "dry-run protective take-profit and stop-loss orders planned"
		}
		return result, nil
	}

	if cfg.Leverage > 1 {
		if _, err := tools.SetLeverage(tools.LeverageRequest{
			Symbol:   s.Symbol,
			Leverage: cfg.Leverage,
		}); err != nil {
			return nil, fmt.Errorf("set leverage before execution: %w", err)
		}
	}

	submission, err := tools.SubmitOrder(tools.OrderRequest{
		Symbol:       s.Symbol,
		OrderType:    cfg.OrderType,
		Side:         side,
		Quantity:     qty,
		TimeInForce:  cfg.TimeInForce,
		ReduceOnly:   false,
		PositionSide: result.PositionSide,
	})
	if err != nil {
		return nil, fmt.Errorf("submit execution order: %w", err)
	}

	result.Status = "submitted"
	result.OrderID = submission.OrderID
	result.Reason = fmt.Sprintf("order submitted after portfolio verdict=%s approved_position_size=%.4f", verdict, positionFactor)
	a.submitProtectiveOrders(ctx, s.Symbol, result, cfg)
	return result, nil
}

func (a *Agent) submitProtectiveOrders(ctx context.Context, symbol string, result *state.ExecutionResult, cfg Agent) {
	if !cfg.ProtectiveOrdersEnabled {
		result.ProtectionStatus = "skipped"
		result.ProtectionReason = "protective orders disabled by config"
		return
	}
	if result.ReduceOnly {
		result.ProtectionStatus = "skipped"
		result.ProtectionReason = "reduction orders do not add exposure"
		return
	}
	if !protectivePricesReady(result) {
		result.ProtectionStatus = "skipped"
		result.ProtectionReason = "take-profit or stop-loss percent is not configured"
		return
	}

	closeSide := oppositeSide(result.Side)
	if closeSide == "" {
		result.ProtectionStatus = "skipped"
		result.ProtectionReason = "unsupported side for protective orders"
		return
	}

	var failures []string
	if result.TakeProfitPrice > 0 {
		submission, err := tools.SubmitOrder(tools.OrderRequest{
			Symbol:       symbol,
			OrderType:    "take_profit_market",
			Side:         closeSide,
			Quantity:     result.Quantity,
			TriggerPrice: result.TakeProfitPrice,
			ReduceOnly:   true,
			PositionSide: result.PositionSide,
			WorkingType:  "MARK_PRICE",
			Remark:       "tp-" + symbol,
		})
		if err != nil {
			failures = append(failures, fmt.Sprintf("take-profit: %v", err))
		} else {
			result.TakeProfitOrderID = submission.OrderID
		}
	}
	if result.StopLossPrice > 0 {
		submission, err := tools.SubmitOrder(tools.OrderRequest{
			Symbol:       symbol,
			OrderType:    "stop_market",
			Side:         closeSide,
			Quantity:     result.Quantity,
			TriggerPrice: result.StopLossPrice,
			ReduceOnly:   true,
			PositionSide: result.PositionSide,
			WorkingType:  "MARK_PRICE",
			Remark:       "sl-" + symbol,
		})
		if err != nil {
			failures = append(failures, fmt.Sprintf("stop-loss: %v", err))
		} else {
			result.StopLossOrderID = submission.OrderID
		}
	}

	switch {
	case len(failures) == 0:
		result.ProtectionStatus = "submitted"
		result.ProtectionReason = "protective take-profit and stop-loss orders submitted"
	case result.TakeProfitOrderID != "" || result.StopLossOrderID != "":
		result.ProtectionStatus = "partial"
		result.ProtectionReason = strings.Join(failures, "; ")
		logx.Logger(ctx).WithField("protection_reason", result.ProtectionReason).Warn("partial protective order submission")
	default:
		result.ProtectionStatus = "failed"
		result.ProtectionReason = strings.Join(failures, "; ")
		logx.Logger(ctx).WithField("protection_reason", result.ProtectionReason).Warn("protective order submission failed")
	}
}

func executionAvailableCash(symbol string, snapshot *tools.AccountSnapshot) (float64, error) {
	if snapshot == nil {
		return 0, fmt.Errorf("account snapshot is empty")
	}
	quote := inferQuoteAsset(symbol)
	for _, item := range snapshot.CashBalances {
		if strings.EqualFold(item.Currency, quote) {
			if item.AvailableCash > 0 {
				return item.AvailableCash, nil
			}
			if item.TotalCash > 0 {
				return item.TotalCash, nil
			}
		}
	}
	for _, item := range snapshot.CashBalances {
		if item.AvailableCash > 0 {
			return item.AvailableCash, nil
		}
		if item.TotalCash > 0 {
			return item.TotalCash, nil
		}
	}
	return 0, fmt.Errorf("no available cash found for execution")
}

func matchingPosition(symbol string, snapshot *tools.AccountSnapshot) (tools.StockPosition, bool) {
	if snapshot == nil {
		return tools.StockPosition{}, false
	}
	for _, item := range snapshot.StockPositions {
		if symbolMatches(symbol, item.Symbol) && item.Quantity != 0 {
			return item, true
		}
	}
	return tools.StockPosition{}, false
}

func reductionPlan(side string, pos tools.StockPosition, approvedPositionSize, refPrice float64, snapshot *tools.AccountSnapshot) (qty float64, submitSide string, reduce bool, positionSide string) {
	positionSide = normalizePositionSide(pos.PositionSide)
	currentQty := pos.Quantity
	switch {
	case currentQty > 0 && side == "sell":
		submitSide = "sell"
		reduce = true
	case currentQty < 0 && side == "buy":
		submitSide = "buy"
		reduce = true
	default:
		return 0, side, false, positionSide
	}

	currentAbsQty := math.Abs(currentQty)
	if isCryptoFuturesPosition(pos) && approvedPositionSize > 0 && approvedPositionSize < currentAbsQty {
		return currentAbsQty - approvedPositionSize, submitSide, true, positionSide
	}

	targetQty := desiredExecutionQuantity(approvedPositionSize, refPrice, snapshot)
	if targetQty <= 0 || targetQty > currentAbsQty {
		targetQty = currentAbsQty
	}
	return targetQty, submitSide, true, positionSide
}

func approvedExecutionSize(d state.PortfolioDecision) float64 {
	switch normalizePortfolioAction(d.Action) {
	case "reduce", "close":
		if d.TargetPositionQty > 0 {
			return d.TargetPositionQty
		}
	}
	if d.ApprovedPositionSize > 0 {
		return d.ApprovedPositionSize
	}
	if d.ApprovedPositionRatio > 0 {
		return d.ApprovedPositionRatio
	}
	return 0
}

func desiredExecutionQuantity(positionFactor, refPrice float64, snapshot *tools.AccountSnapshot) float64 {
	if positionFactor <= 0 || refPrice <= 0 {
		return 0
	}
	availableCash, err := executionAvailableCash("", snapshot)
	if err != nil || availableCash <= 0 {
		return 0
	}
	return availableCash * positionFactor / refPrice
}

func normalizePositionSide(v string) string {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "LONG":
		return "LONG"
	case "SHORT":
		return "SHORT"
	default:
		return "BOTH"
	}
}

func symbolMatches(requested, actual string) bool {
	requested = strings.ToUpper(strings.TrimSpace(requested))
	actual = strings.ToUpper(strings.TrimSpace(actual))
	if requested == actual {
		return true
	}
	for _, quote := range []string{"USDT", "USDC", "FDUSD", "BUSD", "BTC", "ETH", "BNB"} {
		requested = strings.TrimSuffix(requested, quote)
		actual = strings.TrimSuffix(actual, quote)
	}
	return requested == actual
}

func inferQuoteAsset(symbol string) string {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	for _, quote := range []string{"USDT", "USDC", "FDUSD", "BUSD", "BTC", "ETH", "BNB"} {
		if strings.HasSuffix(symbol, quote) {
			return quote
		}
	}
	return "USD"
}

func mapDirectionToSide(direction string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "buy":
		return "buy", true
	case "sell":
		return "sell", true
	default:
		return "", false
	}
}

func shouldReduceFromPortfolio(s *state.AgentState) bool {
	if s == nil {
		return false
	}
	switch normalizePortfolioAction(s.PortfolioDecision.Action) {
	case "reduce", "close":
		return true
	}
	if normalizeExecution(s.PortfolioDecision.Execution) != "调整后执行" && parseFinalExecutionVerdict(s.FinalDecision) != "modify" {
		return false
	}
	text := strings.ToLower(s.FinalDecision + " " + s.PortfolioDecision.Summary + " " + s.RiskReview.Suggestion)
	for _, keyword := range []string{"减仓", "降低仓位", "降低暴露", "reduce", "trim", "decrease"} {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func sideForReducingPosition(pos tools.StockPosition) (string, bool) {
	switch {
	case pos.Quantity > 0:
		return "sell", true
	case pos.Quantity < 0:
		return "buy", true
	default:
		return "", false
	}
}

func isCryptoFuturesPosition(pos tools.StockPosition) bool {
	market := strings.ToLower(strings.TrimSpace(pos.Market))
	channel := strings.ToLower(strings.TrimSpace(pos.AccountChannel))
	symbol := strings.ToUpper(strings.TrimSpace(pos.Symbol))
	return strings.Contains(market, "crypto") || strings.Contains(channel, "futures") || strings.HasSuffix(symbol, "USDT")
}

func normalizeExecution(v string) string {
	switch strings.TrimSpace(v) {
	case "执行", "拒绝", "调整后执行":
		return strings.TrimSpace(v)
	default:
		return ""
	}
}

func normalizePortfolioAction(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "open", "increase", "reduce", "close", "hold", "reject":
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func applyProtectivePrices(result *state.ExecutionResult, cfg Agent, side string, refPrice float64) {
	if !cfg.ProtectiveOrdersEnabled || refPrice <= 0 {
		return
	}
	takeProfit := normalizePercent(cfg.TakeProfitPercent)
	stopLoss := normalizePercent(cfg.StopLossPercent)
	switch side {
	case "buy":
		if takeProfit > 0 {
			result.TakeProfitPrice = refPrice * (1 + takeProfit)
		}
		if stopLoss > 0 && stopLoss < 1 {
			result.StopLossPrice = refPrice * (1 - stopLoss)
		}
	case "sell":
		if takeProfit > 0 && takeProfit < 1 {
			result.TakeProfitPrice = refPrice * (1 - takeProfit)
		}
		if stopLoss > 0 {
			result.StopLossPrice = refPrice * (1 + stopLoss)
		}
	}
}

func protectivePricesReady(result *state.ExecutionResult) bool {
	return result.TakeProfitPrice > 0 && result.StopLossPrice > 0
}

func normalizePercent(v float64) float64 {
	if v > 1 {
		return v / 100
	}
	return v
}

func oppositeSide(side string) string {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "buy":
		return "sell"
	case "sell":
		return "buy"
	default:
		return ""
	}
}

func parseFinalExecutionVerdict(finalDecision string) string {
	switch {
	case strings.HasPrefix(strings.TrimSpace(finalDecision), "执行"):
		return "approve"
	case strings.HasPrefix(strings.TrimSpace(finalDecision), "调整后执行"):
		return "modify"
	case strings.HasPrefix(strings.TrimSpace(finalDecision), "拒绝"):
		return "reject"
	default:
		return "modify"
	}
}

func (a *Agent) normalized() Agent {
	out := *a
	if out.OrderType == "" {
		out.OrderType = "market"
	}
	if out.TimeInForce == "" {
		out.TimeInForce = "GTC"
	}
	if out.PricePeriod == "" {
		out.PricePeriod = "1m"
	}
	if out.Leverage <= 0 {
		out.Leverage = 1
	}
	if out.TakeProfitPercent < 0 {
		out.TakeProfitPercent = 0
	}
	if out.StopLossPercent < 0 {
		out.StopLossPercent = 0
	}
	return out
}
