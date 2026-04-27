package technical

import (
	"math"
	"slices"
	"testing"
	"time"

	"trading-go/tools/core"
)

func TestBuildTimeSeriesSortsInput(t *testing.T) {
	t.Parallel()

	klines := sampleTrendKLines(80)
	reversed := slices.Clone(klines)
	slices.Reverse(reversed)

	rsi1, err := CalculateRSI(klines)
	if err != nil {
		t.Fatalf("CalculateRSI error: %v", err)
	}
	rsi2, err := CalculateRSI(reversed)
	if err != nil {
		t.Fatalf("CalculateRSI reversed error: %v", err)
	}

	idx := len(klines) - 1
	got1 := rsi1.Calculate(idx).Float()
	got2 := rsi2.Calculate(idx).Float()
	if math.Abs(got1-got2) > 1e-6 {
		t.Fatalf("expected same RSI for reversed input, got %f and %f", got1, got2)
	}
}

func TestInferPeriodDuration(t *testing.T) {
	t.Parallel()

	klines := sampleTrendKLines(10)
	if got := inferPeriodDuration(klines); got != 24*time.Hour {
		t.Fatalf("unexpected inferred duration: %s", got)
	}
}

func TestBuildTechnicalSnapshotTrendingUp(t *testing.T) {
	t.Parallel()

	snapshot, err := BuildTechnicalSnapshot(sampleTrendKLines(120))
	if err != nil {
		t.Fatalf("BuildTechnicalSnapshot error: %v", err)
	}

	if snapshot.Close <= 0 {
		t.Fatalf("expected positive close, got %f", snapshot.Close)
	}
	if snapshot.SMA20 <= snapshot.SMA50 {
		t.Fatalf("expected SMA20 > SMA50 in uptrend, got sma20=%f sma50=%f", snapshot.SMA20, snapshot.SMA50)
	}
	if snapshot.MACDHist <= 0 {
		t.Fatalf("expected positive MACD histogram in uptrend, got %f", snapshot.MACDHist)
	}
	if snapshot.ATR14 <= 0 {
		t.Fatalf("expected positive ATR, got %f", snapshot.ATR14)
	}
	if snapshot.VolumeSMA20 <= 0 {
		t.Fatalf("expected positive volume SMA20, got %f", snapshot.VolumeSMA20)
	}
	if snapshot.OBV <= 0 {
		t.Fatalf("expected positive OBV in uptrend, got %f", snapshot.OBV)
	}
	if snapshot.Trend != "up" {
		t.Fatalf("expected trend=up, got %s", snapshot.Trend)
	}
	if snapshot.PricePosition != "above_sma20_and_sma50" {
		t.Fatalf("expected price above moving averages, got %s", snapshot.PricePosition)
	}
	if snapshot.RSIRegime == "" || snapshot.MACDCross == "" || snapshot.VolumeConfirmation == "" {
		t.Fatal("expected snapshot classifications to be populated")
	}
}

func TestBuildTechnicalSnapshotInvalidKLine(t *testing.T) {
	t.Parallel()

	klines := sampleTrendKLines(60)
	klines[10].High = klines[10].Low - 1

	if _, err := BuildTechnicalSnapshot(klines); err == nil {
		t.Fatal("expected invalid kline error")
	}
}

func TestBuildTechnicalIndicatorSeries(t *testing.T) {
	t.Parallel()

	points, err := BuildTechnicalIndicatorSeries(sampleTrendKLines(120), 20)
	if err != nil {
		t.Fatalf("BuildTechnicalIndicatorSeries error: %v", err)
	}
	if len(points) != 20 {
		t.Fatalf("expected 20 indicator points, got %d", len(points))
	}
	first := points[0]
	last := points[len(points)-1]
	if !first.Time.Before(last.Time) {
		t.Fatalf("expected ascending indicator series timestamps, got first=%s last=%s", first.Time, last.Time)
	}
	if first.Close <= 0 || last.Close <= 0 {
		t.Fatalf("expected positive close prices, got first=%f last=%f", first.Close, last.Close)
	}
	if first.RSI14 < 0 || first.RSI14 > 100 || last.RSI14 < 0 || last.RSI14 > 100 {
		t.Fatalf("expected RSI values in range, got first=%f last=%f", first.RSI14, last.RSI14)
	}
	if first.StochK < 0 || first.StochK > 100 || last.StochD < 0 || last.StochD > 100 {
		t.Fatalf("expected stochastic values in range, got firstK=%f lastD=%f", first.StochK, last.StochD)
	}
}

func sampleTrendKLines(count int) []core.KLine {
	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	klines := make([]core.KLine, 0, count)
	closePrice := 100.0
	for i := range count {
		trend := 0.6 + float64(i%5)*0.05
		open := closePrice
		closePrice = open + trend
		high := closePrice + 1.2
		low := open - 1.0
		volume := 1000.0 + float64(i)*12 + float64(i%7)*20
		klines = append(klines, core.KLine{
			Time:   baseTime.Add(time.Duration(i) * 24 * time.Hour),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePrice,
			Volume: volume,
		})
	}
	return klines
}
