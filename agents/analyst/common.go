package analyst

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	toolcomponent "github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/sirupsen/logrus"

	"trading-go/internal/llm"
	"trading-go/internal/logx"

	"trading-go/tools"
)

type symbolInput struct {
	Symbol string `json:"symbol" jsonschema:"required" jsonschema_description:"股票代码"`
}

type newsInput struct {
	Symbol string `json:"symbol" jsonschema:"required" jsonschema_description:"股票代码"`
	Days   int    `json:"days" jsonschema:"required" jsonschema_description:"回溯天数"`
}

type klineInput struct {
	Symbol string `json:"symbol" jsonschema:"required" jsonschema_description:"股票代码"`
	Period string `json:"period" jsonschema:"required" jsonschema_description:"K线周期，例如 1d/1h"`
}

// newReportADKAgent 构造一个可运行的 ChatModelAgent，注入系统提示词与工具集合。
func newReportADKAgent(name, description, prompt string, boundTools []toolcomponent.BaseTool) (adk.Agent, error) {
	m, err := llm.Model()
	if err != nil {
		return nil, err
	}
	return adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        name,
		Description: description,
		Instruction: prompt,
		Model:       m,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: boundTools},
		},
	})
}

// runADKReport 统一消费 ADK 事件流，并提取最后一条非空文本作为报告。
func runADKReport(ctx context.Context, agent adk.Agent, payload string) (string, error) {
	agentName := strings.TrimSpace(agent.Name(ctx))
	if agentName == "" {
		agentName = "unknown_adk_agent"
	}
	logx.Logger(ctx).WithFields(logrus.Fields{
		"adk_phase":  "input",
		"agent_name": agentName,
		"payload":    payload,
	}).Info("running ADK analyst report")
	iter := agent.Run(ctx, &adk.AgentInput{
		Messages: []adk.Message{schema.UserMessage(payload)},
	})
	var last string
	eventIndex := 0
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event == nil {
			continue
		}
		eventIndex++
		if event.Err != nil {
			logADKEvent(ctx, agentName, eventIndex, event, event.Err)
			return "", event.Err
		}
		logADKEvent(ctx, agentName, eventIndex, event, nil)
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		msg, err := event.Output.MessageOutput.GetMessage()
		if err != nil || msg == nil {
			continue
		}
		if strings.TrimSpace(msg.Content) != "" {
			last = msg.Content
		}
	}
	if strings.TrimSpace(last) == "" {
		logx.Logger(ctx).Warn("analyst report generated empty content")
		return "报告生成完成（空内容）", nil
	}
	logx.Logger(ctx).WithField("report_length", len(last)).Info("analyst report generated")
	return last, nil
}

func logADKEvent(ctx context.Context, fallbackAgentName string, eventIndex int, event *adk.AgentEvent, eventErr error) {
	fields := logrus.Fields{
		"adk_phase":   "event",
		"event_index": eventIndex,
		"agent_name":  resolveADKAgentName(event, fallbackAgentName),
	}
	if strings.TrimSpace(event.AgentName) == "" && strings.TrimSpace(fallbackAgentName) != "" {
		fields["agent_name_source"] = "run_agent_fallback"
	}

	if len(event.RunPath) > 0 {
		parts := make([]string, 0, len(event.RunPath))
		for _, step := range event.RunPath {
			parts = append(parts, step.String())
		}
		fields["run_path"] = strings.Join(parts, " -> ")
	}

	if event.Output != nil && event.Output.MessageOutput != nil {
		fields["message_role"] = string(event.Output.MessageOutput.Role)
		if event.Output.MessageOutput.ToolName != "" {
			fields["tool_name"] = event.Output.MessageOutput.ToolName
		}
		msg, err := event.Output.MessageOutput.GetMessage()
		if err != nil {
			fields["message_parse_error"] = err.Error()
		} else if msg != nil {
			if trimmed := strings.TrimSpace(msg.Content); trimmed != "" {
				contentLimit := 1200
				if msg.Role == schema.Tool {
					contentLimit = 4000
					fields["tool_result"] = shortenLogText(trimmed, contentLimit)
					fields["tool_result_length"] = runeLen(trimmed)
				}
				fields["message_content"] = shortenLogText(trimmed, contentLimit)
				fields["message_content_length"] = runeLen(trimmed)
			}
			if len(msg.ToolCalls) > 0 {
				fields["tool_calls"] = summarizeToolCalls(msg.ToolCalls)
				if reasoning := extractReasoningContent(msg); reasoning != "" {
					fields["tool_call_reasoning"] = shortenLogText(reasoning, 4000)
					fields["tool_call_reasoning_length"] = runeLen(reasoning)
				}
			}
			if msg.ToolCallID != "" {
				fields["tool_call_id"] = msg.ToolCallID
			}
			if reasoning := extractReasoningContent(msg); reasoning != "" {
				fields["reasoning_content"] = shortenLogText(reasoning, 4000)
				fields["reasoning_content_length"] = runeLen(reasoning)
			}
			if msg.ResponseMeta != nil && msg.ResponseMeta.FinishReason != "" {
				fields["finish_reason"] = msg.ResponseMeta.FinishReason
			}
			if usage := tokenUsageFields(msg.ResponseMeta); len(usage) > 0 {
				fields["token_usage"] = usage
			}
		}
	}

	if event.Action != nil {
		fields["has_action"] = true
		if event.Action.Exit {
			fields["action_exit"] = true
		}
		if event.Action.TransferToAgent != nil {
			fields["action_transfer_to"] = event.Action.TransferToAgent.DestAgentName
		}
		if event.Action.BreakLoop != nil {
			fields["action_break_loop"] = true
		}
		if event.Action.Interrupted != nil {
			fields["action_interrupted"] = true
		}
	}

	if eventErr != nil {
		fields["event_error"] = eventErr.Error()
		logx.Logger(ctx).WithFields(fields).Error("ADK dialogue event")
		return
	}

	logx.Logger(ctx).WithFields(fields).Info("ADK dialogue event")
}

