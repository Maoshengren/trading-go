package technical

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/sdcoffey/big"
	"github.com/sdcoffey/techan"

	"trading-go/tools/core"
)

type MACDIndicators struct {
	MACD   techan.Indicator
	Signal techan.Indicator
	Hist   techan.Indicator
}

type StochasticIndicators struct {
	K techan.Indicator
	D techan.Indicator
}

type TechnicalSnapshot struct {
	Close              float64
	RSI14              float64
	MACD               float64
	MACDSignal         float64
	MACDHist           float64
	BollingerUpper     float64
	BollingerMiddle    float64
	BollingerLower     float64
	SMA20              float64
	SMA50              float64
	EMA12              float64
	EMA26              float64
	ATR14              float64
	ATRPct             float64
	StochK             float64
	StochD             float64
	OBV                float64
	Volume             float64
	VolumeSMA20        float64
	VolumeRatio        float64
	Return1D           float64
	Return5D           float64
	Return20D          float64
	BollingerBandwidth float64
	BollingerPercentB  float64
	Support20          float64
	Resistance20       float64
	DistanceTo20DHigh  float64
	DistanceTo20DLow   float64
	Breakout20D        bool
	Breakdown20D       bool
	Trend              string
	Momentum           string
	Volatility         string
	PricePosition      string
	BandPosition       string
	VolumeConfirmation string
	MACDCross          string
	RSIRegime          string
}

type TechnicalIndicatorPoint struct {
	Time        time.Time `json:"time"`
	Close       float64   `json:"close"`
	RSI14       float64   `json:"rsi14"`
	MACD        float64   `json:"macd"`
	MACDSignal  float64   `json:"macd_signal"`
	MACDHist    float64   `json:"macd_hist"`
	SMA20       float64   `json:"sma20"`
	SMA50       float64   `json:"sma50"`
	ATRPct      float64   `json:"atr_pct"`
	PercentB    float64   `json:"bb_percent_b"`
	VolumeRatio float64   `json:"volume_ratio"`
	StochK      float64   `json:"stoch_k"`
	StochD      float64   `json:"stoch_d"`
}

func CalculateRSI(klines []core.KLine) (techan.Indicator, error) {
	if len(klines) < 15 {
		return nil, core.ErrInsufficientBars
	}
	series, err := buildTimeSeries(klines)
	if err != nil {
		return nil, err
	}
	closeIndicator := techan.NewClosePriceIndicator(series)
	return techan.NewRelativeStrengthIndexIndicator(closeIndicator, 14), nil
}

func CalculateMACD(klines []core.KLine) (*MACDIndicators, error) {
	if len(klines) < 35 {
		return nil, core.ErrInsufficientBars
	}
	series, err := buildTimeSeries(klines)
	if err != nil {
		return nil, err
	}

	closeIndicator := techan.NewClosePriceIndicator(series)
	macd := techan.NewMACDIndicator(closeIndicator, 12, 26)
	signal := techan.NewEMAIndicator(macd, 9)
	hist := techan.NewDifferenceIndicator(macd, signal)
	return &MACDIndicators{MACD: macd, Signal: signal, Hist: hist}, nil
}

func CalculateBollingerBands(klines []core.KLine) (upper, middle, lower techan.Indicator, err error) {
	if len(klines) < 20 {
		return nil, nil, nil, core.ErrInsufficientBars
	}
	series, err := buildTimeSeries(klines)
	if err != nil {
		return nil, nil, nil, err
	}
	closeIndicator := techan.NewClosePriceIndicator(series)
	middle = techan.NewSimpleMovingAverage(closeIndicator, 20)
	upper = techan.NewBollingerUpperBandIndicator(closeIndicator, 20, 2)
	lower = techan.NewBollingerLowerBandIndicator(closeIndicator, 20, 2)
	return upper, middle, lower, nil
}

