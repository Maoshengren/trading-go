package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/shopspring/decimal"

	"trading-go/tools/core"
)

const (
	defaultBaseURL    = "https://fapi.binance.com"
	testnetBaseURL    = "https://testnet.binancefuture.com"
	defaultQuoteAsset = "USDT"
	defaultRecvWindow = "5000"
	defaultKlineLimit = 120
	maxTradeWindow    = 7 * 24 * time.Hour
)

const (
	apiKeyEnv            = "BINANCE_FUTURES_API_KEY"
	apiSecretEnv         = "BINANCE_FUTURES_API_SECRET"
	baseURLEnv           = "BINANCE_FUTURES_BASE_URL"
	testnetEnv           = "BINANCE_FUTURES_TESTNET"
	defaultQuoteAssetEnv = "BINANCE_DEFAULT_QUOTE_ASSET"
	legacyAPIKeyEnv      = "BINANCE_API_KEY"
	legacyAPISecretEnv   = "BINANCE_API_SECRET"
	legacyBaseURLEnv     = "BINANCE_SPOT_BASE_URL"
	legacyTestnetEnv     = "BINANCE_SPOT_TESTNET"
)

var (
	ErrEmptySymbol         = errors.New("symbol is required")
	ErrInvalidPeriod       = errors.New("unsupported kline period")
	ErrMissingCredentials  = errors.New("binance futures api credentials are not configured")
	ErrInvalidOrderType    = errors.New("unsupported binance futures order type")
	ErrInvalidOrderSide    = errors.New("unsupported binance futures order side")
	ErrInvalidTimeInForce  = errors.New("unsupported binance futures time in force")
	ErrInvalidPositionSide = errors.New("unsupported binance futures position side")
	ErrInvalidWorkingType  = errors.New("unsupported binance futures working type")
	ErrInvalidQuantity     = errors.New("quantity must be greater than zero")
	ErrInvalidTriggerPrice = errors.New("trigger price must be greater than zero")
	ErrInvalidOrderID      = errors.New("order id is required")
	ErrInvalidLeverage     = errors.New("leverage must be between 1 and 125")
	ErrSymbolRequired      = errors.New("symbol is required for binance futures trade history and order operations")
	ErrInvalidMinQuantity  = errors.New("order quantity is below minimum lot size")
	ErrInvalidMinNotional  = errors.New("order notional is below minimum notional")
	loadDotEnvOnce         sync.Once
)

type Provider struct {
	httpClient        *http.Client
	baseURL           string
	apiKey            string
	apiSecret         string
	defaultQuoteAsset string
	accountMode       string
	exchangeInfoMu    sync.RWMutex
	exchangeInfoCache map[string]symbolRules
}

type klineResponse [][]interface{}

type balanceResponse struct {
	AccountAlias       string `json:"accountAlias"`
	Asset              string `json:"asset"`
	Balance            string `json:"balance"`
	WithdrawAvailable  string `json:"withdrawAvailable"`
	AvailableBalance   string `json:"availableBalance"`
	CrossWalletBalance string `json:"crossWalletBalance"`
	CrossUnPnl         string `json:"crossUnPnl"`
	MarginAvailable    bool   `json:"marginAvailable"`
	UpdateTime         int64  `json:"updateTime"`
}

type positionRiskResponse struct {
	Symbol           string `json:"symbol"`
	PositionAmt      string `json:"positionAmt"`
	EntryPrice       string `json:"entryPrice"`
	MarkPrice        string `json:"markPrice"`
	UnRealizedProfit string `json:"unRealizedProfit"`
	LiquidationPrice string `json:"liquidationPrice"`
	Leverage         string `json:"leverage"`
	MaxNotionalValue string `json:"maxNotionalValue"`
	MarginType       string `json:"marginType"`
	IsolatedMargin   string `json:"isolatedMargin"`
	PositionSide     string `json:"positionSide"`
	Notional         string `json:"notional"`
	UpdateTime       int64  `json:"updateTime"`
}

