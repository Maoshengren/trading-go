package longbridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	lbconfig "github.com/longbridge/openapi-go/config"
	lbquote "github.com/longbridge/openapi-go/quote"
	lbtrade "github.com/longbridge/openapi-go/trade"
	"github.com/shopspring/decimal"

	"trading-go/tools/core"
)

const DefaultBars = 120

const (
	AccountModeEnv   = "LONGBRIDGE_ACCOUNT_MODE"
	AccountModeLive  = "live"
	AccountModePaper = "paper"
	PaperTokenEnv    = "LONGBRIDGE_ACCESS_TOKEN_PAPER"
	LiveTokenEnv     = "LONGBRIDGE_ACCESS_TOKEN_LIVE"
)

var (
	ErrEmptySymbol        = errors.New("symbol is required")
	ErrInvalidPeriod      = errors.New("unsupported kline period")
	ErrTokenNotConfigured = errors.New("longbridge access token is not configured for selected account mode")
	ErrInvalidOrderType   = errors.New("unsupported order type")
	ErrInvalidOrderSide   = errors.New("unsupported order side")
	ErrInvalidTimeInForce = errors.New("unsupported time in force")
	ErrInvalidQuantity    = errors.New("quantity must be greater than zero")
	ErrInvalidOrderID     = errors.New("order id is required")
	loadDotEnvOnce        sync.Once
)

type Provider struct {
	quoteCtx      *lbquote.QuoteContext
	tradeCtx      *lbtrade.TradeContext
	defaultRegion string
	accountMode   string
}

func New(defaultRegion string) (*Provider, error) {
	loadProjectDotEnv()

	cfg, accountMode, err := buildConfig()
	if err != nil {
		return nil, fmt.Errorf("init longbridge config: %w", err)
	}

	quoteCtx, err := lbquote.NewFromCfg(cfg)
	if err != nil {
		return nil, fmt.Errorf("init longbridge quote context: %w", err)
	}
	tradeCtx, err := lbtrade.NewFromCfg(cfg)
	if err != nil {
		quoteCtx.Close()
		return nil, fmt.Errorf("init longbridge trade context: %w", err)
	}

	return &Provider{
		quoteCtx:      quoteCtx,
		tradeCtx:      tradeCtx,
		defaultRegion: normalizeDefaultRegion(defaultRegion),
		accountMode:   accountMode,
	}, nil
}

func (p *Provider) Close() error {
	if p.quoteCtx != nil {
		p.quoteCtx.Close()
	}
	if p.tradeCtx != nil {
		p.tradeCtx.Close()
	}
	return nil
}

func (p *Provider) GetFinancialReports(symbol string) (core.FinancialReport, error) {
	return core.FinancialReport{}, core.ErrFeatureNotImplemented
}

func (p *Provider) GetSocialSentiment(symbol string) (float64, error) {
	return 0, core.ErrFeatureNotImplemented
}

func (p *Provider) GetNews(symbol string, days int) ([]core.NewsItem, error) {
	return nil, core.ErrFeatureNotImplemented
}

func (p *Provider) GetKline(symbol, period string) ([]core.KLine, error) {
	normalizedSymbol, err := NormalizeSymbol(symbol, p.defaultRegion)
	if err != nil {
		return nil, err
	}

	normalizedPeriod, err := MapPeriod(period)
	if err != nil {
		return nil, err
	}

	items, err := p.quoteCtx.Candlesticks(
		context.Background(),
		normalizedSymbol,
		normalizedPeriod,
		DefaultBars,
		lbquote.AdjustTypeForward,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch longbridge candlesticks for %s: %w", normalizedSymbol, err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("longbridge returned no candlesticks for %s", normalizedSymbol)
	}

	klines := make([]core.KLine, 0, len(items))
	for _, item := range items {
		klines = append(klines, core.KLine{
			Time:   time.Unix(item.Timestamp, 0).UTC(),
			Open:   AnyToFloat64(item.Open),
			High:   AnyToFloat64(item.High),
			Low:    AnyToFloat64(item.Low),
			Close:  AnyToFloat64(item.Close),
			Volume: AnyToFloat64(item.Volume),
		})
	}

	sort.Slice(klines, func(i, j int) bool {
		return klines[i].Time.Before(klines[j].Time)
	})
	return klines, nil
}

