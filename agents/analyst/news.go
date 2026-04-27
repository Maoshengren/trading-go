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

// NewsAnalystAgent 负责新闻面分析：读取新闻样本并产出事件冲击结论。
type NewsAnalystAgent struct {
	agent        adk.Agent
	boundTools   []toolcomponent.BaseTool
	systemPrompt string
}

// NewNewsAnalystAgent 创建并绑定新闻工具与 ADK Agent。
func NewNewsAnalystAgent() (*NewsAnalystAgent, error) {
	t, err := buildNewsTool()
	if err != nil {
		return nil, err
	}
	agent, err := newReportADKAgent("NewsAnalystAgent", "Analyze event impact from recent news.", prompts.NewsAnalystSystem, []toolcomponent.BaseTool{t})
	if err != nil {
		return nil, err
	}
	return &NewsAnalystAgent{agent: agent, boundTools: []toolcomponent.BaseTool{t}, systemPrompt: prompts.NewsAnalystSystem}, nil
}

func (a *NewsAnalystAgent) Name() string         { return "NewsAnalystAgent" }
func (a *NewsAnalystAgent) Description() string  { return "Analyze event impact from recent news." }
func (a *NewsAnalystAgent) SystemPrompt() string { return a.systemPrompt }
func (a *NewsAnalystAgent) Tools() []toolcomponent.BaseTool {
	return a.boundTools
}

// Run 执行新闻分析流程并将结果写入 AgentState。
func (a *NewsAnalystAgent) Run(ctx context.Context, s *state.AgentState) error {
	if a.agent == nil {
		b, err := NewNewsAnalystAgent()
		if err != nil {
			return err
		}
		*a = *b
	}
	items, err := tools.GetNews(s.Symbol, 30)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Error("failed to fetch news")
		return err
	}
	logx.Logger(ctx).WithFields(logrus.Fields{"agent": a.Name(), "symbol": s.Symbol, "news_count": len(items)}).Info("running news analysis")
	payload := fmt.Sprintf("symbol=%s news_count=%d", strings.ToUpper(s.Symbol), len(items))
	out, err := runADKReport(ctx, a.agent, payload)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Error("failed to generate news report")
		return err
	}
	s.NewsReport = out
	return nil
}
