package binance

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"trading-go/tools/core"
)

func TestNormalizeSymbol(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"BTC", "BTCUSDT"},
		{"btcusdt", "BTCUSDT"},
		{"BTC/USDT", "BTCUSDT"},
		{"eth-usdt", "ETHUSDT"},
	}
	for _, tt := range tests {
		got, err := NormalizeSymbol(tt.in, "USDT")
		if err != nil {
			t.Fatalf("NormalizeSymbol(%q) error: %v", tt.in, err)
		}
		if got != tt.want {
			t.Fatalf("NormalizeSymbol(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMapPeriod(t *testing.T) {
	tests := map[string]string{
		"1m":     "1m",
		"1h":     "1h",
		"60m":    "1h",
		"1d":     "1d",
		"1w":     "1w",
		"1month": "1M",
	}
	for in, want := range tests {
		got, err := MapPeriod(in)
		if err != nil {
			t.Fatalf("MapPeriod(%q) error: %v", in, err)
		}
		if got != want {
			t.Fatalf("MapPeriod(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := MapPeriod("2year"); err == nil {
		t.Fatal("expected invalid period error")
	}
}

func TestTradeMappings(t *testing.T) {
	if got, err := mapOrderType("lo"); err != nil || got != "LIMIT" {
		t.Fatalf("unexpected order type mapping: got=%q err=%v", got, err)
	}
	if got, err := mapOrderType("mo"); err != nil || got != "MARKET" {
		t.Fatalf("unexpected order type mapping: got=%q err=%v", got, err)
	}
	if got, err := mapOrderType("stop_market"); err != nil || got != "STOP_MARKET" {
		t.Fatalf("unexpected stop order type mapping: got=%q err=%v", got, err)
	}
	if got, err := mapOrderType("take_profit_market"); err != nil || got != "TAKE_PROFIT_MARKET" {
		t.Fatalf("unexpected take profit order type mapping: got=%q err=%v", got, err)
	}
	if got, err := mapOrderSide("buy"); err != nil || got != "BUY" {
		t.Fatalf("unexpected order side mapping: got=%q err=%v", got, err)
	}
	if got, err := mapTimeInForce("day"); err != nil || got != "GTC" {
		t.Fatalf("unexpected tif mapping: got=%q err=%v", got, err)
	}
	if got, ok := mapPositionSide("long"); !ok || got != "LONG" {
		t.Fatalf("unexpected position side mapping: got=%q ok=%v", got, ok)
	}
	if got, err := mapWorkingType(""); err != nil || got != "MARK_PRICE" {
		t.Fatalf("unexpected working type mapping: got=%q err=%v", got, err)
	}
	if !canSendReduceOnly("BOTH") || canSendReduceOnly("LONG") || canSendReduceOnly("SHORT") {
		t.Fatal("unexpected reduceOnly compatibility mapping")
	}
	if _, err := mapOrderType("bad"); err == nil {
		t.Fatal("expected invalid order type error")
	}
}

func TestConvertTrades(t *testing.T) {
	records := convertTrades([]userTradeResponse{{
		ID:      10,
		OrderID: 11,
		Symbol:  "BTCUSDT",
		Price:   "65000.12",
		Qty:     "0.005",
		Time:    time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC).UnixMilli(),
	}})
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].TradeID != "10" || records[0].OrderID != "11" || records[0].Quantity != 0.005 {
		t.Fatalf("unexpected record: %+v", records[0])
	}
}

func TestConvertPositions(t *testing.T) {
	positions := convertPositions([]positionRiskResponse{
		{
			Symbol:           "BTCUSDT",
			PositionAmt:      "0.010",
			EntryPrice:       "65000",
			MarkPrice:        "65100",
			UnRealizedProfit: "1.0",
			LiquidationPrice: "50000",
			Leverage:         "20",
			Notional:         "651",
			PositionSide:     "LONG",
			MarginType:       "isolated",
		},
	}, []string{"BTCUSDT"})
	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}
	if positions[0].Leverage != 20 || positions[0].PositionSide != "LONG" || positions[0].EntryPrice != 65000 {
		t.Fatalf("unexpected position conversion: %+v", positions[0])
	}
}

func TestConvertBalances(t *testing.T) {
	balances := convertBalances([]balanceResponse{{
		Asset:             "USDT",
		Balance:           "1000",
		AvailableBalance:  "800",
		WithdrawAvailable: "780",
		CrossUnPnl:        "10",
	}})
	if len(balances) != 1 {
		t.Fatalf("expected 1 balance, got %d", len(balances))
	}
	if balances[0].Currency != "USDT" || balances[0].AvailableCash != 800 || balances[0].WithdrawCash != 780 {
		t.Fatalf("unexpected balance conversion: %+v", balances[0])
	}
}

func TestSetLeverageValidation(t *testing.T) {
	p := &Provider{}
	if _, err := p.SetLeverage(core.LeverageRequest{Symbol: "BTCUSDT", Leverage: 0}); err == nil {
		t.Fatal("expected invalid leverage error")
	}
	if _, err := p.SetLeverage(core.LeverageRequest{Leverage: 20}); err == nil {
		t.Fatal("expected credential/symbol error")
	}
}

func TestSignQuery(t *testing.T) {
	got := signQuery("symbol=BTCUSDT&timestamp=1", "secret")
	if len(got) != 64 {
		t.Fatalf("expected sha256 hex length 64, got %d", len(got))
	}
	if strings.TrimSpace(got) == "" {
		t.Fatal("expected non-empty signature")
	}
}

func TestProviderUsesFuturesDefaults(t *testing.T) {
	t.Setenv(apiKeyEnv, "k")
	t.Setenv(apiSecretEnv, "s")
	t.Setenv(testnetEnv, "true")
	p, err := New()
	if err != nil {
		t.Fatalf("New error: %v", err)
	}
	if p.baseURL != testnetBaseURL {
		t.Fatalf("expected futures testnet base url, got %s", p.baseURL)
	}
	if p.accountMode != "testnet" {
		t.Fatalf("expected testnet account mode, got %s", p.accountMode)
	}
}

func TestNormalizeOrderParams(t *testing.T) {
	p := &Provider{
		exchangeInfoCache: map[string]symbolRules{
			"BTCUSDT": {
				PriceTickSize:  parseDecimal("0.10"),
				LotStepSize:    parseDecimal("0.001"),
				LotMinQty:      parseDecimal("0.001"),
				MarketStepSize: parseDecimal("0.001"),
				MarketMinQty:   parseDecimal("0.001"),
				MinNotional:    parseDecimal("100"),
			},
		},
	}

	qty, price, triggerPrice, err := p.normalizeOrderParams("BTCUSDT", "MARKET", 0.0123456, 65000, 0)
	if err != nil {
		t.Fatalf("normalizeOrderParams market error: %v", err)
	}
	if qty != 0.012 {
		t.Fatalf("expected qty 0.012, got %f", qty)
	}
	if price != 0 {
		t.Fatalf("expected price 0 for market order, got %f", price)
	}
	if triggerPrice != 0 {
		t.Fatalf("expected trigger price 0 for market order, got %f", triggerPrice)
	}

	qty, price, triggerPrice, err = p.normalizeOrderParams("BTCUSDT", "LIMIT", 0.0123456, 65000.123, 0)
	if err != nil {
		t.Fatalf("normalizeOrderParams limit error: %v", err)
	}
	if qty != 0.012 {
		t.Fatalf("expected qty 0.012, got %f", qty)
	}
	if price != 65000.1 {
		t.Fatalf("expected price 65000.1, got %f", price)
	}
	if triggerPrice != 0 {
		t.Fatalf("expected trigger price 0 for limit order, got %f", triggerPrice)
	}

	qty, price, triggerPrice, err = p.normalizeOrderParams("BTCUSDT", "STOP_MARKET", 0.0123456, 0, 64999.987)
	if err != nil {
		t.Fatalf("normalizeOrderParams stop market error: %v", err)
	}
	if qty != 0.012 {
		t.Fatalf("expected qty 0.012, got %f", qty)
	}
	if price != 0 {
		t.Fatalf("expected price 0 for stop market order, got %f", price)
	}
	if triggerPrice != 64999.9 {
		t.Fatalf("expected trigger price 64999.9, got %f", triggerPrice)
	}
}

func TestProviderGetAccountSnapshotLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live binance futures account snapshot test in short mode")
	}
	if !hasBinanceFuturesCredentials() {
		t.Skip("skipping live binance futures account snapshot test because credentials are not configured")
	}

	p, err := New()
	if err != nil {
		t.Fatalf("New error: %v", err)
	}

	snapshot, err := p.GetAccountSnapshot(nil)
	if err != nil {
		t.Fatalf("GetAccountSnapshot error: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil account snapshot")
	}
	if snapshot.Broker != "binance_futures" {
		t.Fatalf("unexpected broker: %s", snapshot.Broker)
	}
	if snapshot.AccountMode == "" {
		t.Fatal("expected account mode to be populated")
	}
	if snapshot.RetrievedAt.IsZero() {
		t.Fatal("expected retrieved_at to be populated")
	}
}

