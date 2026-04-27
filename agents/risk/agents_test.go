package risk

import (
	"testing"
	"time"

	"trading-go/tools"
	technicaltools "trading-go/tools/technical"
)

func TestBuildRiskPositionSnapshotMatchesSymbolWithoutSuffix(t *testing.T) {
	snapshot := &tools.AccountSnapshot{
		AccountMode: "paper",
		CashBalances: []tools.CashBalance{
			{Currency: "USD", AvailableCash: 1000, TotalCash: 1200, NetAssets: 5000, RiskLevel: "low"},
		},
		StockPositions: []tools.StockPosition{
			{Symbol: "AAPL.US", Quantity: 12, AvailableQuantity: 10, CostPrice: 180.5, Currency: "USD", Market: "US"},
		},
	}

	got := buildRiskPositionSnapshot("AAPL", snapshot)
	if got.AccountMode != "paper" {
		t.Fatalf("expected account mode paper, got %s", got.AccountMode)
	}
	if len(got.CashBalances) != 1 || len(got.StockPositions) != 1 {
		t.Fatalf("unexpected snapshot sizes: %+v", got)
	}
	if got.MatchingPosition == nil {
		t.Fatal("expected matching position to be detected")
	}
	if got.MatchingPosition.Symbol != "AAPL.US" || got.MatchingPosition.Quantity != 12 {
		t.Fatalf("unexpected matching position: %+v", got.MatchingPosition)
	}
}

func TestBuildRiskMarketData(t *testing.T) {
	klines := []tools.KLine{
		{Time: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC), Close: 100, High: 101, Low: 99, Volume: 1000},
		{Time: time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC), Close: 102, High: 103, Low: 100, Volume: 2000},
	}
	snapshot := &technicaltools.TechnicalSnapshot{Trend: "up", ATRPct: 0.02}

	got := buildRiskMarketData(klines, snapshot)
	if got.Snapshot != snapshot {
		t.Fatal("expected snapshot to be preserved")
	}
	if got.CurrentPrice != 102 || got.RecentHigh != 103 || got.RecentLow != 99 {
		t.Fatalf("unexpected price summary: %+v", got)
	}
	if got.AverageVolume != 1500 {
		t.Fatalf("expected average volume 1500, got %f", got.AverageVolume)
	}
	if len(got.RecentBars) != 2 {
		t.Fatalf("expected 2 bars, got %d", len(got.RecentBars))
	}
}

func TestSymbolMatchesPosition(t *testing.T) {
	tests := []struct {
		requested string
		actual    string
		want      bool
	}{
		{"AAPL", "AAPL.US", true},
		{"AAPL.US", "AAPL", true},
		{"700", "700.HK", true},
		{"TSLA", "AAPL.US", false},
	}
	for _, tt := range tests {
		if got := symbolMatchesPosition(tt.requested, tt.actual); got != tt.want {
			t.Fatalf("symbolMatchesPosition(%q, %q) = %v, want %v", tt.requested, tt.actual, got, tt.want)
		}
	}
}
