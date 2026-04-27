package trader

import (
	"encoding/json"
	"testing"
	"time"

	"trading-go/state"
	"trading-go/tools"
)

func TestBuildTraderPayloadIncludesResearchSummaryAndExecutionHistory(t *testing.T) {
	s := &state.AgentState{
		Symbol:            "AAPL",
		FundamentalReport: "fundamental report",
		SentimentReport:   "sentiment report",
		NewsReport:        "news report",
		TechnicalReport:   "technical report",
		ResearchSummary:   "bull beats bear because momentum and earnings align",
	}
	records := []tools.ExecutionRecord{
		{
			OrderID:     "order-1",
			TradeID:     "trade-1",
			Symbol:      "AAPL.US",
			TradeDoneAt: time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC),
			Quantity:    10,
			Price:       188.5,
		},
	}

	raw, err := buildTraderPayload(s, records)
	if err != nil {
		t.Fatalf("buildTraderPayload error: %v", err)
	}

	var got struct {
		Symbol           string `json:"symbol"`
		ResearchSummary  string `json:"research_summary"`
		ExecutionHistory []struct {
			OrderID     string  `json:"order_id"`
			TradeID     string  `json:"trade_id"`
			Symbol      string  `json:"symbol"`
			TradeDoneAt string  `json:"trade_done_at"`
			Quantity    float64 `json:"quantity"`
			Price       float64 `json:"price"`
		} `json:"execution_history"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v; raw=%s", err, raw)
	}

	if got.Symbol != "AAPL" {
		t.Fatalf("expected symbol AAPL, got %q", got.Symbol)
	}
	if got.ResearchSummary != s.ResearchSummary {
		t.Fatalf("expected research summary %q, got %q", s.ResearchSummary, got.ResearchSummary)
	}
	if len(got.ExecutionHistory) != 1 {
		t.Fatalf("expected 1 execution record, got %d", len(got.ExecutionHistory))
	}
	if got.ExecutionHistory[0].OrderID != "order-1" || got.ExecutionHistory[0].Price != 188.5 {
		t.Fatalf("unexpected execution history payload: %+v", got.ExecutionHistory[0])
	}
	if got.ExecutionHistory[0].TradeDoneAt != "2026-04-24T10:00:00Z" {
		t.Fatalf("unexpected execution time: %s", got.ExecutionHistory[0].TradeDoneAt)
	}
}

func TestBuildTraderExecutionHistoryPreservesOrder(t *testing.T) {
	records := []tools.ExecutionRecord{
		{OrderID: "1", TradeDoneAt: time.Date(2026, 4, 24, 9, 0, 0, 0, time.UTC)},
		{OrderID: "2", TradeDoneAt: time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)},
	}

	got := buildTraderExecutionHistory(records)
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	if got[0].OrderID != "1" || got[1].OrderID != "2" {
		t.Fatalf("unexpected execution history order: %+v", got)
	}
}
