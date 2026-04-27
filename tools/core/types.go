package core

import "time"

type FinancialReport struct {
	Symbol        string
	Revenue       float64
	NetProfit     float64
	CashFlow      float64
	DebtToEquity  float64
	PE            float64
	YoYGrowth     float64
	ReportQuarter string
}

type NewsItem struct {
	Symbol    string
	Title     string
	Published time.Time
	Impact    string
	Weight    float64
}

type KLine struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

type AccountSnapshot struct {
	Broker         string
	AccountMode    string
	RetrievedAt    time.Time
	CashBalances   []CashBalance
	StockPositions []StockPosition
	FundPositions  []FundPosition
}

type CashBalance struct {
	Currency               string
	TotalCash              float64
	MaxFinanceAmount       float64
	RemainingFinanceAmount float64
	NetAssets              float64
	InitMargin             float64
	MaintenanceMargin      float64
	MarginCall             float64
	RiskLevel              string
	WithdrawCash           float64
	AvailableCash          float64
	FrozenCash             float64
	SettlingCash           float64
}

type StockPosition struct {
	AccountChannel    string
	Symbol            string
	SymbolName        string
	Quantity          float64
	AvailableQuantity float64
	Currency          string
	CostPrice         float64
	Market            string
	EntryPrice        float64
	MarkPrice         float64
	UnrealizedPnL     float64
	LiquidationPrice  float64
	Notional          float64
	Leverage          int
	PositionSide      string
	MarginType        string
}

type FundPosition struct {
	AccountChannel       string
	Symbol               string
	SymbolName           string
	Currency             string
	HoldingUnits         float64
	CostNetAssetValue    float64
	CurrentNetAssetValue float64
	NetAssetValueDay     int64
}

type ExecutionHistoryQuery struct {
	Symbol  string
	StartAt time.Time
	EndAt   time.Time
}

type TodayExecutionQuery struct {
	Symbol  string
	OrderID string
}

type ExecutionRecord struct {
	OrderID     string
	TradeID     string
	Symbol      string
	TradeDoneAt time.Time
	Quantity    float64
	Price       float64
}

type OrderRequest struct {
	Symbol          string
	OrderType       string
	Side            string
	Quantity        float64
	Price           float64
	TriggerPrice    float64
	LimitOffset     float64
	TrailingAmount  float64
	TrailingPercent float64
	TimeInForce     string
	OutsideRTH      string
	PositionSide    string
	ReduceOnly      bool
	ClosePosition   bool
	WorkingType     string
	ExpireDate      *time.Time
	Remark          string
}

type OrderSubmission struct {
	Broker       string
	AccountMode  string
	OrderID      string
	Symbol       string
	OrderType    string
	Side         string
	Quantity     float64
	TriggerPrice float64
	SubmittedAt  time.Time
}

type CancelOrderRequest struct {
	Symbol  string
	OrderID string
}

type LeverageRequest struct {
	Symbol   string
	Leverage int
}

type LeverageUpdate struct {
	Broker           string
	AccountMode      string
	Symbol           string
	Leverage         int
	MaxNotionalValue float64
	UpdatedAt        time.Time
}
