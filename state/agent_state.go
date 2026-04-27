package state

type TraderDecision struct {
	Direction    string
	Strength     int
	Confidence   int
	PositionSize float64
	Reasoning    string
}

type RiskReview struct {
	Conclusion string
	Suggestion string
}

type PortfolioDecision struct {
	Execution             string
	Action                string
	Direction             string
	ApprovedPositionSize  float64
	ApprovedPositionRatio float64
	TargetPositionQty     float64
	Summary               string
}

type ResearchDebateTurn struct {
	Round      int
	Agent      string
	Stance     string
	Argument   string
	RespondsTo string
}

type ExecutionResult struct {
	Status            string
	DryRun            bool
	Symbol            string
	Side              string
	ReduceOnly        bool
	PositionSide      string
	OrderType         string
	TimeInForce       string
	Quantity          float64
	ReferencePrice    float64
	Leverage          int
	OrderID           string
	TakeProfitPrice   float64
	TakeProfitOrderID string
	StopLossPrice     float64
	StopLossOrderID   string
	ProtectionStatus  string
	ProtectionReason  string
	Reason            string
}

type AgentState struct {
	Symbol               string
	FundamentalReport    string
	SentimentReport      string
	NewsReport           string
	TechnicalReport      string
	BullArguments        []string
	BearArguments        []string
	ResearchDebateTurns  []ResearchDebateTurn
	ResearchSummary      string
	TraderDecision       TraderDecision
	RiskAssessmentReport string
	RiskReview           RiskReview
	PortfolioDecision    PortfolioDecision
	FinalDecision        string
	ExecutionResult      ExecutionResult
}