func (p *Provider) GetAccountSnapshot(symbols []string) (*core.AccountSnapshot, error) {
	if p.tradeCtx == nil {
		return nil, core.ErrProviderNotConfigured
	}

	filteredSymbols := normalizePositionSymbols(symbols, p.defaultRegion)
	stockChannels, err := p.tradeCtx.StockPositions(context.Background(), filteredSymbols)
	if err != nil {
		return nil, fmt.Errorf("fetch longbridge stock positions: %w", err)
	}
	fundChannels, err := p.tradeCtx.FundPositions(context.Background(), filteredSymbols)
	if err != nil {
		return nil, fmt.Errorf("fetch longbridge fund positions: %w", err)
	}
	accountBalances, err := p.tradeCtx.AccountBalance(context.Background(), &lbtrade.GetAccountBalance{})
	if err != nil {
		return nil, fmt.Errorf("fetch longbridge account balance: %w", err)
	}

	return &core.AccountSnapshot{
		Broker:         "longbridge",
		AccountMode:    p.accountMode,
		RetrievedAt:    time.Now().UTC(),
		CashBalances:   convertCashBalances(accountBalances),
		StockPositions: convertStockPositions(stockChannels),
		FundPositions:  convertFundPositions(fundChannels),
	}, nil
}

func (p *Provider) GetHistoryExecutions(query core.ExecutionHistoryQuery) ([]core.ExecutionRecord, error) {
	if p.tradeCtx == nil {
		return nil, core.ErrProviderNotConfigured
	}

	params := &lbtrade.GetHistoryExecutions{
		StartAt: query.StartAt,
		EndAt:   query.EndAt,
	}
	if strings.TrimSpace(query.Symbol) != "" {
		symbol, err := NormalizeSymbol(query.Symbol, p.defaultRegion)
		if err != nil {
			return nil, err
		}
		params.Symbol = symbol
	}

	items, err := p.tradeCtx.HistoryExecutions(context.Background(), params)
	if err != nil {
		return nil, fmt.Errorf("fetch longbridge history executions: %w", err)
	}
	return convertExecutions(items), nil
}

func (p *Provider) GetTodayExecutions(query core.TodayExecutionQuery) ([]core.ExecutionRecord, error) {
	if p.tradeCtx == nil {
		return nil, core.ErrProviderNotConfigured
	}

	params := &lbtrade.GetTodayExecutions{
		OrderId: strings.TrimSpace(query.OrderID),
	}
	if strings.TrimSpace(query.Symbol) != "" {
		symbol, err := NormalizeSymbol(query.Symbol, p.defaultRegion)
		if err != nil {
			return nil, err
		}
		params.Symbol = symbol
	}

	items, err := p.tradeCtx.TodayExecutions(context.Background(), params)
	if err != nil {
		return nil, fmt.Errorf("fetch longbridge today executions: %w", err)
	}
	return convertExecutions(items), nil
}

