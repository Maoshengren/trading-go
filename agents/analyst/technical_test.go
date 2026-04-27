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

	indicatorSeries, err := tools.BuildTechnicalIndicatorSeries(klines, technicalPayloadIndicatorSeries)
	if err != nil {
		t.Fatalf("BuildTechnicalIndicatorSeries error: %v", err)
	}
	timeframes := []technicalTimeframe{{
		Name:            "primary",
		Period:          "1d",
		BarCount:        len(klines),
		Snapshot:        snapshot,
		RecentBars:      buildRecentTechnicalBars(klines, technicalPayloadRecentBars),
		IndicatorSeries: buildTechnicalIndicatorPayloadSeries(indicatorSeries),
	}}

	raw, err := buildTechnicalPayload("aapl", timeframes)
	if err != nil {
		t.Fatalf("buildTechnicalPayload error: %v", err)
	}

	var got struct {
		Symbol     string `json:"symbol"`
		Timeframes []struct {
			Name     string `json:"name"`
			Period   string `json:"period"`
			BarCount int    `json:"bar_count"`
			Snapshot struct {
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
		} `json:"timeframes"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v; raw=%s", err, raw)
	}

	if got.Symbol != "AAPL" {
		t.Fatalf("expected symbol AAPL, got %q", got.Symbol)
	}
	if len(got.Timeframes) != 1 || got.Timeframes[0].Name != "primary" || got.Timeframes[0].BarCount != 80 {
		t.Fatalf("unexpected timeframes payload: %+v", got.Timeframes)
	}
	tf := got.Timeframes[0]
	if tf.Period != "1d" {
		t.Fatalf("expected primary period 1d, got %q", tf.Period)
	}
	if tf.Snapshot.Trend != "up" || tf.Snapshot.Momentum != "strong" || tf.Snapshot.Close != 125.5 {
		t.Fatalf("unexpected snapshot payload: %+v", tf.Snapshot)
	}
	if len(tf.RecentBars) != 80 {
		t.Fatalf("expected 80 recent bars, got %d", len(tf.RecentBars))
	}
	if tf.RecentBars[0].Time != "2026-04-01T00:00:00Z" {
		t.Fatalf("unexpected first recent bar time: %s", tf.RecentBars[0].Time)
	}
	if tf.RecentBars[len(tf.RecentBars)-1].Time != "2026-06-19T00:00:00Z" {
		t.Fatalf("unexpected last recent bar time: %s", tf.RecentBars[len(tf.RecentBars)-1].Time)
	}
	if len(tf.IndicatorSeries) != technicalPayloadIndicatorSeries {
		t.Fatalf("expected %d indicator points, got %d", technicalPayloadIndicatorSeries, len(tf.IndicatorSeries))
	}
	if tf.IndicatorSeries[0].Time != "2026-05-31T00:00:00Z" {
		t.Fatalf("unexpected first indicator point time: %s", tf.IndicatorSeries[0].Time)
	}
	if tf.IndicatorSeries[len(tf.IndicatorSeries)-1].Time != "2026-06-19T00:00:00Z" {
		t.Fatalf("unexpected last indicator point time: %s", tf.IndicatorSeries[len(tf.IndicatorSeries)-1].Time)
	}
}

func TestNormalizeKLineTimeframes(t *testing.T) {
	got := normalizeKLineTimeframes([]KLineTimeframeConfig{
		{Name: "execution", Period: "15m", Bars: 192, RecentBars: 96, IndicatorBars: 32},
		{Name: "trend", Period: "1h", Bars: 240, RecentBars: 999, IndicatorBars: 40},
		{Name: "duplicate", Period: "1h", Bars: 10},
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 unique timeframes, got %+v", got)
	}
	if got[0].Name != "execution" || got[0].Period != "15m" || got[0].Bars != 192 || got[0].RecentBars != 96 || got[0].IndicatorBars != 32 {
		t.Fatalf("unexpected first timeframe: %+v", got[0])
	}
	if got[1].Name != "trend" || got[1].Period != "1h" || got[1].RecentBars != 120 {
		t.Fatalf("unexpected second timeframe: %+v", got[1])
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
