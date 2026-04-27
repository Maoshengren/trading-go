package reports

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"trading-go/internal/logx"
	"trading-go/state"
)

const DefaultDir = "reports"

type section struct {
	Stage string
	Name  string
	Path  string
	Body  string
}

var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// WriteRun persists a completed workflow's reports into markdown files grouped by stage.
func WriteRun(ctx context.Context, dir string, s *state.AgentState) (string, error) {
	if s == nil {
		return "", fmt.Errorf("agent state is nil")
	}
	if strings.TrimSpace(dir) == "" {
		dir = DefaultDir
	}

	traceID := logx.TraceIDFromContext(ctx)
	if strings.TrimSpace(traceID) == "" {
		traceID = fallbackTraceID(time.Now())
	}
	symbol := sanitizeFilename(s.Symbol)
	if symbol == "" {
		symbol = "unknown-symbol"
	}

	runDir := filepath.Join(dir, fmt.Sprintf("%s-%s", sanitizeFilename(traceID), symbol))
	sections := buildSections(s)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", fmt.Errorf("create report dir: %w", err)
	}

	for _, item := range sections {
		if strings.TrimSpace(item.Body) == "" {
			continue
		}
		path := filepath.Join(runDir, item.Path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("create report stage dir: %w", err)
		}
		if err := os.WriteFile(path, []byte(renderSection(ctx, item, s.Symbol)), 0o644); err != nil {
			return "", fmt.Errorf("write report %s: %w", item.Path, err)
		}
	}

	if err := os.WriteFile(filepath.Join(runDir, "00-index.md"), []byte(renderIndex(ctx, sections, s)), 0o644); err != nil {
		return "", fmt.Errorf("write report index: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "combined.md"), []byte(renderCombined(ctx, sections, s)), 0o644); err != nil {
		return "", fmt.Errorf("write combined report: %w", err)
	}

	logx.Logger(ctx).WithFields(logrus.Fields{
		"report_dir":    runDir,
		"report_count":  countNonEmpty(sections),
		"report_symbol": s.Symbol,
	}).Info("workflow reports persisted")
	return runDir, nil
}

func buildSections(s *state.AgentState) []section {
	return []section{
		{Stage: "Analyst", Name: "Fundamental Analyst", Path: "analysts/01-fundamental.md", Body: s.FundamentalReport},
		{Stage: "Analyst", Name: "Sentiment Analyst", Path: "analysts/02-sentiment.md", Body: s.SentimentReport},
		{Stage: "Analyst", Name: "News Analyst", Path: "analysts/03-news.md", Body: s.NewsReport},
		{Stage: "Analyst", Name: "Technical Analyst", Path: "analysts/04-technical.md", Body: s.TechnicalReport},
		{Stage: "Research", Name: "Debate History", Path: "research/00-debate-history.md", Body: renderDebateHistory(s.ResearchDebateTurns)},
		{Stage: "Research", Name: "Bull Researcher", Path: "research/01-bull.md", Body: renderArguments("Bull Researcher", s.BullArguments)},
		{Stage: "Research", Name: "Bear Researcher", Path: "research/02-bear.md", Body: renderArguments("Bear Researcher", s.BearArguments)},
		{Stage: "Research", Name: "Research Manager", Path: "research/03-summary.md", Body: s.ResearchSummary},
		{Stage: "Trading", Name: "Trader", Path: "trading/01-trader-decision.md", Body: renderTraderDecision(s.TraderDecision)},
		{Stage: "Risk", Name: "Risk Analyst", Path: "risk/01-risk-assessment.md", Body: s.RiskAssessmentReport},
		{Stage: "Risk", Name: "Risk Manager", Path: "risk/02-risk-review.md", Body: renderRiskReview(s.RiskReview)},
		{Stage: "Portfolio", Name: "Portfolio Manager", Path: "portfolio/01-final-decision.md", Body: renderPortfolioDecision(s.PortfolioDecision, s.FinalDecision)},
		{Stage: "Execution", Name: "Execution Agent", Path: "execution/01-execution-result.md", Body: renderExecutionResult(s.ExecutionResult)},
	}
}

func renderSection(ctx context.Context, item section, symbol string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("symbol: %s\n", symbol))
	b.WriteString(fmt.Sprintf("trace_id: %s\n", logx.TraceIDFromContext(ctx)))
	b.WriteString(fmt.Sprintf("stage: %s\n", item.Stage))
	b.WriteString(fmt.Sprintf("name: %s\n", item.Name))
	b.WriteString(fmt.Sprintf("generated_at: %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString("---\n\n")
	b.WriteString(fmt.Sprintf("# %s\n\n", item.Name))
	b.WriteString(strings.TrimSpace(item.Body))
	b.WriteString("\n")
	return b.String()
}

func renderIndex(ctx context.Context, sections []section, s *state.AgentState) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Trading Report Index: %s\n\n", s.Symbol))
	b.WriteString(fmt.Sprintf("- Trace ID: `%s`\n", logx.TraceIDFromContext(ctx)))
	b.WriteString(fmt.Sprintf("- Generated At: `%s`\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Final Decision: %s\n\n", blankDash(s.FinalDecision)))
	b.WriteString("## Files\n\n")
	for _, item := range sections {
		if strings.TrimSpace(item.Body) == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("- `%s` - %s / %s\n", item.Path, item.Stage, item.Name))
	}
	b.WriteString("\n## Merge Entry\n\n")
	b.WriteString("- Start with `combined.md` for a single-file review.\n")
	b.WriteString("- Use stage folders when you want to re-rank or rewrite individual analyst outputs.\n")
	return b.String()
}