func (p *Provider) SubmitOrder(req core.OrderRequest) (*core.OrderSubmission, error) {
	if p.tradeCtx == nil {
		return nil, core.ErrProviderNotConfigured
	}
	if req.Quantity <= 0 {
		return nil, ErrInvalidQuantity
	}
	if req.Quantity != float64(uint64(req.Quantity)) {
		return nil, fmt.Errorf("longbridge requires whole-share quantity")
	}
	qty := uint64(req.Quantity)

	symbol, err := NormalizeSymbol(req.Symbol, p.defaultRegion)
	if err != nil {
		return nil, err
	}
	orderType, err := mapOrderType(req.OrderType)
	if err != nil {
		return nil, err
	}
	side, err := mapOrderSide(req.Side)
	if err != nil {
		return nil, err
	}
	timeInForce, err := mapTimeInForce(req.TimeInForce)
	if err != nil {
		return nil, err
	}

	params := &lbtrade.SubmitOrder{
		Symbol:            symbol,
		OrderType:         orderType,
		Side:              side,
		SubmittedQuantity: qty,
		TimeInForce:       timeInForce,
		Remark:            strings.TrimSpace(req.Remark),
	}
	if req.ExpireDate != nil {
		params.ExpireDate = req.ExpireDate
	}
	if outsideRTH, ok := mapOutsideRTH(req.OutsideRTH); ok {
		params.OutsideRTH = outsideRTH
	}
	if req.Price > 0 {
		params.SubmittedPrice = decimalFromFloat(req.Price)
	}
	if req.TriggerPrice > 0 {
		params.TriggerPrice = decimalFromFloat(req.TriggerPrice)
	}
	if req.LimitOffset > 0 {
		params.LimitOffset = decimalFromFloat(req.LimitOffset)
	}
	if req.TrailingAmount > 0 {
		params.TrailingAmount = decimalFromFloat(req.TrailingAmount)
	}
	if req.TrailingPercent > 0 {
		params.TrailingPercent = decimalFromFloat(req.TrailingPercent)
	}

	orderID, err := p.tradeCtx.SubmitOrder(context.Background(), params)
	if err != nil {
		return nil, fmt.Errorf("submit longbridge order: %w", err)
	}
	return &core.OrderSubmission{
		Broker:      "longbridge",
		AccountMode: p.accountMode,
		OrderID:     orderID,
		Symbol:      symbol,
		OrderType:   string(orderType),
		Side:        string(side),
		Quantity:    req.Quantity,
		SubmittedAt: time.Now().UTC(),
	}, nil
}

func (p *Provider) CancelOrder(req core.CancelOrderRequest) error {
	if p.tradeCtx == nil {
		return core.ErrProviderNotConfigured
	}
	orderID := strings.TrimSpace(req.OrderID)
	if orderID == "" {
		return ErrInvalidOrderID
	}
	if err := p.tradeCtx.CancelOrder(context.Background(), orderID); err != nil {
		return fmt.Errorf("cancel longbridge order: %w", err)
	}
	return nil
}

func (p *Provider) SetLeverage(req core.LeverageRequest) (*core.LeverageUpdate, error) {
	return nil, core.ErrFeatureNotImplemented
}

func buildConfig() (*lbconfig.Config, string, error) {
	accountMode := NormalizeAccountMode(os.Getenv(AccountModeEnv))
	appKey := strings.TrimSpace(os.Getenv("LONGBRIDGE_APP_KEY"))
	appSecret := strings.TrimSpace(os.Getenv("LONGBRIDGE_APP_SECRET"))
	accessToken := strings.TrimSpace(resolveAccessToken(accountMode))
	if accessToken == "" {
		return nil, "", ErrTokenNotConfigured
	}
	cfg, err := lbconfig.New(lbconfig.WithConfigKey(appKey, appSecret, accessToken))
	if err != nil {
		return nil, "", err
	}
	return cfg, accountMode, nil
}

func NormalizeAccountMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AccountModeLive:
		return AccountModeLive
	default:
		return AccountModePaper
	}
}

func resolveAccessToken(accountMode string) string {
	if accountMode == AccountModeLive {
		if token := strings.TrimSpace(os.Getenv(LiveTokenEnv)); token != "" {
			return token
		}
	}
	if accountMode == AccountModePaper {
		if token := strings.TrimSpace(os.Getenv(PaperTokenEnv)); token != "" {
			return token
		}
	}
	return strings.TrimSpace(os.Getenv("LONGBRIDGE_ACCESS_TOKEN"))
}

