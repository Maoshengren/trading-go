package analyst

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/adk"
	toolcomponent "github.com/cloudwego/eino/components/tool"
	"github.com/sirupsen/logrus"

	"trading-go/internal/logx"
	"trading-go/internal/prompts"
	"trading-go/state"
	"trading-go/tools"
)

// FundamentalAnalystAgent 负责基本面分析：读取财务数据并输出结构化文本报告。
type FundamentalAnalystAgent struct {
	agent        adk.Agent
	boundTools   []toolcomponent.BaseTool
	systemPrompt string
}

// NewFundamentalAnalystAgent 创建并绑定基础工具与 ADK Agent。
func NewFundamentalAnalystAgent() (*FundamentalAnalystAgent, error) {
	t, err := buildFinancialReportsTool()
	if err != nil {
		return nil, err
	}
	agent, err := newReportADKAgent("FundamentalAnalystAgent", "Analyze financial reports and valuation.", prompts.FundamentalAnalystSystem, []toolcomponent.BaseTool{t})
	if err != nil {
		return nil, err
	}
	return &FundamentalAnalystAgent{agent: agent, boundTools: []toolcomponent.BaseTool{t}, systemPrompt: prompts.FundamentalAnalystSystem}, nil
}

func (a *FundamentalAnalystAgent) Name() string { return "FundamentalAnalystAgent" }
func (a *FundamentalAnalystAgent) Description() string {
	return "Analyze financial reports and valuation."
}
func (a *FundamentalAnalystAgent) SystemPrompt() string { return a.systemPrompt }
func (a *FundamentalAnalystAgent) Tools() []toolcomponent.BaseTool {
	return a.boundTools
}

// Run 执行基本面分析流程：取数 -> 组织输入 -> 调用 ADK -> 回写状态。
func (a *FundamentalAnalystAgent) Run(ctx context.Context, s *state.AgentState) error {
	if a.agent == nil {
		b, err := NewFundamentalAnalystAgent()
		if err != nil {
			return err
		}
		*a = *b
	}
	report, err := tools.GetFinancialReports(s.Symbol)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Error("failed to fetch financial reports")
		return err
	}
	logx.Logger(ctx).WithFields(logrus.Fields{"agent": a.Name(), "symbol": s.Symbol}).Info("running fundamental analysis")
	payload := fmt.Sprintf("symbol=%s revenue=%.2f net_profit=%.2f pe=%.2f yoy=%.2f", report.Symbol, report.Revenue, report.NetProfit, report.PE, report.YoYGrowth)
	out, err := runADKReport(ctx, a.agent, payload)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Error("failed to generate fundamental report")
		return err
	}
	s.FundamentalReport = out
	return nil
}
