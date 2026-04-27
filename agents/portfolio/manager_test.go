package portfolio

import (
	"context"
	"encoding/json"
	"testing"

	"trading-go/state"
)

func TestNormalizeExecution(t *testing.T) {
	tests := map[string]string{
		"执行":    "执行",
		"拒绝":    "拒绝",
		"调整后执行": "调整后执行",
		"bad":   "调整后执行",
	}
	for input, want := range tests {
		if got := normalizeExecution(input); got != want {
			t.Fatalf("normalizeExecution(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildPortfolioPayloadIncludesTraderAndRiskInputs(t *testing.T) {
	s := &state.AgentState{
		Symbol: "AAPL",
		TraderDecision: state.TraderDecision{
			Direction:    "buy",
			Strength:     7,
			Confidence:   82,
			PositionSize: 0.3,
			Reasoning:    "trend and debate align",
		},
		RiskReview: state.RiskReview{
			Conclusion: "modify",
			Suggestion: "cut size to 0.2",
		},
		RiskAssessmentReport: "volatility high, liquidity good",
	}

	raw, err := buildPortfolioPayload(context.Background(), s)
	if err != nil {
		t.Fatalf("buildPortfolioPayload error: %v", err)
	}

	var got struct {
		Symbol         string `json:"symbol"`
		RiskReport     string `json:"risk_report"`
		TraderDecision struct {
			Direction string `json:"direction"`
			Strength  int    `json:"strength"`
		} `json:"trader_decision"`
		RiskReview struct {
			Conclusion string `json:"conclusion"`
			Suggestion string `json:"suggestion"`
		} `json:"risk_review"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v; raw=%s", err, raw)
	}
	if got.Symbol != "AAPL" {
		t.Fatalf("expected symbol AAPL, got %q", got.Symbol)
	}
	if got.TraderDecision.Direction != "buy" || got.TraderDecision.Strength != 7 {
		t.Fatalf("unexpected trader decision payload: %+v", got.TraderDecision)
	}
	if got.RiskReview.Conclusion != "modify" || got.RiskReview.Suggestion != "cut size to 0.2" {
		t.Fatalf("unexpected risk review payload: %+v", got.RiskReview)
	}
	if got.RiskReport != "volatility high, liquidity good" {
		t.Fatalf("unexpected risk report payload: %q", got.RiskReport)
	}
}

func TestClampPositionSize(t *testing.T) {
	tests := []struct {
		name               string
		input              float64
		execution          string
		traderPositionSize float64
		want               float64
	}{
		{"reject zeroes", 0.6, "拒绝", 0.8, 0},
		{"execute fallback to trader", 0, "执行", 0.3, 0.3},
		{"modify caps at trader", 0.8, "调整后执行", 0.2, 0.2},
		{"clamp to one", 1.5, "执行", 0.4, 1},
	}
	for _, tt := range tests {
		if got := clampPositionSize(tt.input, tt.execution, tt.traderPositionSize); got != tt.want {
			t.Fatalf("%s: got %f want %f", tt.name, got, tt.want)
		}
	}
}
