package analyst

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"trading-go/tools"
)

func TestResolveADKAgentNameFallsBackWhenEventNameEmpty(t *testing.T) {
	got := resolveADKAgentName(&adk.AgentEvent{}, "TechnicalAnalystAgent")
	if got != "TechnicalAnalystAgent" {
		t.Fatalf("expected fallback agent name, got %q", got)
	}
}

func TestResolveADKAgentNamePrefersEventName(t *testing.T) {
	got := resolveADKAgentName(&adk.AgentEvent{AgentName: "NewsAnalystAgent"}, "TechnicalAnalystAgent")
	if got != "NewsAnalystAgent" {
		t.Fatalf("expected event agent name, got %q", got)
	}
}

func TestExtractReasoningContentReadsSchemaFieldFirst(t *testing.T) {
	got := extractReasoningContent(&schema.Message{
		ReasoningContent: "  choose technical indicators before final report  ",
		Extra:            map[string]any{"reasoning": "fallback"},
	})
	if got != "choose technical indicators before final report" {
		t.Fatalf("unexpected reasoning content: %q", got)
	}
}

func TestExtractReasoningContentReadsProviderExtra(t *testing.T) {
	got := extractReasoningContent(&schema.Message{
		Extra: map[string]any{"reasoning-content": "  inspect fresh klines  "},
	})
	if got != "inspect fresh klines" {
		t.Fatalf("unexpected reasoning content: %q", got)
	}
}

func TestShortenLogTextIsRuneSafe(t *testing.T) {
	got := shortenLogText("技术分析报告", 4)
	want := "技术分析...(truncated)"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestFormatKlineToolResultReturnsOHLCVSeries(t *testing.T) {
	raw, err := formatKlineToolResult("aapl", "60m", []tools.KLine{
		{
			Time:   time.Date(2026, 4, 24, 1, 0, 0, 0, time.UTC),
			Open:   100.1,
			High:   101.2,
			Low:    99.8,
			Close:  100.9,
			Volume: 12345,
		},
	})
	if err != nil {
		t.Fatalf("formatKlineToolResult error: %v", err)
	}

	var got struct {
		Symbol   string `json:"symbol"`
		Period   string `json:"period"`
		BarCount int    `json:"bar_count"`
		Bars     []struct {
			Time   string  `json:"time"`
			Open   float64 `json:"open"`
			High   float64 `json:"high"`
			Low    float64 `json:"low"`
			Close  float64 `json:"close"`
			Volume float64 `json:"volume"`
		} `json:"bars"`
	}
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v; raw=%s", err, raw)
	}

	if got.Symbol != "AAPL" {
		t.Fatalf("expected symbol AAPL, got %q", got.Symbol)
	}
	if got.Period != "1h" {
		t.Fatalf("expected normalized period 1h, got %q", got.Period)
	}
	if got.BarCount != 1 || len(got.Bars) != 1 {
		t.Fatalf("expected one bar, got bar_count=%d len=%d", got.BarCount, len(got.Bars))
	}
	if got.Bars[0].Open != 100.1 || got.Bars[0].High != 101.2 || got.Bars[0].Low != 99.8 || got.Bars[0].Close != 100.9 {
		t.Fatalf("unexpected bar payload: %+v", got.Bars[0])
	}
	if got.Bars[0].Time != "2026-04-24T01:00:00Z" {
		t.Fatalf("unexpected bar time: %s", got.Bars[0].Time)
	}
}

func TestNormalizeKlinePeriodLabel(t *testing.T) {
	tests := map[string]string{
		"1m":      "1m",
		"60m":     "1h",
		"1h":      "1h",
		"1month":  "1mo",
		"1mth":    "1mo",
		" 1d ":    "1d",
		"unknown": "unknown",
	}

	for input, want := range tests {
		if got := normalizeKlinePeriodLabel(input); got != want {
			t.Fatalf("normalizeKlinePeriodLabel(%q) = %q, want %q", input, got, want)
		}
	}
}
