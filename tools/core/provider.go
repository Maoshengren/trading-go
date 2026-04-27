package core

type MarketDataProvider interface {
	GetFinancialReports(symbol string) (FinancialReport, error)
	GetSocialSentiment(symbol string) (float64, error)
	GetNews(symbol string, days int) ([]NewsItem, error)
	GetKline(symbol, period string) ([]KLine, error)
}

type AccountProvider interface {
	GetAccountSnapshot(symbols []string) (*AccountSnapshot, error)
}

type TradeProvider interface {
	GetHistoryExecutions(query ExecutionHistoryQuery) ([]ExecutionRecord, error)
	GetTodayExecutions(query TodayExecutionQuery) ([]ExecutionRecord, error)
	SubmitOrder(req OrderRequest) (*OrderSubmission, error)
	CancelOrder(req CancelOrderRequest) error
	SetLeverage(req LeverageRequest) (*LeverageUpdate, error)
}
