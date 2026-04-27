package execution

import (
	"context"
	"math"
	"testing"

	"trading-go/state"
	"trading-go/tools"
)

func TestParseFinalExecutionVerdict(t *testing.T) {
	tests := map[string]string{
		"执行 | ok":    "approve",
		"调整后执行 | ok": "modify",
		"拒绝 | ok":    "reject",
		"unknown":    "modify",
	}
	for input, want := range tests {
		if got := parseFinalExecutionVerdict(input); got != want {
			t.Fatalf("parseFinalExecutionVerdict(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildExecutionResultSkipsRejectAndHold(t *testing.T) {
	a := &Agent{Enabled: true, DryRun: true, OrderType: "market", TimeInForce: "GTC", Leverage: 3, PricePeriod: "1m"}

	rejectState := &state.AgentState{
		Symbol:         "BTCUSDT",
		FinalDecision:  "拒绝 | risk too high",
		TraderDecision: state.TraderDecision{Direction: "buy", PositionSize: 0.2},
	}
	result, err := a.buildExecutionResult(context.Background(), rejectState)
	if err != nil {
		t.Fatalf("buildExecutionResult reject error: %v", err)
	}
	if result.Status != "skipped" {
		t.Fatalf("expected skipped status for reject, got %+v", result)
	}

	holdState := &state.AgentState{
		Symbol:         "BTCUSDT",
		FinalDecision:  "执行 | proceed",
		TraderDecision: state.TraderDecision{Direction: "hold", PositionSize: 0.2},
		PortfolioDecision: state.PortfolioDecision{
			Execution:            "执行",
			ApprovedPositionSize: 0.2,
		},
	}
	result, err = a.buildExecutionResult(context.Background(), holdState)
	if err != nil {
		t.Fatalf("buildExecutionResult hold error: %v", err)
	}
	if result.Status != "skipped" {
		t.Fatalf("expected skipped status for hold, got %+v", result)
	}
}

func TestNormalizedDefaults(t *testing.T) {
	a := (&Agent{}).normalized()
	if a.OrderType != "market" || a.TimeInForce != "GTC" || a.PricePeriod != "1m" || a.Leverage != 1 {
		t.Fatalf("unexpected normalized defaults: %+v", a)
	}
}

func TestBuildExecutionResultSkipsZeroApprovedPosition(t *testing.T) {
	a := &Agent{Enabled: true, DryRun: true, OrderType: "market", TimeInForce: "GTC", Leverage: 3, PricePeriod: "1m"}
	s := &state.AgentState{
		Symbol:         "BTCUSDT",
		FinalDecision:  "执行 | proceed",
		TraderDecision: state.TraderDecision{Direction: "buy", PositionSize: 0.4},
		PortfolioDecision: state.PortfolioDecision{
			Execution:            "执行",
			ApprovedPositionSize: 0,
		},
	}
	result, err := a.buildExecutionResult(context.Background(), s)
	if err != nil {
		t.Fatalf("buildExecutionResult error: %v", err)
	}
	if result.Status != "skipped" || result.Reason != "approved position size is zero" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestReductionPlanForExistingLongPosition(t *testing.T) {
	snapshot := &tools.AccountSnapshot{
		CashBalances: []tools.CashBalance{
			{Currency: "USDT", AvailableCash: 1000},
		},
		StockPositions: []tools.StockPosition{
			{Symbol: "BTCUSDT", Quantity: 0.05, PositionSide: "LONG"},
		},
	}

	qty, side, reduce, positionSide := reductionPlan("sell", snapshot.StockPositions[0], 0.3, 50000, snapshot)
	if !reduce {
		t.Fatal("expected reduction plan to be enabled")
	}
	if side != "sell" {
		t.Fatalf("expected sell side, got %s", side)
	}
	if positionSide != "LONG" {
		t.Fatalf("expected LONG position side, got %s", positionSide)
	}
	if qty <= 0 || qty > 0.05 {
		t.Fatalf("expected reduced quantity within current position size, got %f", qty)
	}
}

func TestApplyProtectivePricesForLongAndShort(t *testing.T) {
	cfg := Agent{
		ProtectiveOrdersEnabled: true,
		TakeProfitPercent:       0.015,
		StopLossPercent:         0.008,
	}

	longResult := &state.ExecutionResult{}
	applyProtectivePrices(longResult, cfg, "buy", 100)
	if !nearlyEqual(longResult.TakeProfitPrice, 101.5) {
		t.Fatalf("expected long take profit 101.5, got %f", longResult.TakeProfitPrice)
	}
	if !nearlyEqual(longResult.StopLossPrice, 99.2) {
		t.Fatalf("expected long stop loss 99.2, got %f", longResult.StopLossPrice)
	}

	shortResult := &state.ExecutionResult{}
	applyProtectivePrices(shortResult, cfg, "sell", 100)
	if !nearlyEqual(shortResult.TakeProfitPrice, 98.5) {
		t.Fatalf("expected short take profit 98.5, got %f", shortResult.TakeProfitPrice)
	}
	if !nearlyEqual(shortResult.StopLossPrice, 100.8) {
		t.Fatalf("expected short stop loss 100.8, got %f", shortResult.StopLossPrice)
	}
}

func nearlyEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
