package analyst

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	toolcomponent "github.com/cloudwego/eino/components/tool"
	"github.com/sirupsen/logrus"

	"trading-go/internal/logx"
	"trading-go/internal/prompts"
	"trading-go/state"
	"trading-go/tools"
)

// SentimentAnalystAgent 负责情绪面分析：读取情绪分值并输出报告。
type SentimentAnalystAgent struct {
	agent        adk.Agent
	boundTools   []toolcomponent.BaseTool
	systemPrompt string
}

// NewSentimentAnalystAgent 创建并绑定情绪工具与 ADK Agent。
func NewSentimentAnalystAgent() (*SentimentAnalystAgent, error) {
	t, err := buildSocialSentimentTool()
	if err != nil {
		return nil, err
	}
	agent, err := newReportADKAgent("SentimentAnalystAgent", "Analyze social sentiment and market mood.", prompts.SentimentAnalystSystem, []toolcomponent.BaseTool{t})
	if err != nil {
		return nil, err
	}
	return &SentimentAnalystAgent{agent: agent, boundTools: []toolcomponent.BaseTool{t}, systemPrompt: prompts.SentimentAnalystSystem}, nil
}

func (a *SentimentAnalystAgent) Name() string { return "SentimentAnalystAgent" }
func (a *SentimentAnalystAgent) Description() string {
	return "Analyze social sentiment and market mood."
}
func (a *SentimentAnalystAgent) SystemPrompt() string { return a.systemPrompt }
func (a *SentimentAnalystAgent) Tools() []toolcomponent.BaseTool {
	return a.boundTools
}

// Run 执行情绪分析流程并更新 AgentState。
func (a *SentimentAnalystAgent) Run(ctx context.Context, s *state.AgentState) error {
	if a.agent == nil {
		b, err := NewSentimentAnalystAgent()
		if err != nil {
			return err
		}
		*a = *b
	}
	score, err := tools.GetSocialSentiment(s.Symbol)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Error("failed to fetch social sentiment")
		return err
	}
	logx.Logger(ctx).WithFields(logrus.Fields{"agent": a.Name(), "symbol": s.Symbol}).Info("running sentiment analysis")
	payload := fmt.Sprintf("symbol=%s sentiment_score=%.4f", strings.ToUpper(s.Symbol), score)
	out, err := runADKReport(ctx, a.agent, payload)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Error("failed to generate sentiment report")
		return err
	}
	s.SentimentReport = out
	return nil
}
