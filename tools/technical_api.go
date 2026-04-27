package tools

import (
	"github.com/sdcoffey/techan"

	"trading-go/tools/technical"
)

type MACDIndicators = technical.MACDIndicators
type StochasticIndicators = technical.StochasticIndicators
type TechnicalSnapshot = technical.TechnicalSnapshot
type TechnicalIndicatorPoint = technical.TechnicalIndicatorPoint

func CalculateRSI(klines []KLine) (techan.Indicator, error) {
	return technical.CalculateRSI(klines)
}

func CalculateMACD(klines []KLine) (*MACDIndicators, error) {
	return technical.CalculateMACD(klines)
}

func CalculateBollingerBands(klines []KLine) (upper, middle, lower techan.Indicator, err error) {
	return technical.CalculateBollingerBands(klines)
}

func CalculateSMA(klines []KLine, window int) (techan.Indicator, error) {
	return technical.CalculateSMA(klines, window)
}

func CalculateEMA(klines []KLine, window int) (techan.Indicator, error) {
	return technical.CalculateEMA(klines, window)
}

func CalculateATR(klines []KLine, window int) (techan.Indicator, error) {
	return technical.CalculateATR(klines, window)
}

func CalculateStochastic(klines []KLine, kWindow, dWindow int) (*StochasticIndicators, error) {
	return technical.CalculateStochastic(klines, kWindow, dWindow)
}

func CalculateOBV(klines []KLine) (float64, error) {
	return technical.CalculateOBV(klines)
}

func BuildTechnicalSnapshot(klines []KLine) (*TechnicalSnapshot, error) {
	return technical.BuildTechnicalSnapshot(klines)
}

func BuildTechnicalIndicatorSeries(klines []KLine, lookback int) ([]TechnicalIndicatorPoint, error) {
	return technical.BuildTechnicalIndicatorSeries(klines, lookback)
}