func renderCombined(ctx context.Context, sections []section, s *state.AgentState) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Combined Trading Report: %s\n\n", s.Symbol))
	b.WriteString(fmt.Sprintf("- Trace ID: `%s`\n", logx.TraceIDFromContext(ctx)))
	b.WriteString(fmt.Sprintf("- Generated At: `%s`\n\n", time.Now().Format(time.RFC3339)))
	for _, item := range sections {
		if strings.TrimSpace(item.Body) == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("## %s - %s\n\n", item.Stage, item.Name))
		b.WriteString(strings.TrimSpace(item.Body))
		b.WriteString("\n\n")
	}
	return b.String()
}

func renderArguments(title string, items []string) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("## %s Arguments\n\n", title))
	for i, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("### Round %d\n\n%s\n\n", i+1, strings.TrimSpace(item)))
	}
	return strings.TrimSpace(b.String())
}

func renderDebateHistory(turns []state.ResearchDebateTurn) string {
	if len(turns) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Research Debate History\n\n")
	for _, turn := range turns {
		b.WriteString(fmt.Sprintf("## Round %d - %s (%s)\n\n", turn.Round, turn.Agent, turn.Stance))
		if strings.TrimSpace(turn.RespondsTo) != "" {
			b.WriteString("### Responds To\n\n")
			b.WriteString(strings.TrimSpace(turn.RespondsTo))
			b.WriteString("\n\n")
		}
		b.WriteString("### Argument\n\n")
		b.WriteString(strings.TrimSpace(turn.Argument))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func renderTraderDecision(d state.TraderDecision) string {
	if d.Direction == "" && d.Reasoning == "" && d.Strength == 0 && d.Confidence == 0 && d.PositionSize == 0 {
		return ""
	}
	return fmt.Sprintf(
		"Direction: `%s`\n\nStrength: `%d`\n\nConfidence: `%d`\n\nPosition Size: `%.4f`\n\n## Reasoning\n\n%s",
		d.Direction,
		d.Strength,
		d.Confidence,
		d.PositionSize,
		blankDash(d.Reasoning),
	)
}

func renderRiskReview(r state.RiskReview) string {
	if r.Conclusion == "" && r.Suggestion == "" {
		return ""
	}
	return fmt.Sprintf("Conclusion: `%s`\n\n## Suggestion\n\n%s", r.Conclusion, blankDash(r.Suggestion))
}

func renderExecutionResult(r state.ExecutionResult) string {
	if strings.TrimSpace(r.Status) == "" && strings.TrimSpace(r.Reason) == "" && strings.TrimSpace(r.Symbol) == "" {
		return ""
	}
	return fmt.Sprintf(
		"Status: `%s`\n\nDry Run: `%t`\n\nSymbol: `%s`\n\nSide: `%s`\n\nReduce Only: `%t`\n\nPosition Side: `%s`\n\nOrder Type: `%s`\n\nTime In Force: `%s`\n\nQuantity: `%.8f`\n\nReference Price: `%.8f`\n\nLeverage: `%d`\n\nOrder ID: `%s`\n\nTake Profit Price: `%.8f`\n\nTake Profit Order ID: `%s`\n\nStop Loss Price: `%.8f`\n\nStop Loss Order ID: `%s`\n\nProtection Status: `%s`\n\n## Protection Reason\n\n%s\n\n## Reason\n\n%s",
		r.Status,
		r.DryRun,
		blankDash(r.Symbol),
		blankDash(r.Side),
		r.ReduceOnly,
		blankDash(r.PositionSide),
		blankDash(r.OrderType),
		blankDash(r.TimeInForce),
		r.Quantity,
		r.ReferencePrice,
		r.Leverage,
		blankDash(r.OrderID),
		r.TakeProfitPrice,
		blankDash(r.TakeProfitOrderID),
		r.StopLossPrice,
		blankDash(r.StopLossOrderID),
		blankDash(r.ProtectionStatus),
		blankDash(r.ProtectionReason),
		blankDash(r.Reason),
	)
}

func renderPortfolioDecision(d state.PortfolioDecision, fallback string) string {
	if strings.TrimSpace(d.Execution) == "" && strings.TrimSpace(d.Summary) == "" {
		return strings.TrimSpace(fallback)
	}
	return fmt.Sprintf(
		"Execution: `%s`\n\nAction: `%s`\n\nDirection: `%s`\n\nApproved Position Size: `%.4f`\n\nApproved Position Ratio: `%.4f`\n\nTarget Position Qty: `%.8f`\n\n## Summary\n\n%s",
		blankDash(d.Execution),
		blankDash(d.Action),
		blankDash(d.Direction),
		d.ApprovedPositionSize,
		d.ApprovedPositionRatio,
		d.TargetPositionQty,
		blankDash(d.Summary),
	)
}

func sanitizeFilename(value string) string {
	value = strings.TrimSpace(value)
	value = unsafeFilenameChars.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return value
}

func blankDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return strings.TrimSpace(value)
}

func countNonEmpty(sections []section) int {
	count := 0
	for _, item := range sections {
		if strings.TrimSpace(item.Body) != "" {
			count++
		}
	}
	return count
}

func fallbackTraceID(now time.Time) string {
	return fmt.Sprintf("trace-%s-%d", now.Format("20060102"), now.UnixNano())
}