type userTradeResponse struct {
	ID           int64  `json:"id"`
	OrderID      int64  `json:"orderId"`
	Symbol       string `json:"symbol"`
	Price        string `json:"price"`
	Qty          string `json:"qty"`
	QuoteQty     string `json:"quoteQty"`
	RealizedPnl  string `json:"realizedPnl"`
	Side         string `json:"side"`
	PositionSide string `json:"positionSide"`
	Time         int64  `json:"time"`
}

type orderResponse struct {
	Symbol     string `json:"symbol"`
	OrderID    int64  `json:"orderId"`
	UpdateTime int64  `json:"updateTime"`
}

type leverageResponse struct {
	Leverage         int    `json:"leverage"`
	MaxNotionalValue string `json:"maxNotionalValue"`
	Symbol           string `json:"symbol"`
}

type exchangeInfoResponse struct {
	Symbols []exchangeInfoSymbol `json:"symbols"`
}

type exchangeInfoSymbol struct {
	Symbol  string               `json:"symbol"`
	Filters []exchangeInfoFilter `json:"filters"`
}

type exchangeInfoFilter struct {
	FilterType string `json:"filterType"`
	MinPrice   string `json:"minPrice"`
	MaxPrice   string `json:"maxPrice"`
	TickSize   string `json:"tickSize"`
	MinQty     string `json:"minQty"`
	MaxQty     string `json:"maxQty"`
	StepSize   string `json:"stepSize"`
	Notional   string `json:"notional"`
}

type symbolRules struct {
	PriceTickSize  decimal.Decimal
	LotStepSize    decimal.Decimal
	LotMinQty      decimal.Decimal
	MarketStepSize decimal.Decimal
	MarketMinQty   decimal.Decimal
	MinNotional    decimal.Decimal
}

func New() (*Provider, error) {
	loadProjectDotEnv()

	baseURL := strings.TrimSpace(firstNonEmpty(os.Getenv(baseURLEnv), os.Getenv(legacyBaseURLEnv)))
	if baseURL == "" {
		if strings.EqualFold(strings.TrimSpace(firstNonEmpty(os.Getenv(testnetEnv), os.Getenv(legacyTestnetEnv))), "true") {
			baseURL = testnetBaseURL
		} else {
			baseURL = defaultBaseURL
		}
	}
	quoteAsset := strings.ToUpper(strings.TrimSpace(os.Getenv(defaultQuoteAssetEnv)))
	if quoteAsset == "" {
		quoteAsset = defaultQuoteAsset
	}

	accountMode := "futures"
	if strings.EqualFold(baseURL, testnetBaseURL) {
		accountMode = "testnet"
	}

	return &Provider{
		httpClient:        &http.Client{Timeout: 15 * time.Second},
		baseURL:           strings.TrimRight(baseURL, "/"),
		apiKey:            strings.TrimSpace(firstNonEmpty(os.Getenv(apiKeyEnv), os.Getenv(legacyAPIKeyEnv))),
		apiSecret:         strings.TrimSpace(firstNonEmpty(os.Getenv(apiSecretEnv), os.Getenv(legacyAPISecretEnv))),
		defaultQuoteAsset: quoteAsset,
		accountMode:       accountMode,
		exchangeInfoCache: make(map[string]symbolRules),
	}, nil
}