func CalculateSMA(klines []core.KLine, window int) (techan.Indicator, error) {
	if len(klines) < window {
		return nil, core.ErrInsufficientBars
	}
	series, err := buildTimeSeries(klines)
	if err != nil {
		return nil, err
	}
	return techan.NewSimpleMovingAverage(techan.NewClosePriceIndicator(series), window), nil
}

func CalculateEMA(klines []core.KLine, window int) (techan.Indicator, error) {
	if len(klines) < window {
		return nil, core.ErrInsufficientBars
	}
	series, err := buildTimeSeries(klines)
	if err != nil {
		return nil, err
	}
	return techan.NewEMAIndicator(techan.NewClosePriceIndicator(series), window), nil
}

func CalculateATR(klines []core.KLine, window int) (techan.Indicator, error) {
	if len(klines) <= window {
		return nil, core.ErrInsufficientBars
	}
	series, err := buildTimeSeries(klines)
	if err != nil {
		return nil, err
	}
	return techan.NewAverageTrueRangeIndicator(series, window), nil
}

func CalculateStochastic(klines []core.KLine, kWindow, dWindow int) (*StochasticIndicators, error) {
	if len(klines) < kWindow+dWindow-1 {
		return nil, core.ErrInsufficientBars
	}
	series, err := buildTimeSeries(klines)
	if err != nil {
		return nil, err
	}
	k := techan.NewFastStochasticIndicator(series, kWindow)
	d := techan.NewSlowStochasticIndicator(k, dWindow)
	return &StochasticIndicators{K: k, D: d}, nil
}

func CalculateOBV(klines []core.KLine) (float64, error) {
	sorted, err := normalizeKLines(klines)
	if err != nil {
		return 0, err
	}
	if len(sorted) < 2 {
		return 0, core.ErrInsufficientBars
	}

	var obv float64
	for i := 1; i < len(sorted); i++ {
		switch {
		case sorted[i].Close > sorted[i-1].Close:
			obv += sorted[i].Volume
		case sorted[i].Close < sorted[i-1].Close:
			obv -= sorted[i].Volume
		}
	}
	return obv, nil
}