func NormalizeSymbol(symbol, defaultRegion string) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if s == "" {
		return "", ErrEmptySymbol
	}
	if strings.Contains(s, ".") {
		return s, nil
	}
	return s + "." + normalizeDefaultRegion(defaultRegion), nil
}

func normalizeDefaultRegion(defaultRegion string) string {
	switch strings.ToUpper(strings.TrimSpace(defaultRegion)) {
	case "HK", "SH", "SZ":
		return strings.ToUpper(strings.TrimSpace(defaultRegion))
	default:
		return "US"
	}
}

func MapPeriod(period string) (lbquote.Period, error) {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "1m":
		return lbquote.PeriodOneMinute, nil
	case "5m":
		return lbquote.PeriodFiveMinute, nil
	case "15m":
		return lbquote.PeriodFifteenMinute, nil
	case "30m":
		return lbquote.PeriodThirtyMinute, nil
	case "1h", "60m":
		return lbquote.PeriodSixtyMinute, nil
	case "1d":
		return lbquote.PeriodDay, nil
	case "1w":
		return lbquote.PeriodWeek, nil
	case "1mo", "1mth", "1month":
		return lbquote.PeriodMonth, nil
	default:
		return 0, ErrInvalidPeriod
	}
}

func AnyToFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return 0
	}

	switch value := v.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int32:
		return float64(value)
	case int64:
		return float64(value)
	case uint:
		return float64(value)
	case uint32:
		return float64(value)
	case uint64:
		return float64(value)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil {
			return f
		}
	}

	f, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(v)), 64)
	if err != nil {
		return 0
	}
	return f
}

func normalizePositionSymbols(symbols []string, defaultRegion string) []string {
	if len(symbols) == 0 {
		return nil
	}
	items := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		normalized, err := NormalizeSymbol(symbol, defaultRegion)
		if err != nil {
			continue
		}
		items = append(items, normalized)
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

func convertCashBalances(items []*lbtrade.AccountBalance) []core.CashBalance {
	balances := make([]core.CashBalance, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		balance := core.CashBalance{
			Currency:               item.Currency,
			TotalCash:              AnyToFloat64(item.TotalCash),
			MaxFinanceAmount:       AnyToFloat64(item.MaxFinanceAmount),
			RemainingFinanceAmount: AnyToFloat64(item.RemainingFinanceAmount),
			NetAssets:              AnyToFloat64(item.NetAssets),
			InitMargin:             AnyToFloat64(item.InitMargin),
			MaintenanceMargin:      AnyToFloat64(item.MaintenanceMargin),
			MarginCall:             AnyToFloat64(item.MarginCall),
			RiskLevel:              item.RiskLevel,
		}
		if len(item.CashInfos) > 0 && item.CashInfos[0] != nil {
			balance.WithdrawCash = AnyToFloat64(item.CashInfos[0].WithdrawCash)
			balance.AvailableCash = AnyToFloat64(item.CashInfos[0].AvailableCash)
			balance.FrozenCash = AnyToFloat64(item.CashInfos[0].FrozenCash)
			balance.SettlingCash = AnyToFloat64(item.CashInfos[0].SettlingCash)
			if strings.TrimSpace(balance.Currency) == "" {
				balance.Currency = item.CashInfos[0].Currency
			}
		}
		balances = append(balances, balance)
	}
	return balances
}

func convertStockPositions(channels []*lbtrade.StockPositionChannel) []core.StockPosition {
	positions := make([]core.StockPosition, 0)
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		for _, item := range channel.Positions {
			if item == nil {
				continue
			}
			positions = append(positions, core.StockPosition{
				AccountChannel:    channel.AccountChannel,
				Symbol:            item.Symbol,
				SymbolName:        item.SymbolName,
				Quantity:          AnyToFloat64(item.Quantity),
				AvailableQuantity: AnyToFloat64(item.AvailableQuantity),
				Currency:          item.Currency,
				CostPrice:         AnyToFloat64(item.CostPrice),
				Market:            fmt.Sprint(item.Market),
			})
		}
	}
	return positions
}