func (p *Provider) Close() error { return nil }

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
	normalizedSymbol, err := NormalizeSymbol(symbol, p.defaultQuoteAsset)
	if err != nil {
		return nil, err
	}
	interval, err := MapPeriod(period)
	if err != nil {
		return nil, err
	}

	var resp klineResponse
	if err := p.publicRequest(context.Background(), http.MethodGet, "/fapi/v1/klines", url.Values{
		"symbol":   []string{normalizedSymbol},
		"interval": []string{interval},
		"limit":    []string{strconv.Itoa(defaultKlineLimit)},
	}, &resp); err != nil {
		return nil, fmt.Errorf("fetch binance futures klines for %s: %w", normalizedSymbol, err)
	}

	klines := make([]core.KLine, 0, len(resp))
	for _, item := range resp {
		if len(item) < 6 {
			continue
		}
		openTime, _ := asInt64(item[0])
		open, _ := asFloat64(item[1])
		high, _ := asFloat64(item[2])
		low, _ := asFloat64(item[3])
		closePrice, _ := asFloat64(item[4])
		volume, _ := asFloat64(item[5])
		klines = append(klines, core.KLine{
			Time:   time.UnixMilli(openTime).UTC(),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closePrice,
			Volume: volume,
		})
	}
	return klines, nil
}

func (p *Provider) GetAccountSnapshot(symbols []string) (*core.AccountSnapshot, error) {
	if err := p.requireCredentials(); err != nil {
		return nil, err
	}

	var balancesResp []balanceResponse
	if err := p.signedRequest(context.Background(), http.MethodGet, "/fapi/v2/balance", url.Values{}, &balancesResp); err != nil {
		return nil, fmt.Errorf("fetch binance futures balances: %w", err)
	}

	params := url.Values{}
	requestedSymbols := normalizeRequestedSymbols(symbols, p.defaultQuoteAsset)
	if len(requestedSymbols) == 1 {
		params.Set("symbol", requestedSymbols[0])
	}

	var positionsResp []positionRiskResponse
	if err := p.signedRequest(context.Background(), http.MethodGet, "/fapi/v2/positionRisk", params, &positionsResp); err != nil {
		return nil, fmt.Errorf("fetch binance futures positions: %w", err)
	}

	return &core.AccountSnapshot{
		Broker:         "binance_futures",
		AccountMode:    p.accountMode,
		RetrievedAt:    time.Now().UTC(),
		CashBalances:   convertBalances(balancesResp),
		StockPositions: convertPositions(positionsResp, requestedSymbols),
	}, nil
}

func (p *Provider) GetHistoryExecutions(query core.ExecutionHistoryQuery) ([]core.ExecutionRecord, error) {
	if err := p.requireCredentials(); err != nil {
		return nil, err
	}
	symbol, err := NormalizeSymbol(query.Symbol, p.defaultQuoteAsset)
	if err != nil {
		return nil, ErrSymbolRequired
	}

	endAt := query.EndAt.UTC()
	if endAt.IsZero() {
		endAt = time.Now().UTC()
	}
	startAt := query.StartAt.UTC()
	if startAt.IsZero() || !startAt.Before(endAt) {
		startAt = endAt.Add(-maxTradeWindow)
	}

	allTrades := make([]userTradeResponse, 0)
	for windowStart := startAt; windowStart.Before(endAt); {
		windowEnd := windowStart.Add(maxTradeWindow)
		if windowEnd.After(endAt) {
			windowEnd = endAt
		}

		params := url.Values{
			"symbol":    []string{symbol},
			"startTime": []string{strconv.FormatInt(windowStart.UnixMilli(), 10)},
			"endTime":   []string{strconv.FormatInt(windowEnd.UnixMilli(), 10)},
		}
		var resp []userTradeResponse
		if err := p.signedRequest(context.Background(), http.MethodGet, "/fapi/v1/userTrades", params, &resp); err != nil {
			return nil, fmt.Errorf("fetch binance futures history trades: %w", err)
		}
		allTrades = append(allTrades, resp...)
		if !windowEnd.Before(endAt) {
			break
		}
		windowStart = windowEnd.Add(time.Millisecond)
	}
	return convertTrades(allTrades), nil
}