func BuildTechnicalSnapshot(klines []core.KLine) (*TechnicalSnapshot, error) {
	sorted, err := normalizeKLines(klines)
	if err != nil {
		return nil, err
	}
	if len(sorted) < 50 {
		return nil, core.ErrInsufficientBars
	}

	idx := len(sorted) - 1
	rsi, err := CalculateRSI(sorted)
	if err != nil {
		return nil, err
	}
	macd, err := CalculateMACD(sorted)
	if err != nil {
		return nil, err
	}
	upper, middle, lower, err := CalculateBollingerBands(sorted)
	if err != nil {
		return nil, err
	}
	sma20, err := CalculateSMA(sorted, 20)
	if err != nil {
		return nil, err
	}
	sma50, err := CalculateSMA(sorted, 50)
	if err != nil {
		return nil, err
	}
	ema12, err := CalculateEMA(sorted, 12)
	if err != nil {
		return nil, err
	}
	ema26, err := CalculateEMA(sorted, 26)
	if err != nil {
		return nil, err
	}
	atr14, err := CalculateATR(sorted, 14)
	if err != nil {
		return nil, err
	}
	stochastic, err := CalculateStochastic(sorted, 14, 3)
	if err != nil {
		return nil, err
	}
	obv, err := CalculateOBV(sorted)
	if err != nil {
		return nil, err
	}

	series, err := buildTimeSeries(sorted)
	if err != nil {
		return nil, err
	}
	volumeIndicator := techan.NewVolumeIndicator(series)
	volumeSMA20 := techan.NewSimpleMovingAverage(volumeIndicator, 20)

	closePrice := sorted[idx].Close
	upperVal := upper.Calculate(idx).Float()
	middleVal := middle.Calculate(idx).Float()
	lowerVal := lower.Calculate(idx).Float()
	sma20Val := sma20.Calculate(idx).Float()
	sma50Val := sma50.Calculate(idx).Float()
	ema12Val := ema12.Calculate(idx).Float()
	ema26Val := ema26.Calculate(idx).Float()
	atr14Val := atr14.Calculate(idx).Float()
	volumeVal := sorted[idx].Volume
	volumeSMA20Val := volumeSMA20.Calculate(idx).Float()

	prevMACDHist := macd.Hist.Calculate(idx - 1).Float()
	macdHistVal := macd.Hist.Calculate(idx).Float()
	stochKVal := clampFloat(stochastic.K.Calculate(idx).Float(), 0, 100)
	stochDVal := clampFloat(stochastic.D.Calculate(idx).Float(), 0, 100)
	bandwidth := safeRatio(upperVal-lowerVal, middleVal)
	percentB := safeRatio(closePrice-lowerVal, upperVal-lowerVal)
	resistance20, support20 := rollingHighLow(sorted, 20)
	prevResistance20, prevSupport20 := rollingHighLow(sorted[:idx], 20)

	snapshot := &TechnicalSnapshot{
		Close:              closePrice,
		RSI14:              clampFloat(rsi.Calculate(idx).Float(), 0, 100),
		MACD:               macd.MACD.Calculate(idx).Float(),
		MACDSignal:         macd.Signal.Calculate(idx).Float(),
		MACDHist:           macdHistVal,
		BollingerUpper:     upperVal,
		BollingerMiddle:    middleVal,
		BollingerLower:     lowerVal,
		SMA20:              sma20Val,
		SMA50:              sma50Val,
		EMA12:              ema12Val,
		EMA26:              ema26Val,
		ATR14:              atr14Val,
		ATRPct:             safeRatio(atr14Val, closePrice),
		StochK:             stochKVal,
		StochD:             stochDVal,
		OBV:                obv,
		Volume:             volumeVal,
		VolumeSMA20:        volumeSMA20Val,
		VolumeRatio:        safeRatio(volumeVal, volumeSMA20Val),
		Return1D:           windowReturn(sorted, 1),
		Return5D:           windowReturn(sorted, 5),
		Return20D:          windowReturn(sorted, 20),
		BollingerBandwidth: bandwidth,
		BollingerPercentB:  percentB,
		Support20:          support20,
		Resistance20:       resistance20,
		DistanceTo20DHigh:  safeRatio(closePrice-prevResistance20, prevResistance20),
		DistanceTo20DLow:   safeRatio(closePrice-prevSupport20, prevSupport20),
		Breakout20D:        closePrice > prevResistance20 && prevResistance20 > 0,
		Breakdown20D:       closePrice < prevSupport20 && prevSupport20 > 0,
		MACDCross:          classifyMACDCross(prevMACDHist, macdHistVal),
		RSIRegime:          classifyRSIRegime(clampFloat(rsi.Calculate(idx).Float(), 0, 100)),
	}

	snapshot.Trend = classifyTrend(closePrice, sma20Val, sma50Val, macdHistVal)
	snapshot.Momentum = classifyMomentum(snapshot.RSI14, macdHistVal, stochKVal, stochDVal)
	snapshot.Volatility = classifyVolatility(snapshot.ATRPct, bandwidth)
	snapshot.PricePosition = classifyPricePosition(closePrice, sma20Val, sma50Val)
	snapshot.BandPosition = classifyBandPosition(percentB)
	snapshot.VolumeConfirmation = classifyVolumeConfirmation(snapshot.VolumeRatio)

	return snapshot, nil
}

