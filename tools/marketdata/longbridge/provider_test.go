package longbridge

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/longbridge/openapi-go"
	lbtrade "github.com/longbridge/openapi-go/trade"
	"github.com/shopspring/decimal"
)

func TestNormalizeSymbol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		symbol        string
		defaultRegion string
		want          string
		wantErr       bool
	}{
		{name: "keep explicit suffix", symbol: "AAPL.US", defaultRegion: "HK", want: "AAPL.US"},
		{name: "append us by default", symbol: "AAPL", defaultRegion: "US", want: "AAPL.US"},
		{name: "append hk", symbol: "700", defaultRegion: "HK", want: "700.HK"},
		{name: "fallback invalid region", symbol: "MSFT", defaultRegion: "bad", want: "MSFT.US"},
		{name: "empty symbol", symbol: " ", defaultRegion: "US", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NormalizeSymbol(tt.symbol, tt.defaultRegion)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeSymbol error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected symbol, got %s want %s", got, tt.want)
			}
		})
	}
}

func TestMapPeriod(t *testing.T) {
	t.Parallel()

	valid := []string{"1m", "5m", "15m", "30m", "1h", "60m", "1d", "1w", "1mo", "1month"}
	for _, period := range valid {
		period := period
		t.Run(period, func(t *testing.T) {
			t.Parallel()
			if _, err := MapPeriod(period); err != nil {
				t.Fatalf("MapPeriod(%s) error: %v", period, err)
			}
		})
	}

	if _, err := MapPeriod("2h"); err == nil {
		t.Fatal("expected unsupported period error")
	}
}

func TestAnyToFloat64(t *testing.T) {
	t.Parallel()

	if got := AnyToFloat64("123.45"); got != 123.45 {
		t.Fatalf("unexpected float conversion from string: %f", got)
	}
	if got := AnyToFloat64(int64(7)); got != 7 {
		t.Fatalf("unexpected float conversion from int64: %f", got)
	}
}

func TestNormalizeLongbridgeAccountMode(t *testing.T) {
	t.Parallel()

	if got := NormalizeAccountMode("live"); got != AccountModeLive {
		t.Fatalf("expected live mode, got %s", got)
	}
	if got := NormalizeAccountMode("paper"); got != AccountModePaper {
		t.Fatalf("expected paper mode, got %s", got)
	}
	if got := NormalizeAccountMode(""); got != AccountModePaper {
		t.Fatalf("expected default paper mode, got %s", got)
	}
}

func TestResolveLongbridgeAccessToken(t *testing.T) {
	t.Setenv(PaperTokenEnv, "paper-token")
	t.Setenv(LiveTokenEnv, "live-token")
	t.Setenv("LONGBRIDGE_ACCESS_TOKEN", "fallback-token")

	if got := resolveAccessToken(AccountModePaper); got != "paper-token" {
		t.Fatalf("expected paper token, got %s", got)
	}
	if got := resolveAccessToken(AccountModeLive); got != "live-token" {
		t.Fatalf("expected live token, got %s", got)
	}

	t.Setenv(PaperTokenEnv, "")
	if got := resolveAccessToken(AccountModePaper); got != "fallback-token" {
		t.Fatalf("expected fallback token, got %s", got)
	}
}

func TestConvertPositionAndBalanceData(t *testing.T) {
	t.Parallel()

	cashBalances := convertCashBalances([]*lbtrade.AccountBalance{
		{
			Currency:  "USD",
			RiskLevel: "low",
			TotalCash: mustDecimalPtr("10000.50"),
			NetAssets: mustDecimalPtr("12000.75"),
			CashInfos: []*lbtrade.CashInfo{
				{
					Currency:      "USD",
					AvailableCash: mustDecimalPtr("8000.25"),
					FrozenCash:    mustDecimalPtr("500.00"),
				},
			},
		},
	})
	if len(cashBalances) != 1 {
		t.Fatalf("expected 1 cash balance, got %d", len(cashBalances))
	}
	if cashBalances[0].Currency != "USD" || cashBalances[0].AvailableCash != 8000.25 {
		t.Fatalf("unexpected cash balance: %+v", cashBalances[0])
	}

	stockPositions := convertStockPositions([]*lbtrade.StockPositionChannel{
		{
			AccountChannel: "lb_cash",
			Positions: []*lbtrade.StockPosition{
				{
					Symbol:            "AAPL.US",
					SymbolName:        "Apple",
					Quantity:          "10",
					AvailableQuantity: "8",
					Currency:          "USD",
					CostPrice:         mustDecimalPtr("185.32"),
					Market:            openapi.MarketUS,
				},
			},
		},
	})
	if len(stockPositions) != 1 {
		t.Fatalf("expected 1 stock position, got %d", len(stockPositions))
	}
	if stockPositions[0].Symbol != "AAPL.US" || stockPositions[0].Quantity != 10 || stockPositions[0].CostPrice != 185.32 {
		t.Fatalf("unexpected stock position: %+v", stockPositions[0])
	}

	fundPositions := convertFundPositions([]*lbtrade.FundPositionChannel{
		{
			AccountChannel: "lb_fund",
			Positions: []*lbtrade.FundPosition{
				{
					Symbol:               "FUND001",
					SymbolName:           "Growth Fund",
					Currency:             "USD",
					HoldingUnits:         mustDecimalPtr("12.5"),
					CostNetAssetValue:    mustDecimalPtr("10.1"),
					CurrentNetAssetValue: mustDecimalPtr("10.8"),
					NetAssetValueDay:     20260424,
				},
			},
		},
	})
	if len(fundPositions) != 1 {
		t.Fatalf("expected 1 fund position, got %d", len(fundPositions))
	}
	if fundPositions[0].Symbol != "FUND001" || fundPositions[0].HoldingUnits != 12.5 || fundPositions[0].CurrentNetAssetValue != 10.8 {
		t.Fatalf("unexpected fund position: %+v", fundPositions[0])
	}
}

func TestProviderGetKlineLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live longbridge test in short mode")
	}
	if !hasLongbridgeCredentials() {
		t.Skip("skipping live longbridge test because credentials are not configured")
	}

	provider, err := New("US")
	if err != nil {
		if isTransientLiveTestError(err) {
			t.Skipf("skipping live longbridge test due to transient init error: %v", err)
		}
		t.Fatalf("New error: %v", err)
	}
	defer func() {
		if err := provider.Close(); err != nil {
			t.Fatalf("close provider error: %v", err)
		}
	}()

	klines, err := provider.GetKline("AAPL", "1d")
	if err != nil {
		if isTransientLiveTestError(err) {
			t.Skipf("skipping live longbridge test due to transient fetch error: %v", err)
		}
		t.Fatalf("GetKline error: %v", err)
	}
	if len(klines) == 0 {
		t.Fatal("expected non-empty klines from longbridge")
	}

	last := klines[len(klines)-1]
	if last.Time.IsZero() {
		t.Fatal("expected last kline time to be set")
	}
	if last.Close <= 0 {
		t.Fatalf("expected positive close price, got %f", last.Close)
	}
	if last.High < last.Low {
		t.Fatalf("expected high >= low, got high=%f low=%f", last.High, last.Low)
	}
	if last.Time.Before(time.Now().AddDate(-2, 0, 0)) {
		t.Fatalf("expected recent kline data, got timestamp %s", last.Time.Format(time.RFC3339))
	}
}

func TestProviderGetAccountSnapshotLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live longbridge account snapshot test in short mode")
	}
	if !hasLongbridgeCredentials() {
		t.Skip("skipping live longbridge account snapshot test because credentials are not configured")
	}

	provider, err := New("US")
	if err != nil {
		if isTransientLiveTestError(err) {
			t.Skipf("skipping live longbridge account snapshot test due to transient init error: %v", err)
		}
		t.Fatalf("New error: %v", err)
	}
	defer func() {
		if err := provider.Close(); err != nil {
			t.Fatalf("close provider error: %v", err)
		}
	}()

	snapshot, err := provider.GetAccountSnapshot(nil)
	if err != nil {
		if isTransientLiveTestError(err) {
			t.Skipf("skipping live longbridge account snapshot test due to transient fetch error: %v", err)
		}
		t.Fatalf("GetAccountSnapshot error: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil account snapshot")
	}
	if snapshot.Broker != "longbridge" {
		t.Fatalf("expected broker longbridge, got %s", snapshot.Broker)
	}
	if snapshot.AccountMode == "" {
		t.Fatal("expected account mode to be populated")
	}
	if snapshot.RetrievedAt.IsZero() {
		t.Fatal("expected retrieved_at to be populated")
	}
}

func TestConvertExecutions(t *testing.T) {
	t.Parallel()

	items := []*lbtrade.Execution{
		{
			OrderId:     "order-1",
			TradeId:     "trade-1",
			Symbol:      "AAPL.US",
			TradeDoneAt: time.Date(2026, 4, 24, 9, 30, 0, 0, time.UTC),
			Quantity:    "5",
			Price:       mustDecimalPtr("188.66"),
		},
	}
	got := convertExecutions(items)
	if len(got) != 1 {
		t.Fatalf("expected 1 execution, got %d", len(got))
	}
	if got[0].OrderID != "order-1" || got[0].TradeID != "trade-1" || got[0].Quantity != 5 || got[0].Price != 188.66 {
		t.Fatalf("unexpected execution conversion: %+v", got[0])
	}
}

func TestMapTradeEnums(t *testing.T) {
	t.Parallel()

	if got, err := mapOrderType("lo"); err != nil || got != lbtrade.OrderTypeLO {
		t.Fatalf("unexpected order type mapping: got=%v err=%v", got, err)
	}
	if got, err := mapOrderSide("buy"); err != nil || got != lbtrade.OrderSideBuy {
		t.Fatalf("unexpected order side mapping: got=%v err=%v", got, err)
	}
	if got, err := mapTimeInForce("gtc"); err != nil || got != lbtrade.TimeTypeGTC {
		t.Fatalf("unexpected tif mapping: got=%v err=%v", got, err)
	}
	if _, err := mapOrderType("bad"); err == nil {
		t.Fatal("expected invalid order type error")
	}
	if _, err := mapOrderSide("hold"); err == nil {
		t.Fatal("expected invalid order side error")
	}
}

func hasLongbridgeCredentials() bool {
	if os.Getenv("LONGBRIDGE_APP_KEY") != "" &&
		os.Getenv("LONGBRIDGE_APP_SECRET") != "" &&
		(os.Getenv("LONGBRIDGE_ACCESS_TOKEN") != "" ||
			os.Getenv(PaperTokenEnv) != "" ||
			os.Getenv(LiveTokenEnv) != "") {
		return true
	}

	if _, err := os.Stat(filepath.Join("..", "..", "..", ".env")); err == nil {
		return true
	}
	if _, err := os.Stat(".env"); err == nil {
		return true
	}
	return false
}

func isTransientLiveTestError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "eof")
}

func mustDecimalPtr(raw string) *decimal.Decimal {
	d, err := decimal.NewFromString(raw)
	if err != nil {
		panic(err)
	}
	return &d
}