func (p *Provider) GetTodayExecutions(query core.TodayExecutionQuery) ([]core.ExecutionRecord, error) {
	if err := p.requireCredentials(); err != nil {
		return nil, err
	}
	symbol, err := NormalizeSymbol(query.Symbol, p.defaultQuoteAsset)
	if err != nil {
		return nil, ErrSymbolRequired
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	params := url.Values{
		"symbol":    []string{symbol},
		"startTime": []string{strconv.FormatInt(start.UnixMilli(), 10)},
		"endTime":   []string{strconv.FormatInt(now.UnixMilli(), 10)},
	}
	if strings.TrimSpace(query.OrderID) != "" {
		params.Set("orderId", strings.TrimSpace(query.OrderID))
	}

	var resp []userTradeResponse
	if err := p.signedRequest(context.Background(), http.MethodGet, "/fapi/v1/userTrades", params, &resp); err != nil {
		return nil, fmt.Errorf("fetch binance futures today trades: %w", err)
	}
	return convertTrades(resp), nil
}

func (p *Provider) SubmitOrder(req core.OrderRequest) (*core.OrderSubmission, error) {
	if err := p.requireCredentials(); err != nil {
		return nil, err
	}
	symbol, err := NormalizeSymbol(req.Symbol, p.defaultQuoteAsset)
	if err != nil {
		return nil, err
	}
	side, err := mapOrderSide(req.Side)
	if err != nil {
		return nil, err
	}
	orderType, err := mapOrderType(req.OrderType)
	if err != nil {
		return nil, err
	}
	if !req.ClosePosition && req.Quantity <= 0 {
		return nil, ErrInvalidQuantity
	}
	if isTriggerOrderType(orderType) && req.TriggerPrice <= 0 {
		return nil, ErrInvalidTriggerPrice
	}

	params := url.Values{
		"symbol": []string{symbol},
		"side":   []string{side},
		"type":   []string{orderType},
	}
	qtyForNormalization := req.Quantity
	if req.ClosePosition {
		qtyForNormalization = 1
	}
	normalizedQty, normalizedPrice, normalizedTriggerPrice, err := p.normalizeOrderParams(symbol, orderType, qtyForNormalization, req.Price, req.TriggerPrice)
	if err != nil {
		return nil, err
	}
	if req.ClosePosition {
		normalizedQty = 0
	}
	if req.ClosePosition {
		params.Set("closePosition", "true")
	} else {
		params.Set("quantity", formatDecimal(normalizedQty))
	}
	if orderType == "LIMIT" {
		tif, err := mapTimeInForce(req.TimeInForce)
		if err != nil {
			return nil, err
		}
		params.Set("timeInForce", tif)
		if normalizedPrice <= 0 {
			return nil, fmt.Errorf("price is required for limit order")
		}
		params.Set("price", formatDecimal(normalizedPrice))
	}
	if isTriggerOrderType(orderType) {
		params.Set("stopPrice", formatDecimal(normalizedTriggerPrice))
		workingType, err := mapWorkingType(req.WorkingType)
		if err != nil {
			return nil, err
		}
		if workingType != "" {
			params.Set("workingType", workingType)
		}
	}
	positionSide := ""
	if sideMode, ok := mapPositionSide(req.PositionSide); ok {
		positionSide = sideMode
		params.Set("positionSide", sideMode)
	} else if strings.TrimSpace(req.PositionSide) != "" {
		return nil, ErrInvalidPositionSide
	}
	if req.ReduceOnly && !req.ClosePosition && canSendReduceOnly(positionSide) {
		params.Set("reduceOnly", "true")
	}
	if strings.TrimSpace(req.Remark) != "" {
		params.Set("newClientOrderId", sanitizeClientOrderID(req.Remark))
	}

	var resp orderResponse
	if err := p.signedRequest(context.Background(), http.MethodPost, "/fapi/v1/order", params, &resp); err != nil {
		return nil, fmt.Errorf("submit binance futures order: %w", err)
	}
	return &core.OrderSubmission{
		Broker:       "binance_futures",
		AccountMode:  p.accountMode,
		OrderID:      strconv.FormatInt(resp.OrderID, 10),
		Symbol:       resp.Symbol,
		OrderType:    orderType,
		Side:         side,
		Quantity:     normalizedQty,
		TriggerPrice: normalizedTriggerPrice,
		SubmittedAt:  time.UnixMilli(resp.UpdateTime).UTC(),
	}, nil
}

func (p *Provider) CancelOrder(req core.CancelOrderRequest) error {
	if err := p.requireCredentials(); err != nil {
		return err
	}
	orderID := strings.TrimSpace(req.OrderID)
	if orderID == "" {
		return ErrInvalidOrderID
	}
	symbol, err := NormalizeSymbol(req.Symbol, p.defaultQuoteAsset)
	if err != nil {
		if errors.Is(err, ErrEmptySymbol) {
			return ErrSymbolRequired
		}
		return err
	}
	params := url.Values{
		"symbol":  []string{symbol},
		"orderId": []string{orderID},
	}
	if err := p.signedRequest(context.Background(), http.MethodDelete, "/fapi/v1/order", params, nil); err != nil {
		return fmt.Errorf("cancel binance futures order: %w", err)
	}
	return nil
}

func (p *Provider) SetLeverage(req core.LeverageRequest) (*core.LeverageUpdate, error) {
	if req.Leverage < 1 || req.Leverage > 125 {
		return nil, ErrInvalidLeverage
	}
	if err := p.requireCredentials(); err != nil {
		return nil, err
	}
	symbol, err := NormalizeSymbol(req.Symbol, p.defaultQuoteAsset)
	if err != nil {
		return nil, ErrSymbolRequired
	}
	params := url.Values{
		"symbol":   []string{symbol},
		"leverage": []string{strconv.Itoa(req.Leverage)},
	}
	var resp leverageResponse
	if err := p.signedRequest(context.Background(), http.MethodPost, "/fapi/v1/leverage", params, &resp); err != nil {
		return nil, fmt.Errorf("set binance futures leverage: %w", err)
	}
	return &core.LeverageUpdate{
		Broker:           "binance_futures",
		AccountMode:      p.accountMode,
		Symbol:           resp.Symbol,
		Leverage:         resp.Leverage,
		MaxNotionalValue: parseFloat(resp.MaxNotionalValue),
		UpdatedAt:        time.Now().UTC(),
	}, nil
}

func (p *Provider) publicRequest(ctx context.Context, method, path string, params url.Values, out any) error {
	endpoint := p.baseURL + path
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return err
	}
	return p.do(req, out)
}