func BuildTechnicalIndicatorSeries(klines []core.KLine, lookback int) ([]TechnicalIndicatorPoint, error) {
	sorted, err := normalizeKLines(klines)
	if err != nil {
		return nil, err
	}
	if len(sorted) < 50 {
		return nil, core.ErrInsufficientBars
	}
	if lookback <= 0 {
		return nil, nil
	}

	rsi, err := CalculateRSI(sorted)
	if err != nil {
		return nil, err
	}
	macd, err := CalculateMACD(sorted)
	if err != nil {
		return nil, err
	}
	upper, _, lower, err := CalculateBollingerBands(sorted)
	if err != nil {
		return nil, err
	}
	sma20, err := CalculateSMA(sorted, 20)
	if err != nil {
		return nil, err
	}
	sma50, err := CalculateSMA(sorted, 50)
	if err != nil {
		return nil, err
	}
	atr14, err := CalculateATR(sorted, 14)
	if err != nil {
		return nil, err
	}
	stochastic, err := CalculateStochastic(sorted, 14, 3)
	if err != nil {
		return nil, err
	}

	series, err := buildTimeSeries(sorted)
	if err != nil {
		return nil, err
	}
	volumeIndicator := techan.NewVolumeIndicator(series)
	volumeSMA20 := techan.NewSimpleMovingAverage(volumeIndicator, 20)

	start := len(sorted) - lookback
	if start < 49 {
		start = 49
	}
	points := make([]TechnicalIndicatorPoint, 0, len(sorted)-start)
	for i := start; i < len(sorted); i++ {
		closePrice := sorted[i].Close
		upperVal := upper.Calculate(i).Float()
		lowerVal := lower.Calculate(i).Float()
		volumeVal := sorted[i].Volume
		volumeSMA20Val := volumeSMA20.Calculate(i).Float()
		atr14Val := atr14.Calculate(i).Float()

		points = append(points, TechnicalIndicatorPoint{
			Time:        sorted[i].Time,
			Close:       closePrice,
			RSI14:       clampFloat(rsi.Calculate(i).Float(), 0, 100),
			MACD:        macd.MACD.Calculate(i).Float(),
			MACDSignal:  macd.Signal.Calculate(i).Float(),
			MACDHist:    macd.Hist.Calculate(i).Float(),
			SMA20:       sma20.Calculate(i).Float(),
			SMA50:       sma50.Calculate(i).Float(),
			ATRPct:      safeRatio(atr14Val, closePrice),
			PercentB:    safeRatio(closePrice-lowerVal, upperVal-lowerVal),
			VolumeRatio: safeRatio(volumeVal, volumeSMA20Val),
			StochK:      clampFloat(stochastic.K.Calculate(i).Float(), 0, 100),
			StochD:      clampFloat(stochastic.D.Calculate(i).Float(), 0, 100),
		})
	}
	return points, nil
}

func buildTimeSeries(klines []core.KLine) (*techan.TimeSeries, error) {
	sorted, err := normalizeKLines(klines)
	if err != nil {
		return nil, err
	}
	series := techan.NewTimeSeries()
	periodDuration := inferPeriodDuration(sorted)
	for _, k := range sorted {
		period := techan.NewTimePeriod(k.Time, periodDuration)
		candle := techan.NewCandle(period)
		candle.OpenPrice = big.NewDecimal(k.Open)
		candle.ClosePrice = big.NewDecimal(k.Close)
		candle.MaxPrice = big.NewDecimal(k.High)
		candle.MinPrice = big.NewDecimal(k.Low)
		candle.Volume = big.NewDecimal(k.Volume)
		series.AddCandle(candle)
	}
	return series, nil
}

func normalizeKLines(klines []core.KLine) ([]core.KLine, error) {
	if len(klines) == 0 {
		return nil, core.ErrInsufficientBars
	}
	normalized := append([]core.KLine(nil), klines...)
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Time.Before(normalized[j].Time)
	})
	for i, k := range normalized {
		if k.Time.IsZero() {
			return nil, fmt.Errorf("invalid kline at index %d: missing timestamp", i)
		}
		if k.High < k.Low {
			return nil, fmt.Errorf("invalid kline at index %d: high < low", i)
		}
		if k.Open < 0 || k.Close < 0 || k.Volume < 0 {
			return nil, fmt.Errorf("invalid kline at index %d: negative value", i)
		}
	}
	return normalized, nil
}