func resolveADKAgentName(event *adk.AgentEvent, fallbackAgentName string) string {
	if event != nil && strings.TrimSpace(event.AgentName) != "" {
		return strings.TrimSpace(event.AgentName)
	}
	if strings.TrimSpace(fallbackAgentName) != "" {
		return strings.TrimSpace(fallbackAgentName)
	}
	return "unknown_adk_agent"
}

func summarizeToolCalls(toolCalls []schema.ToolCall) []map[string]string {
	items := make([]map[string]string, 0, len(toolCalls))
	for _, call := range toolCalls {
		items = append(items, map[string]string{
			"id":        call.ID,
			"name":      call.Function.Name,
			"arguments": shortenLogText(call.Function.Arguments, 500),
		})
	}
	return items
}

func extractReasoningContent(msg *schema.Message) string {
	if msg == nil {
		return ""
	}
	if trimmed := strings.TrimSpace(msg.ReasoningContent); trimmed != "" {
		return trimmed
	}
	if msg.Extra == nil {
		return ""
	}
	for _, key := range []string{"reasoning-content", "reasoning_content", "reasoning"} {
		if v, ok := msg.Extra[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func tokenUsageFields(meta *schema.ResponseMeta) map[string]int {
	if meta == nil || meta.Usage == nil {
		return nil
	}
	return map[string]int{
		"prompt_tokens":     meta.Usage.PromptTokens,
		"completion_tokens": meta.Usage.CompletionTokens,
		"total_tokens":      meta.Usage.TotalTokens,
		"reasoning_tokens":  meta.Usage.CompletionTokensDetails.ReasoningTokens,
	}
}

func shortenLogText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "...(truncated)"
}

func runeLen(text string) int {
	return len([]rune(text))
}

type klineToolResult struct {
	Symbol   string         `json:"symbol"`
	Period   string         `json:"period"`
	BarCount int            `json:"bar_count"`
	Bars     []klineToolBar `json:"bars"`
}

type klineToolBar struct {
	Time   string  `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type technicalIndicatorsToolResult struct {
	Symbol          string                         `json:"symbol"`
	Period          string                         `json:"period"`
	Snapshot        *tools.TechnicalSnapshot       `json:"snapshot"`
	IndicatorSeries []technicalIndicatorsToolPoint `json:"indicator_series"`
}

type technicalIndicatorsToolPoint struct {
	Time        string  `json:"time"`
	Close       float64 `json:"close"`
	RSI14       float64 `json:"rsi14"`
	MACD        float64 `json:"macd"`
	MACDSignal  float64 `json:"macd_signal"`
	MACDHist    float64 `json:"macd_hist"`
	SMA20       float64 `json:"sma20"`
	SMA50       float64 `json:"sma50"`
	ATRPct      float64 `json:"atr_pct"`
	PercentB    float64 `json:"bb_percent_b"`
	VolumeRatio float64 `json:"volume_ratio"`
	StochK      float64 `json:"stoch_k"`
	StochD      float64 `json:"stoch_d"`
}

func formatKlineToolResult(symbol, period string, klines []tools.KLine) (string, error) {
	result := klineToolResult{
		Symbol:   strings.ToUpper(strings.TrimSpace(symbol)),
		Period:   normalizeKlinePeriodLabel(period),
		BarCount: len(klines),
		Bars:     make([]klineToolBar, 0, len(klines)),
	}

	for _, k := range klines {
		result.Bars = append(result.Bars, klineToolBar{
			Time:   k.Time.UTC().Format(time.RFC3339),
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  k.Close,
			Volume: k.Volume,
		})
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal kline tool result: %w", err)
	}
	return string(payload), nil
}

func normalizeKlinePeriodLabel(period string) string {
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "1m":
		return "1m"
	case "5m":
		return "5m"
	case "15m":
		return "15m"
	case "30m":
		return "30m"
	case "1h", "60m":
		return "1h"
	case "1d":
		return "1d"
	case "1w":
		return "1w"
	case "1mo", "1mth", "1month":
		return "1mo"
	default:
		return strings.TrimSpace(period)
	}
}

func formatTechnicalIndicatorsToolResult(symbol, period string, snapshot *tools.TechnicalSnapshot, points []tools.TechnicalIndicatorPoint) (string, error) {
	result := technicalIndicatorsToolResult{
		Symbol:          strings.ToUpper(strings.TrimSpace(symbol)),
		Period:          normalizeKlinePeriodLabel(period),
		Snapshot:        snapshot,
		IndicatorSeries: make([]technicalIndicatorsToolPoint, 0, len(points)),
	}
	for _, p := range points {
		result.IndicatorSeries = append(result.IndicatorSeries, technicalIndicatorsToolPoint{
			Time:        p.Time.UTC().Format(time.RFC3339),
			Close:       p.Close,
			RSI14:       p.RSI14,
			MACD:        p.MACD,
			MACDSignal:  p.MACDSignal,
			MACDHist:    p.MACDHist,
			SMA20:       p.SMA20,
			SMA50:       p.SMA50,
			ATRPct:      p.ATRPct,
			PercentB:    p.PercentB,
			VolumeRatio: p.VolumeRatio,
			StochK:      p.StochK,
			StochD:      p.StochD,
		})
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal technical indicators tool result: %w", err)
	}
	return string(payload), nil
}

// 以下函数将项目内 tools 适配为 Eino Tool，供 Analyst Agent 绑定调用。
func buildFinancialReportsTool() (toolcomponent.BaseTool, error) {
	return toolutils.InferTool("GetFinancialReports", "获取财务报表数据", func(_ context.Context, in symbolInput) (string, error) {
		report, err := tools.GetFinancialReports(in.Symbol)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("symbol=%s revenue=%.2f net_profit=%.2f cash_flow=%.2f pe=%.2f yoy=%.2f", report.Symbol, report.Revenue, report.NetProfit, report.CashFlow, report.PE, report.YoYGrowth), nil
	})
}

func buildSocialSentimentTool() (toolcomponent.BaseTool, error) {
	return toolutils.InferTool("GetSocialSentiment", "获取社交情绪分数", func(_ context.Context, in symbolInput) (string, error) {
		score, err := tools.GetSocialSentiment(in.Symbol)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("symbol=%s sentiment=%.4f", strings.ToUpper(in.Symbol), score), nil
	})
}

func buildNewsTool() (toolcomponent.BaseTool, error) {
	return toolutils.InferTool("GetNews", "获取新闻并附带时间权重", func(_ context.Context, in newsInput) (string, error) {
		items, err := tools.GetNews(in.Symbol, in.Days)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("symbol=%s news_count=%d", strings.ToUpper(in.Symbol), len(items)), nil
	})
}

func buildKlineTool() (toolcomponent.BaseTool, error) {
	return toolutils.InferTool("GetKline", "获取真实K线数据，支持 1m/5m/15m/30m/1h/1d/1w/1mo 周期，返回完整 OHLCV K 线序列", func(_ context.Context, in klineInput) (string, error) {
		klines, err := tools.GetKline(in.Symbol, in.Period)
		if err != nil {
			return "", err
		}
		return formatKlineToolResult(in.Symbol, in.Period, klines)
	})
}

func buildTechnicalIndicatorsTool() (toolcomponent.BaseTool, error) {
	return toolutils.InferTool("BuildTechnicalIndicators", "生成统一的结构化技术指标结果，返回最新 snapshot 和最近一段连续 indicator series", func(_ context.Context, in klineInput) (string, error) {
		klines, err := tools.GetKline(in.Symbol, in.Period)
		if err != nil {
			return "", err
		}
		snapshot, err := tools.BuildTechnicalSnapshot(klines)
		if err != nil {
			return "", err
		}
		points, err := tools.BuildTechnicalIndicatorSeries(klines, 20)
		if err != nil {
			return "", err
		}
		return formatTechnicalIndicatorsToolResult(in.Symbol, in.Period, snapshot, points)
	})
}