func (p *Provider) signedRequest(ctx context.Context, method, path string, params url.Values, out any) error {
	if err := p.requireCredentials(); err != nil {
		return err
	}
	if params == nil {
		params = url.Values{}
	}
	params.Set("timestamp", strconv.FormatInt(time.Now().UTC().UnixMilli(), 10))
	params.Set("recvWindow", defaultRecvWindow)
	signature := signQuery(params.Encode(), p.apiSecret)
	params.Set("signature", signature)
	endpoint := p.baseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-MBX-APIKEY", p.apiKey)
	return p.do(req, out)
}

func (p *Provider) do(req *http.Request, out any) error {
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("binance futures api status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode binance futures response: %w", err)
	}
	return nil
}

func (p *Provider) requireCredentials() error {
	if p.apiKey == "" || p.apiSecret == "" {
		return ErrMissingCredentials
	}
	return nil
}

func NormalizeSymbol(symbol, defaultQuote string) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if s == "" {
		return "", ErrEmptySymbol
	}
	replacer := strings.NewReplacer("/", "", "-", "", "_", "", ".", "")
	s = replacer.Replace(s)
	if defaultQuote == "" {
		defaultQuote = defaultQuoteAsset
	}
	quotes := []string{strings.ToUpper(defaultQuote), "USDT", "USDC", "BUSD", "FDUSD", "BTC", "ETH", "BNB", "EUR", "TRY"}
	for _, quote := range quotes {
		if strings.HasSuffix(s, quote) && len(s) > len(quote) {
			return s, nil
		}
	}
	return s + strings.ToUpper(defaultQuote), nil
}

