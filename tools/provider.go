package tools

import "trading-go/tools/core"

type MarketDataProvider = core.MarketDataProvider
type AccountProvider = core.AccountProvider
type TradeProvider = core.TradeProvider

type unavailableProvider struct{}

func (p unavailableProvider) GetFinancialReports(symbol string) (FinancialReport, error) {
	return FinancialReport{}, core.ErrProviderNotConfigured
}

func (p unavailableProvider) GetSocialSentiment(symbol string) (float64, error) {
	return 0, core.ErrProviderNotConfigured
}

func (p unavailableProvider) GetNews(symbol string, days int) ([]NewsItem, error) {
	return nil, core.ErrProviderNotConfigured
}

func (p unavailableProvider) GetKline(symbol, period string) ([]KLine, error) {
	return nil, core.ErrProviderNotConfigured
}

type unavailableAccountProvider struct{}
type unavailableTradeProvider struct{}

func (p unavailableAccountProvider) GetAccountSnapshot(symbols []string) (*AccountSnapshot, error) {
	return nil, core.ErrProviderNotConfigured
}

func (p unavailableTradeProvider) GetHistoryExecutions(query ExecutionHistoryQuery) ([]ExecutionRecord, error) {
	return nil, core.ErrProviderNotConfigured
}

func (p unavailableTradeProvider) GetTodayExecutions(query TodayExecutionQuery) ([]ExecutionRecord, error) {
	return nil, core.ErrProviderNotConfigured
}

func (p unavailableTradeProvider) SubmitOrder(req OrderRequest) (*OrderSubmission, error) {
	return nil, core.ErrProviderNotConfigured
}

func (p unavailableTradeProvider) CancelOrder(req CancelOrderRequest) error {
	return core.ErrProviderNotConfigured
}

func (p unavailableTradeProvider) SetLeverage(req LeverageRequest) (*LeverageUpdate, error) {
	return nil, core.ErrProviderNotConfigured
}

var defaultProvider MarketDataProvider = unavailableProvider{}
var defaultAccountProvider AccountProvider = unavailableAccountProvider{}
var defaultTradeProvider TradeProvider = unavailableTradeProvider{}

func SetDefaultProvider(provider MarketDataProvider) {
	if provider == nil {
		return
	}
	defaultProvider = provider
}

func SetDefaultAccountProvider(provider AccountProvider) {
	if provider == nil {
		return
	}
	defaultAccountProvider = provider
}

func SetDefaultTradeProvider(provider TradeProvider) {
	if provider == nil {
		return
	}
	defaultTradeProvider = provider
}

func CloseDefaultProvider() error {
	if provider, ok := defaultProvider.(interface{ Close() error }); ok {
		return provider.Close()
	}
	if provider, ok := defaultProvider.(interface{ Close() }); ok {
		provider.Close()
	}
	return nil
}

func GetFinancialReports(symbol string) (FinancialReport, error) {
	return defaultProvider.GetFinancialReports(symbol)
}

func GetSocialSentiment(symbol string) (float64, error) {
	return defaultProvider.GetSocialSentiment(symbol)
}

func GetNews(symbol string, days int) ([]NewsItem, error) {
	return defaultProvider.GetNews(symbol, days)
}

func GetKline(symbol, period string) ([]KLine, error) {
	return defaultProvider.GetKline(symbol, period)
}

func GetAccountSnapshot(symbols []string) (*AccountSnapshot, error) {
	return defaultAccountProvider.GetAccountSnapshot(symbols)
}

func GetHistoryExecutions(query ExecutionHistoryQuery) ([]ExecutionRecord, error) {
	return defaultTradeProvider.GetHistoryExecutions(query)
}

func GetTodayExecutions(query TodayExecutionQuery) ([]ExecutionRecord, error) {
	return defaultTradeProvider.GetTodayExecutions(query)
}

func SubmitOrder(req OrderRequest) (*OrderSubmission, error) {
	return defaultTradeProvider.SubmitOrder(req)
}

func CancelOrder(req CancelOrderRequest) error {
	return defaultTradeProvider.CancelOrder(req)
}

func SetLeverage(req LeverageRequest) (*LeverageUpdate, error) {
	return defaultTradeProvider.SetLeverage(req)
}
