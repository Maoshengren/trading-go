package tools

import (
	"fmt"
	"time"

	"github.com/sdcoffey/techan"
)

type FinancialReport struct {
	Revenue       float64
	NetProfit     float64
	CashFlow      float64
	DebtToEquity  float64
	ReportQuarter string
}

type NewsItem struct {
	Title     string
	Published time.Time
	Impact    string
}

type KLine struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

func GetFinancialReports(symbol string) (FinancialReport, error) {
	return FinancialReport{
		Revenue:       120_000_000,
		NetProfit:     16_500_000,
		CashFlow:      20_100_000,
		DebtToEquity:  0.65,
		ReportQuarter: "2026Q1",
	}, nil
}

func GetSocialSentiment(symbol string) (float64, error) {
	return 0.62, nil
}

func GetNews(symbol string, days int) ([]NewsItem, error) {
	now := time.Now()
	return []NewsItem{
		{Title: fmt.Sprintf("%s launches new product line", symbol), Published: now.Add(-24 * time.Hour), Impact: "positive"},
		{Title: fmt.Sprintf("%s faces margin pressure concerns", symbol), Published: now.Add(-72 * time.Hour), Impact: "negative"},
	}, nil
}

func GetKline(symbol, period string) ([]KLine, error) {
	now := time.Now()
	return []KLine{
		{Time: now.Add(-3 * time.Hour), Open: 100, High: 102, Low: 99, Close: 101, Volume: 10500},
		{Time: now.Add(-2 * time.Hour), Open: 101, High: 103, Low: 100, Close: 102, Volume: 9800},
		{Time: now.Add(-1 * time.Hour), Open: 102, High: 104, Low: 101, Close: 103, Volume: 11200},
	}, nil
}

func CalculateRSI(_ []KLine) (*techan.Indicator, error) {
	return nil, nil
}

func CalculateMACD(_ []KLine) (*techan.Indicator, error) {
	return nil, nil
}

func CalculateBollingerBands(_ []KLine) (upper, middle, lower *techan.Indicator, err error) {
	return nil, nil, nil, nil
}