func MapPeriod(period string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "1m":
		return "1m", nil
	case "3m":
		return "3m", nil
	case "5m":
		return "5m", nil
	case "15m":
		return "15m", nil
	case "30m":
		return "30m", nil
	case "1h", "60m":
		return "1h", nil
	case "2h":
		return "2h", nil
	case "4h":
		return "4h", nil
	case "6h":
		return "6h", nil
	case "8h":
		return "8h", nil
	case "12h":
		return "12h", nil
	case "1d":
		return "1d", nil
	case "3d":
		return "3d", nil
	case "1w":
		return "1w", nil
	case "1mo", "1mth", "1month":
		return "1M", nil
	default:
		return "", ErrInvalidPeriod
	}
}

func mapOrderType(v string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(v))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	switch normalized {
	case "LO", "LIMIT":
		return "LIMIT", nil
	case "MO", "MARKET":
		return "MARKET", nil
	case "STOP", "STOP_LOSS", "STOP_MARKET", "SL":
		return "STOP_MARKET", nil
	case "TAKE_PROFIT", "TAKE_PROFIT_MARKET", "TP":
		return "TAKE_PROFIT_MARKET", nil
	default:
		return "", ErrInvalidOrderType
	}
}

func mapOrderSide(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "buy":
		return "BUY", nil
	case "sell":
		return "SELL", nil
	default:
		return "", ErrInvalidOrderSide
	}
}

func mapTimeInForce(v string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "", "GTC", "DAY":
		return "GTC", nil
	case "IOC":
		return "IOC", nil
	case "FOK":
		return "FOK", nil
	default:
		return "", ErrInvalidTimeInForce
	}
}

func mapPositionSide(v string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "":
		return "", false
	case "LONG":
		return "LONG", true
	case "SHORT":
		return "SHORT", true
	case "BOTH":
		return "BOTH", true
	default:
		return "", false
	}
}

func mapWorkingType(v string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "":
		return "MARK_PRICE", nil
	case "MARK_PRICE", "CONTRACT_PRICE":
		return strings.ToUpper(strings.TrimSpace(v)), nil
	default:
		return "", ErrInvalidWorkingType
	}
}

func isTriggerOrderType(orderType string) bool {
	switch strings.ToUpper(strings.TrimSpace(orderType)) {
	case "STOP_MARKET", "TAKE_PROFIT_MARKET":
		return true
	default:
		return false
	}
}

func canSendReduceOnly(positionSide string) bool {
	switch strings.ToUpper(strings.TrimSpace(positionSide)) {
	case "", "BOTH":
		return true
	default:
		return false
	}
}