func convertFundPositions(channels []*lbtrade.FundPositionChannel) []core.FundPosition {
	positions := make([]core.FundPosition, 0)
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		for _, item := range channel.Positions {
			if item == nil {
				continue
			}
			positions = append(positions, core.FundPosition{
				AccountChannel:       channel.AccountChannel,
				Symbol:               item.Symbol,
				SymbolName:           item.SymbolName,
				Currency:             item.Currency,
				HoldingUnits:         AnyToFloat64(item.HoldingUnits),
				CostNetAssetValue:    AnyToFloat64(item.CostNetAssetValue),
				CurrentNetAssetValue: AnyToFloat64(item.CurrentNetAssetValue),
				NetAssetValueDay:     item.NetAssetValueDay,
			})
		}
	}
	return positions
}

func convertExecutions(items []*lbtrade.Execution) []core.ExecutionRecord {
	records := make([]core.ExecutionRecord, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		records = append(records, core.ExecutionRecord{
			OrderID:     item.OrderId,
			TradeID:     item.TradeId,
			Symbol:      item.Symbol,
			TradeDoneAt: item.TradeDoneAt.UTC(),
			Quantity:    AnyToFloat64(item.Quantity),
			Price:       AnyToFloat64(item.Price),
		})
	}
	return records
}

func mapOrderType(v string) (lbtrade.OrderType, error) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "LO":
		return lbtrade.OrderTypeLO, nil
	case "ELO":
		return lbtrade.OrderTypeELO, nil
	case "MO":
		return lbtrade.OrderTypeMO, nil
	case "AO":
		return lbtrade.OrderTypeAO, nil
	case "ALO":
		return lbtrade.OrderTypeALO, nil
	case "ODD":
		return lbtrade.OrderTypeODD, nil
	case "LIT":
		return lbtrade.OrderTypeLIT, nil
	case "MIT":
		return lbtrade.OrderTypeMIT, nil
	case "TSLPAMT":
		return lbtrade.OrderTypeTSLPAMT, nil
	case "TSLPPCT":
		return lbtrade.OrderTypeTSLPPCT, nil
	case "TSMAMT":
		return lbtrade.OrderTypeTSMAMT, nil
	case "TSMPCT":
		return lbtrade.OrderTypeTSMPCT, nil
	case "SLO":
		return lbtrade.OrderTypeSLO, nil
	default:
		return "", ErrInvalidOrderType
	}
}

func mapOrderSide(v string) (lbtrade.OrderSide, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "buy":
		return lbtrade.OrderSideBuy, nil
	case "sell":
		return lbtrade.OrderSideSell, nil
	default:
		return "", ErrInvalidOrderSide
	}
}

func mapTimeInForce(v string) (lbtrade.TimeType, error) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "", "DAY":
		return lbtrade.TimeTypeDay, nil
	case "GTC":
		return lbtrade.TimeTypeGTC, nil
	case "GTD":
		return lbtrade.TimeTypeGTD, nil
	default:
		return "", ErrInvalidTimeInForce
	}
}

func mapOutsideRTH(v string) (lbtrade.OutsideRTH, bool) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "RTH_ONLY":
		return lbtrade.OutsideRTHOnly, true
	case "ANY_TIME":
		return lbtrade.OutsideRTHAny, true
	default:
		return "", false
	}
}

func decimalFromFloat(v float64) decimal.Decimal {
	return decimal.NewFromFloat(v)
}

func loadProjectDotEnv() {
	loadDotEnvOnce.Do(func() {
		_ = godotenv.Load(".env")
		_ = godotenv.Load("../.env")
		_ = godotenv.Load("../../.env")
		_ = godotenv.Load("../../../.env")
	})
}
