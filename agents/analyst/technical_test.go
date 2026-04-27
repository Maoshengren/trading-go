package analyst

import (
	"encoding/json"
	"testing"
	"time"

	"trading-go/tools"
)

func TestBuildTechnicalPayloadIncludesSnapshotAndRecentBars(t *testing.T) {
	klines := sampleTechnicalPayloadKlines(80)
	snapshot := &tools.TechnicalSnapshot{
		Close:         125.5,
		Trend:         "up",
		Momentum:      "strong",
		Volatility:    "medium",
		PricePosition: "above_sma20_and_sma50",
	}

	raw, err := buildTechnicalPayload("aapl", "1d", klines, snapshot)
	if err != nil {
		t.Fatalf("buildTechnicalPayload error: %v", err)
	}

	var got struct {
		Symbol         string `json:"symbol"`
		AnalysisPeriod string `json:"analysis_period"`
		Snapshot       struct {
			Close    float64 `json:"close"`
			Trend    string  `json:"trend"`
			Momentum string  `json:"momentum"`
		} `json:"snapshot"`
		RecentBars []struct {
			Time  string  `json:"time"`
			Close float64 `json:"close"`
		} `json:"recent_bars"`
		IndicatorSeries []struct {
			Time     string  `json:"time"`
			MACDHist float64 `json:"macd_hist"`
			RSI14    float64 `json:"rsi14"`
		} `json:"indicator_series"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v; raw=%s", err, raw)
	}

	if got.Symbol != "AAPL" {
		t.Fatalf("expected symbol AAPL, got %q", got.Symbol)
	}
	if got.AnalysisPeriod != "1d" {
		t.Fatalf("expected analysis period 1d, got %q", got.AnalysisPeriod)
	}
	if got.Snapshot.Trend != "up" || got.Snapshot.Momentum != "strong" || got.Snapshot.Close != 125.5 {
		t.Fatalf("unexpected snapshot payload: %+v", got.Snapshot)
	}
	if len(got.RecentBars) != 80 {
		t.Fatalf("expected 80 recent bars, got %d", len(got.RecentBars))
	}
	if got.RecentBars[0].Time != "2026-04-01T00:00:00Z" {
		t.Fatalf("unexpected first recent bar time: %s", got.RecentBars[0].Time)
	}
	if got.RecentBars[len(got.RecentBars)-1].Time != "2026-06-19T00:00:00Z" {
		t.Fatalf("unexpected last recent bar time: %s", got.RecentBars[len(got.RecentBars)-1].Time)
	}
	if len(got.IndicatorSeries) != technicalPayloadIndicatorSeries {
		t.Fatalf("expected %d indicator points, got %d", technicalPayloadIndicatorSeries, len(got.IndicatorSeries))
	}
	if got.IndicatorSeries[0].Time != "2026-05-31T00:00:00Z" {
		t.Fatalf("unexpected first indicator point time: %s", got.IndicatorSeries[0].Time)
	}
	if got.IndicatorSeries[len(got.IndicatorSeries)-1].Time != "2026-06-19T00:00:00Z" {
		t.Fatalf("unexpected last indicator point time: %s", got.IndicatorSeries[len(got.IndicatorSeries)-1].Time)
	}
}

func TestBuildRecentTechnicalBarsReturnsAllWhenLimitExceedsLength(t *testing.T) {
	klines := sampleTechnicalPayloadKlines(3)
	bars := buildRecentTechnicalBars(klines, 10)
	if len(bars) != 3 {
		t.Fatalf("expected all bars to be returned, got %d", len(bars))
	}
	if bars[0].Time != "2026-04-01T00:00:00Z" {
		t.Fatalf("unexpected first bar time: %s", bars[0].Time)
	}
}

func sampleTechnicalPayloadKlines(count int) []tools.KLine {
	base := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	klines := make([]tools.KLine, 0, count)
	for i := range count {
		price := 100.0 + float64(i)
		klines = append(klines, tools.KLine{
			Time:   base.AddDate(0, 0, i),
			Open:   price,
			High:   price + 1,
			Low:    price - 1,
			Close:  price + 0.5,
			Volume: 1000 + float64(i)*10,
		})
	}
	return klines
}