func signQuery(raw, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

func parseFloat(raw string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	return f
}

func asFloat64(v interface{}) (float64, error) {
	switch value := v.(type) {
	case string:
		return strconv.ParseFloat(value, 64)
	case float64:
		return value, nil
	case int64:
		return float64(value), nil
	case json.Number:
		return value.Float64()
	default:
		return 0, fmt.Errorf("unsupported float value %T", v)
	}
}

func asInt64(v interface{}) (int64, error) {
	switch value := v.(type) {
	case int64:
		return value, nil
	case float64:
		return int64(value), nil
	case string:
		return strconv.ParseInt(value, 10, 64)
	case json.Number:
		return value.Int64()
	default:
		return 0, fmt.Errorf("unsupported int value %T", v)
	}
}

func formatDecimal(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func convertTrades(items []userTradeResponse) []core.ExecutionRecord {
	records := make([]core.ExecutionRecord, 0, len(items))
	for _, item := range items {
		records = append(records, core.ExecutionRecord{
			OrderID:     strconv.FormatInt(item.OrderID, 10),
			TradeID:     strconv.FormatInt(item.ID, 10),
			Symbol:      item.Symbol,
			TradeDoneAt: time.UnixMilli(item.Time).UTC(),
			Quantity:    parseFloat(item.Qty),
			Price:       parseFloat(item.Price),
		})
	}
	return records
}

func convertBalances(items []balanceResponse) []core.CashBalance {
	balances := make([]core.CashBalance, 0, len(items))
	for _, item := range items {
		total := parseFloat(item.Balance)
		available := parseFloat(item.AvailableBalance)
		if total == 0 && available == 0 {
			continue
		}
		balances = append(balances, core.CashBalance{
			Currency:      strings.ToUpper(item.Asset),
			TotalCash:     total,
			AvailableCash: available,
			FrozenCash:    total - available,
			WithdrawCash:  parseFloat(item.WithdrawAvailable),
			SettlingCash:  parseFloat(item.CrossUnPnl),
		})
	}
	return balances
}

func convertPositions(items []positionRiskResponse, requested []string) []core.StockPosition {
	requestedSet := make(map[string]struct{}, len(requested))
	for _, item := range requested {
		requestedSet[item] = struct{}{}
	}
	positions := make([]core.StockPosition, 0, len(items))
	for _, item := range items {
		qty := parseFloat(item.PositionAmt)
		if qty == 0 {
			continue
		}
		if len(requestedSet) > 0 {
			if _, ok := requestedSet[item.Symbol]; !ok {
				continue
			}
		}
		positions = append(positions, core.StockPosition{
			AccountChannel:    "futures",
			Symbol:            item.Symbol,
			SymbolName:        item.Symbol,
			Quantity:          qty,
			AvailableQuantity: qty,
			Currency:          defaultQuoteAsset,
			CostPrice:         parseFloat(item.EntryPrice),
			Market:            "crypto_futures",
			EntryPrice:        parseFloat(item.EntryPrice),
			MarkPrice:         parseFloat(item.MarkPrice),
			UnrealizedPnL:     parseFloat(item.UnRealizedProfit),
			LiquidationPrice:  parseFloat(item.LiquidationPrice),
			Notional:          parseFloat(item.Notional),
			Leverage:          int(parseFloat(item.Leverage)),
			PositionSide:      item.PositionSide,
			MarginType:        item.MarginType,
		})
	}
	return positions
}

func normalizeRequestedSymbols(symbols []string, defaultQuote string) []string {
	if len(symbols) == 0 {
		return nil
	}
	items := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		normalized, err := NormalizeSymbol(symbol, defaultQuote)
		if err != nil {
			continue
		}
		items = append(items, normalized)
	}
	return items
}

func (p *Provider) normalizeOrderParams(symbol, orderType string, qty, price, triggerPrice float64) (float64, float64, float64, error) {
	rules, err := p.getSymbolRules(symbol)
	if err != nil {
		return 0, 0, 0, err
	}

	qtyDec := decimal.NewFromFloat(qty)
	stepSize := rules.LotStepSize
	minQty := rules.LotMinQty
	if orderType == "MARKET" || isTriggerOrderType(orderType) {
		if rules.MarketStepSize.IsPositive() {
			stepSize = rules.MarketStepSize
		}
		if rules.MarketMinQty.IsPositive() {
			minQty = rules.MarketMinQty
		}
	}
	if !stepSize.IsPositive() {
		return 0, 0, 0, fmt.Errorf("missing step size rule for %s", symbol)
	}
	qtyDec = floorByStep(qtyDec, stepSize)
	if qtyDec.Cmp(minQty) < 0 {
		return 0, 0, 0, fmt.Errorf("%w: got=%s min=%s", ErrInvalidMinQuantity, qtyDec.String(), minQty.String())
	}

	priceDec := decimal.Zero
	if orderType == "LIMIT" {
		priceDec = decimal.NewFromFloat(price)
		if rules.PriceTickSize.IsPositive() {
			priceDec = floorByStep(priceDec, rules.PriceTickSize)
		}
	}
	triggerPriceDec := decimal.Zero
	if isTriggerOrderType(orderType) {
		triggerPriceDec = decimal.NewFromFloat(triggerPrice)
		if rules.PriceTickSize.IsPositive() {
			triggerPriceDec = floorByStep(triggerPriceDec, rules.PriceTickSize)
		}
		if !triggerPriceDec.IsPositive() {
			return 0, 0, 0, ErrInvalidTriggerPrice
		}
	}

	notionalRef := priceDec
	if !notionalRef.IsPositive() {
		notionalRef = decimal.NewFromFloat(price)
	}
	if !notionalRef.IsPositive() {
		notionalRef = triggerPriceDec
	}
	if notionalRef.IsPositive() && rules.MinNotional.IsPositive() {
		notional := qtyDec.Mul(notionalRef)
		if notional.Cmp(rules.MinNotional) < 0 {
			return 0, 0, 0, fmt.Errorf("%w: got=%s min=%s", ErrInvalidMinNotional, notional.String(), rules.MinNotional.String())
		}
	}

	qtyOut, _ := qtyDec.Float64()
	priceOut, _ := priceDec.Float64()
	triggerPriceOut, _ := triggerPriceDec.Float64()
	return qtyOut, priceOut, triggerPriceOut, nil
}

func (p *Provider) getSymbolRules(symbol string) (symbolRules, error) {
	p.exchangeInfoMu.RLock()
	if rules, ok := p.exchangeInfoCache[symbol]; ok {
		p.exchangeInfoMu.RUnlock()
		return rules, nil
	}
	p.exchangeInfoMu.RUnlock()

	var resp exchangeInfoResponse
	if err := p.publicRequest(context.Background(), http.MethodGet, "/fapi/v1/exchangeInfo", url.Values{}, &resp); err != nil {
		return symbolRules{}, fmt.Errorf("fetch binance futures exchangeInfo: %w", err)
	}

	cache := make(map[string]symbolRules, len(resp.Symbols))
	for _, item := range resp.Symbols {
		cache[item.Symbol] = buildSymbolRules(item.Filters)
	}

	p.exchangeInfoMu.Lock()
	for k, v := range cache {
		p.exchangeInfoCache[k] = v
	}
	rules, ok := p.exchangeInfoCache[symbol]
	p.exchangeInfoMu.Unlock()
	if !ok {
		return symbolRules{}, fmt.Errorf("binance futures symbol rules not found for %s", symbol)
	}
	return rules, nil
}

func buildSymbolRules(filters []exchangeInfoFilter) symbolRules {
	var rules symbolRules
	for _, filter := range filters {
		switch filter.FilterType {
		case "PRICE_FILTER":
			rules.PriceTickSize = parseDecimal(filter.TickSize)
		case "LOT_SIZE":
			rules.LotStepSize = parseDecimal(filter.StepSize)
			rules.LotMinQty = parseDecimal(filter.MinQty)
		case "MARKET_LOT_SIZE":
			rules.MarketStepSize = parseDecimal(filter.StepSize)
			rules.MarketMinQty = parseDecimal(filter.MinQty)
		case "MIN_NOTIONAL", "NOTIONAL":
			rules.MinNotional = parseDecimal(filter.Notional)
		}
	}
	return rules
}

func parseDecimal(raw string) decimal.Decimal {
	if strings.TrimSpace(raw) == "" {
		return decimal.Zero
	}
	d, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil {
		return decimal.Zero
	}
	return d
}

func floorByStep(value, step decimal.Decimal) decimal.Decimal {
	if !step.IsPositive() {
		return value
	}
	return value.Div(step).Floor().Mul(step)
}

func sanitizeClientOrderID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 36 {
		return out[:36]
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func loadProjectDotEnv() {
	loadDotEnvOnce.Do(func() {
		_ = godotenv.Load(".env")
		_ = godotenv.Load("../.env")
		_ = godotenv.Load("../../.env")
		_ = godotenv.Load("../../../.env")
	})
}
