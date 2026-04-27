package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"trading-go/internal/llm"
	"trading-go/internal/logx"
	"trading-go/state"
)

type debatePayload struct {
	Symbol         string            `json:"symbol"`
	Round          int               `json:"round"`
	Stance         string            `json:"stance"`
	Task           string            `json:"task"`
	AnalystReports map[string]string `json:"analyst_reports"`
	DebateContext  debateContext     `json:"debate_context"`
}

type debateContext struct {
	OwnHistory      []string                   `json:"own_history"`
	OpponentHistory []string                   `json:"opponent_history"`
	OwnLastArgument string                     `json:"own_last_argument,omitempty"`
	OpponentLast    string                     `json:"opponent_last_argument,omitempty"`
	RecentTurns     []state.ResearchDebateTurn `json:"recent_turns"`
}

type debateTurnConfig struct {
	agentName       string
	stance          string
	systemPrompt    string
	ownHistory      []string
	opponentHistory []string
	appendArgument  func(string)
}

func runDebateTurn(ctx context.Context, s *state.AgentState, cfg debateTurnConfig) error {
	round := len(cfg.ownHistory) + 1
	payload, err := buildDebatePayload(s, cfg.stance, round, cfg.ownHistory, cfg.opponentHistory)
	if err != nil {
		return err
	}

	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":                  cfg.agentName,
		"symbol":                 s.Symbol,
		"round":                  round,
		"own_history_count":      len(cfg.ownHistory),
		"opponent_history_count": len(cfg.opponentHistory),
	}).Info("generating debate turn")

	argument, err := llm.Generate(ctx, cfg.systemPrompt, payload)
	if err != nil {
		return err
	}
	cfg.appendArgument(argument)
	s.ResearchDebateTurns = append(s.ResearchDebateTurns, state.ResearchDebateTurn{
		Round:      round,
		Agent:      cfg.agentName,
		Stance:     cfg.stance,
		Argument:   argument,
		RespondsTo: lastOrEmpty(cfg.opponentHistory),
	})
	return nil
}

func buildDebatePayload(s *state.AgentState, stance string, round int, ownHistory, opponentHistory []string) (string, error) {
	task := "提出新的初始论点。"
	if len(opponentHistory) > 0 {
		task = "先明确回应并反驳对手上一轮最关键的观点，再补充一条新的支持性论据。"
	}

	payload := debatePayload{
		Symbol: strings.ToUpper(strings.TrimSpace(s.Symbol)),
		Round:  round,
		Stance: stance,
		Task:   task,
		AnalystReports: map[string]string{
			"fundamental": s.FundamentalReport,
			"sentiment":   s.SentimentReport,
			"news":        s.NewsReport,
			"technical":   s.TechnicalReport,
		},
		DebateContext: debateContext{
			OwnHistory:      append([]string(nil), ownHistory...),
			OpponentHistory: append([]string(nil), opponentHistory...),
			OwnLastArgument: lastOrEmpty(ownHistory),
			OpponentLast:    lastOrEmpty(opponentHistory),
			RecentTurns:     recentDebateTurns(s.ResearchDebateTurns, 4),
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal debate payload: %w", err)
	}
	return string(data), nil
}

func buildResearchSummaryPayload(s *state.AgentState) (string, error) {
	payload := map[string]any{
		"symbol":         strings.ToUpper(strings.TrimSpace(s.Symbol)),
		"debate_rounds":  max(len(s.BullArguments), len(s.BearArguments)),
		"bull_arguments": append([]string(nil), s.BullArguments...),
		"bear_arguments": append([]string(nil), s.BearArguments...),
		"research_turns": append([]state.ResearchDebateTurn(nil), s.ResearchDebateTurns...),
		"analyst_reports": map[string]string{
			"fundamental": s.FundamentalReport,
			"sentiment":   s.SentimentReport,
			"news":        s.NewsReport,
			"technical":   s.TechnicalReport,
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal research summary payload: %w", err)
	}
	return string(data), nil
}

func recentDebateTurns(turns []state.ResearchDebateTurn, limit int) []state.ResearchDebateTurn {
	if limit <= 0 || len(turns) == 0 {
		return nil
	}
	start := len(turns) - limit
	if start < 0 {
		start = 0
	}
	return append([]state.ResearchDebateTurn(nil), turns[start:]...)
}

func lastOrEmpty(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