func TestSubmitTriggerOrderBuildsStopPriceAndReduceOnly(t *testing.T) {
	var capturedQuery string
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/fapi/v1/order" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			capturedQuery = r.URL.RawQuery
			body, _ := json.Marshal(orderResponse{
				Symbol:     "BTCUSDT",
				OrderID:    123,
				UpdateTime: time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC).UnixMilli(),
			})
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(body))),
			}, nil
		}),
	}
	p := &Provider{
		httpClient:        client,
		baseURL:           "https://unit.test",
		apiKey:            "test-key",
		apiSecret:         "test-secret",
		defaultQuoteAsset: "USDT",
		accountMode:       "testnet",
		exchangeInfoCache: map[string]symbolRules{
			"BTCUSDT": {
				PriceTickSize:  parseDecimal("0.10"),
				LotStepSize:    parseDecimal("0.001"),
				LotMinQty:      parseDecimal("0.001"),
				MarketStepSize: parseDecimal("0.001"),
				MarketMinQty:   parseDecimal("0.001"),
			},
		},
	}
	submission, err := p.SubmitOrder(core.OrderRequest{
		Symbol:       "BTCUSDT",
		OrderType:    "stop_market",
		Side:         "sell",
		Quantity:     0.012345,
		TriggerPrice: 64999.987,
		ReduceOnly:   true,
		PositionSide: "BOTH",
		WorkingType:  "MARK_PRICE",
	})
	if err != nil {
		t.Fatalf("SubmitOrder error: %v", err)
	}
	if submission.OrderID != "123" || submission.TriggerPrice != 64999.9 {
		t.Fatalf("unexpected submission: %+v", submission)
	}
	values, err := url.ParseQuery(capturedQuery)
	if err != nil {
		t.Fatalf("parse captured query: %v", err)
	}
	if values.Get("type") != "STOP_MARKET" || values.Get("stopPrice") != "64999.9" || values.Get("reduceOnly") != "true" {
		t.Fatalf("unexpected trigger order query: %s", capturedQuery)
	}
	if values.Get("quantity") != "0.012" || values.Get("workingType") != "MARK_PRICE" {
		t.Fatalf("unexpected quantity/workingType query: %s", capturedQuery)
	}
}

