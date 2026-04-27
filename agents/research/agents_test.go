package research

import (
	"encoding/json"
	"testing"

	"trading-go/state"
)

func TestBuildDebatePayloadIncludesOpponentContext(t *testing.T) {
	s := &state.AgentState{
		Symbol:            "AAPL",
		FundamentalReport: "fundamental",
		SentimentReport:   "sentiment",
		NewsReport:        "news",
		TechnicalReport:   "technical",
		BullArguments:     []string{"bull round one"},
		BearArguments:     []string{"bear round one"},
		ResearchDebateTurns: []state.ResearchDebateTurn{
			{Round: 1, Agent: "BullResearcherAgent", Stance: "bull", Argument: "bull round one"},
			{Round: 1, Agent: "BearResearcherAgent", Stance: "bear", Argument: "bear round one", RespondsTo: "bull round one"},
		},
	}

	raw, err := buildDebatePayload(s, "bull", 2, s.BullArguments, s.BearArguments)
	if err != nil {
		t.Fatalf("buildDebatePayload error: %v", err)
	}

	var got struct {
		Round         int    `json:"round"`
		Stance        string `json:"stance"`
		Task          string `json:"task"`
		DebateContext struct {
			OwnLastArgument string `json:"own_last_argument"`
			OpponentLast    string `json:"opponent_last_argument"`
			RecentTurns     []struct {
				Agent string `json:"agent"`
			} `json:"recent_turns"`
		} `json:"debate_context"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v; raw=%s", err, raw)
	}

	if got.Round != 2 || got.Stance != "bull" {
		t.Fatalf("unexpected round/stance payload: %+v", got)
	}
	if got.DebateContext.OwnLastArgument != "bull round one" {
		t.Fatalf("unexpected own_last_argument: %q", got.DebateContext.OwnLastArgument)
	}
	if got.DebateContext.OpponentLast != "bear round one" {
		t.Fatalf("unexpected opponent_last_argument: %q", got.DebateContext.OpponentLast)
	}
	if len(got.DebateContext.RecentTurns) != 2 {
		t.Fatalf("expected 2 recent turns, got %d", len(got.DebateContext.RecentTurns))
	}
}

func TestRecentDebateTurnsAppliesLimit(t *testing.T) {
	turns := []state.ResearchDebateTurn{
		{Round: 1, Agent: "a"},
		{Round: 1, Agent: "b"},
		{Round: 2, Agent: "a"},
		{Round: 2, Agent: "b"},
	}
	got := recentDebateTurns(turns, 3)
	if len(got) != 3 {
		t.Fatalf("expected 3 turns, got %d", len(got))
	}
	if got[0].Agent != "b" || got[2].Agent != "b" {
		t.Fatalf("unexpected trimmed turns: %+v", got)
	}
}
