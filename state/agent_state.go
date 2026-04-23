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

type AgentState struct {
	Symbol               string
	FundamentalReport    string
	SentimentReport      string
	NewsReport           string
	TechnicalReport      string
	BullArguments        []string
	BearArguments        []string
	ResearchSummary      string
	TraderDecision       TraderDecision
	RiskAssessmentReport string
	RiskReview           RiskReview
	FinalDecision        string
}