func TestGetHistoryExecutionsSplitsLargeWindow(t *testing.T) {
	callCount := 0
	client := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			callCount++
			if r.URL.Path != "/fapi/v1/userTrades" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.URL.Query().Get("symbol") != "BTCUSDT" {
				t.Fatalf("unexpected symbol query: %s", r.URL.Query().Get("symbol"))
			}
			body, _ := json.Marshal([]userTradeResponse{})
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(body))),
			}, nil
		}),
	}

	p := &Provider{
		httpClient:        client,
		baseURL:           "https://unit.test",
		apiKey:            "test-key",
		apiSecret:         "test-secret",
		defaultQuoteAsset: "USDT",
		accountMode:       "testnet",
	}

	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(15 * 24 * time.Hour)
	_, err := p.GetHistoryExecutions(core.ExecutionHistoryQuery{
		Symbol:  "BTCUSDT",
		StartAt: start,
		EndAt:   end,
	})
	if err != nil {
		t.Fatalf("GetHistoryExecutions error: %v", err)
	}
	if callCount != 3 {
		t.Fatalf("expected 3 chunked requests, got %d", callCount)
	}
}

func hasBinanceFuturesCredentials() bool {
	loadProjectDotEnv()
	return strings.TrimSpace(firstNonEmpty(
		os.Getenv(apiKeyEnv),
		os.Getenv(legacyAPIKeyEnv),
	)) != "" && strings.TrimSpace(firstNonEmpty(
		os.Getenv(apiSecretEnv),
		os.Getenv(legacyAPISecretEnv),
	)) != ""
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