func inferPeriodDuration(klines []core.KLine) time.Duration {
	if len(klines) < 2 {
		return time.Minute
	}
	minPositive := time.Duration(0)
	for i := 1; i < len(klines); i++ {
		diff := klines[i].Time.Sub(klines[i-1].Time)
		if diff <= 0 {
			continue
		}
		if minPositive == 0 || diff < minPositive {
			minPositive = diff
		}
	}
	if minPositive == 0 {
		return time.Minute
	}
	return minPositive
}

func windowReturn(klines []core.KLine, lookback int) float64 {
	if len(klines) <= lookback {
		return 0
	}
	prev := klines[len(klines)-1-lookback].Close
	curr := klines[len(klines)-1].Close
	return safeRatio(curr-prev, prev)
}

func rollingHighLow(klines []core.KLine, window int) (high float64, low float64) {
	if len(klines) == 0 {
		return 0, 0
	}
	start := len(klines) - window
	if start < 0 {
		start = 0
	}
	high = klines[start].High
	low = klines[start].Low
	for _, k := range klines[start:] {
		if k.High > high {
			high = k.High
		}
		if k.Low < low {
			low = k.Low
		}
	}
	return high, low
}

func safeRatio(numerator, denominator float64) float64 {
	if denominator == 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		return 0
	}
	v := numerator / denominator
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}

func clampFloat(v, minV, maxV float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if math.IsInf(v, 1) {
		return maxV
	}
	if math.IsInf(v, -1) {
		return minV
	}
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func classifyTrend(closePrice, sma20, sma50, macdHist float64) string {
	switch {
	case closePrice > sma20 && sma20 > sma50 && macdHist > 0:
		return "up"
	case closePrice < sma20 && sma20 < sma50 && macdHist < 0:
		return "down"
	default:
		return "sideways"
	}
}

func classifyMomentum(rsi, macdHist, stochK, stochD float64) string {
	switch {
	case macdHist > 0 && rsi >= 60 && stochK >= stochD:
		return "strong"
	case macdHist < 0 && rsi <= 40 && stochK <= stochD:
		return "weak"
	default:
		return "mixed"
	}
}

func classifyVolatility(atrPct, bandwidth float64) string {
	switch {
	case atrPct >= 0.04 || bandwidth >= 0.18:
		return "high"
	case atrPct <= 0.015 && bandwidth <= 0.08:
		return "low"
	default:
		return "medium"
	}
}

func classifyPricePosition(closePrice, sma20, sma50 float64) string {
	switch {
	case closePrice > sma20 && closePrice > sma50:
		return "above_sma20_and_sma50"
	case closePrice < sma20 && closePrice < sma50:
		return "below_sma20_and_sma50"
	case closePrice > sma20:
		return "above_sma20_below_sma50"
	case closePrice > sma50:
		return "below_sma20_above_sma50"
	default:
		return "between_sma20_and_sma50"
	}
}

func classifyBandPosition(percentB float64) string {
	switch {
	case percentB >= 0.8:
		return "near_upper"
	case percentB <= 0.2:
		return "near_lower"
	default:
		return "middle"
	}
}

func classifyVolumeConfirmation(volumeRatio float64) string {
	switch {
	case volumeRatio >= 1.2:
		return "yes"
	case volumeRatio <= 0.8:
		return "no"
	default:
		return "neutral"
	}
}

func classifyMACDCross(prevHist, currHist float64) string {
	switch {
	case prevHist <= 0 && currHist > 0:
		return "golden_cross"
	case prevHist >= 0 && currHist < 0:
		return "death_cross"
	default:
		return "none"
	}
}

func classifyRSIRegime(rsi float64) string {
	switch {
	case rsi >= 70:
		return "overbought"
	case rsi <= 30:
		return "oversold"
	default:
		return "neutral"
	}
}
