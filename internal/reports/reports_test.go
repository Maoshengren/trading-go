package reports

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"trading-go/internal/logx"
	"trading-go/state"
)

func TestWriteRunPersistsStageReportsAndCombinedReport(t *testing.T) {
	dir := t.TempDir()
	ctx := logx.WithTraceID(context.Background(), "trace/test:123")
	s := &state.AgentState{
		Symbol:          "AAPL.US",
		TechnicalReport: "technical report body",
		BullArguments:   []string{"bull round one"},
		BearArguments:   []string{"bear round one"},
		ResearchDebateTurns: []state.ResearchDebateTurn{
			{Round: 1, Agent: "BullResearcherAgent", Stance: "bull", Argument: "bull round one"},
			{Round: 1, Agent: "BearResearcherAgent", Stance: "bear", Argument: "bear round one", RespondsTo: "bull round one"},
		},
		ResearchSummary:   "research summary body",
		FinalDecision:     "final decision body",
		TraderDecision:    state.TraderDecision{Direction: "buy", Strength: 7, Confidence: 80, PositionSize: 0.25, Reasoning: "trader reasoning"},
		RiskReview:        state.RiskReview{Conclusion: "approve", Suggestion: "keep stop loss"},
		SentimentReport:   "",
		NewsReport:        "",
		FundamentalReport: "",
	}

	runDir, err := WriteRun(ctx, dir, s)
	if err != nil {
		t.Fatalf("WriteRun error: %v", err)
	}
	if !strings.HasSuffix(runDir, filepath.Join("trace-test-123-AAPL.US")) {
		t.Fatalf("unexpected run dir: %s", runDir)
	}

	for _, rel := range []string{
		"00-index.md",
		"combined.md",
		"analysts/04-technical.md",
		"research/00-debate-history.md",
		"research/01-bull.md",
		"research/03-summary.md",
		"trading/01-trader-decision.md",
		"risk/02-risk-review.md",
		"portfolio/01-final-decision.md",
	} {
		if _, err := os.Stat(filepath.Join(runDir, rel)); err != nil {
			t.Fatalf("expected report %s to exist: %v", rel, err)
		}
	}

	combined, err := os.ReadFile(filepath.Join(runDir, "combined.md"))
	if err != nil {
		t.Fatalf("ReadFile combined error: %v", err)
	}
	for _, want := range []string{"technical report body", "bull round one", "bear round one", "final decision body"} {
		if !strings.Contains(string(combined), want) {
			t.Fatalf("combined report missing %q:\n%s", want, string(combined))
		}
	}
}
