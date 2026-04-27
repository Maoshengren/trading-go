package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/compose"
	"github.com/sirupsen/logrus"

	"trading-go/agents/analyst"
	"trading-go/agents/execution"
	"trading-go/agents/portfolio"
	"trading-go/agents/research"
	"trading-go/agents/risk"
	"trading-go/agents/trader"
	"trading-go/internal/logx"
	"trading-go/internal/reports"
	"trading-go/state"
)

const (
	// bootstrap 节点负责把外部传入的 AgentState 放进 compose 的本地状态里，
	// 后续所有节点都只从这份共享状态读写，而不是手工在 context 中塞值。
	nodeBootstrap   = "bootstrap"
	nodeResearch    = "research"
	nodeTrader      = "trader"
	nodeRiskAnalyst = "risk_analyst"
	nodeRiskManager = "risk_manager"
	nodePortfolio   = "portfolio"
	nodeExecution   = "execution"
	nodeFinalize    = "finalize"
)

// Workflow 是整个五层交易分析流程的编排器。
// 这里保留了各个业务 Agent 的实例，同时把真正可执行的图编排结果缓存到 rootRunnable 中。
type Workflow struct {
	Fundamental     *analyst.FundamentalAnalystAgent
	Sentiment       *analyst.SentimentAnalystAgent
	News            *analyst.NewsAnalystAgent
	Technical       *analyst.TechnicalAnalystAgent
	Bull            *research.BullResearcherAgent
	Bear            *research.BearResearcherAgent
	ResearchMgr     *research.ManagerAgent
	Trader          *trader.TraderAgent
	RiskAnalyst     *risk.RiskAnalystAgent
	RiskManager     *risk.RiskManagerAgent
	Portfolio       *portfolio.ManagerAgent
	Execution       *execution.Agent
	enabledAnalysts []string
	analysisPeriod  string
	rootRunnable    compose.Runnable[*state.AgentState, *state.AgentState]
}

// workflowState 是 compose.Workflow 的本地共享状态。
// 和旧实现最大的区别是：状态不再靠 context.WithValue 透传，而是由图运行时统一托管。
// 由于分析师节点会并行执行，这里额外用互斥锁保护读写。
type workflowState struct {
	mu         sync.RWMutex
	agentState *state.AgentState
}

func NewWorkflow(debateRounds int, enabledAnalysts []string, analysisPeriod string, executionAgent *execution.Agent) *Workflow {
	bull := &research.BullResearcherAgent{}
	bear := &research.BearResearcherAgent{}
	technical := &analyst.TechnicalAnalystAgent{}
	technical.SetAnalysisPeriod(analysisPeriod)
	if len(enabledAnalysts) == 0 {
		enabledAnalysts = []string{"fundamental", "sentiment", "news", "technical"}
	}
	return &Workflow{
		Fundamental:     &analyst.FundamentalAnalystAgent{},
		Sentiment:       &analyst.SentimentAnalystAgent{},
		News:            &analyst.NewsAnalystAgent{},
		Technical:       technical,
		Bull:            bull,
		Bear:            bear,
		ResearchMgr:     research.NewResearchManagerAgent(debateRounds, bull, bear),
		Trader:          &trader.TraderAgent{},
		RiskAnalyst:     &risk.RiskAnalystAgent{},
		RiskManager:     &risk.RiskManagerAgent{},
		Portfolio:       &portfolio.ManagerAgent{},
		Execution:       executionAgent,
		enabledAnalysts: append([]string(nil), enabledAnalysts...),
		analysisPeriod:  analysisPeriod,
	}
}

func (w *Workflow) Run(ctx context.Context, s *state.AgentState) error {
	if err := w.ensureRunnable(ctx); err != nil {
		return err
	}

	logx.Logger(ctx).WithFields(logrus.Fields{
		"symbol":           s.Symbol,
		"enabled_analysts": strings.Join(w.enabledAnalysts, ","),
		"orchestration":    "compose.Workflow",
	}).Info("workflow started")

	result, err := w.rootRunnable.Invoke(ctx, s)
	if err != nil {
		return err
	}
	if result != nil {
		*s = *result
	}
	reportDir, err := reports.WriteRun(ctx, reports.DefaultDir, s)
	if err != nil {
		return err
	}

	logx.Logger(ctx).WithFields(logrus.Fields{
		"symbol":         s.Symbol,
		"final_decision": s.FinalDecision,
		"orchestration":  "compose.Workflow",
		"report_dir":     reportDir,
	}).Info("workflow finished")
	return nil
}

// ensureRunnable 只在第一次运行时构建一次 compose.Workflow。
// 这部分代码就是“图编排”的核心：把固定的业务阶段声明成图上的节点和依赖关系。
func (w *Workflow) ensureRunnable(ctx context.Context) error {
	if w.rootRunnable != nil {
		return nil
	}

	// NewWorkflow + WithGenLocalState 是这次改造的关键：
	// 1. 用 Workflow 显式描述节点依赖，而不是用 stateBridgeAgent 手工桥接；
	// 2. 用 local state 统一管理 AgentState 的生命周期。
	wf := compose.NewWorkflow[*state.AgentState, *state.AgentState](
		compose.WithGenLocalState(func(context.Context) *workflowState {
			return &workflowState{}
		}),
	)

	// bootstrap 是图的入口：把外部输入初始化到 workflowState 中。
	wf.AddLambdaNode(nodeBootstrap, compose.InvokableLambda(w.bootstrapNode)).AddInput(compose.START)

	// 四个分析师节点按配置动态加入图中。
	// 这一步只负责把节点挂到图上，并返回节点名，后面 research 节点会依赖它们全部完成。
	analystNodes := w.addAnalystNodes(wf)
	if len(analystNodes) == 0 {
		return fmt.Errorf("no analyst agents enabled")
	}

	// 研究节点既依赖 bootstrap，也依赖所有启用的 analyst 节点。
	// 这样可以表达“分析师并行 -> 研究汇总”的固定拓扑。
	researchNode := wf.AddLambdaNode(nodeResearch, compose.InvokableLambda(w.researchNode))
	researchNode.AddInput(nodeBootstrap)
	for _, analystNode := range analystNodes {
		researchNode.AddDependency(analystNode)
	}

	// 后续节点就是固定的顺序链：
	// Research -> Trader -> RiskAnalyst -> RiskManager -> Portfolio -> Execution -> Finalize
	wf.AddLambdaNode(nodeTrader, compose.InvokableLambda(w.traderNode)).AddInput(nodeResearch)
	wf.AddLambdaNode(nodeRiskAnalyst, compose.InvokableLambda(w.riskAnalystNode)).AddInput(nodeTrader)
	wf.AddLambdaNode(nodeRiskManager, compose.InvokableLambda(w.riskManagerNode)).AddInput(nodeRiskAnalyst)
	wf.AddLambdaNode(nodePortfolio, compose.InvokableLambda(w.portfolioNode)).AddInput(nodeRiskManager)
	wf.AddLambdaNode(nodeExecution, compose.InvokableLambda(w.executionNode)).AddInput(nodePortfolio)
	wf.AddLambdaNode(nodeFinalize, compose.InvokableLambda(w.finalizeNode)).AddInput(nodeExecution)
	wf.End().AddInput(nodeFinalize)

	runnable, err := wf.Compile(ctx, compose.WithGraphName("TradingFiveLayerGraph"))
	if err != nil {
		return err
	}
	w.rootRunnable = runnable
	return nil
}

// addAnalystNodes 根据配置把启用的分析师节点挂到图上。
// 每个节点本身只返回一个简单字符串作为“执行完成信号”，真正的业务结果写回 workflowState。
func (w *Workflow) addAnalystNodes(wf *compose.Workflow[*state.AgentState, *state.AgentState]) []string {
	nodes := make([]string, 0, len(w.enabledAnalysts))
	for _, enabled := range w.enabledAnalysts {
		switch strings.ToLower(strings.TrimSpace(enabled)) {
		case "fundamental":
			name := "analyst_fundamental"
			wf.AddLambdaNode(name, compose.InvokableLambda(
				func(ctx context.Context, _ string) (string, error) {
					return "fundamental", w.runAnalystNode(ctx, w.Fundamental.Run, func(dst, src *state.AgentState) {
						dst.FundamentalReport = src.FundamentalReport
					})
				},
			)).AddInput(nodeBootstrap)
			nodes = append(nodes, name)
		case "sentiment":
			name := "analyst_sentiment"
			wf.AddLambdaNode(name, compose.InvokableLambda(
				func(ctx context.Context, _ string) (string, error) {
					return "sentiment", w.runAnalystNode(ctx, w.Sentiment.Run, func(dst, src *state.AgentState) {
						dst.SentimentReport = src.SentimentReport
					})
				},
			)).AddInput(nodeBootstrap)
			nodes = append(nodes, name)
		case "news":
			name := "analyst_news"
			wf.AddLambdaNode(name, compose.InvokableLambda(
				func(ctx context.Context, _ string) (string, error) {
					return "news", w.runAnalystNode(ctx, w.News.Run, func(dst, src *state.AgentState) {
						dst.NewsReport = src.NewsReport
					})
				},
			)).AddInput(nodeBootstrap)
			nodes = append(nodes, name)
		case "technical":
			name := "analyst_technical"
			wf.AddLambdaNode(name, compose.InvokableLambda(
				func(ctx context.Context, _ string) (string, error) {
					return "technical", w.runAnalystNode(ctx, w.Technical.Run, func(dst, src *state.AgentState) {
						dst.TechnicalReport = src.TechnicalReport
					})
				},
			)).AddInput(nodeBootstrap)
			nodes = append(nodes, name)
		}
	}
	return nodes
}

// bootstrapNode 把外部传入的初始 AgentState 复制一份放进共享状态。
// 复制而不是直接持有原指针，是为了避免图内部和外部同时改同一块数据。
func (w *Workflow) bootstrapNode(ctx context.Context, input *state.AgentState) (string, error) {
	ws, err := getWorkflowState(ctx)
	if err != nil {
		return "", err
	}
	ws.initialize(input)
	return "bootstrap_ready", nil
}

// 下面这些 node 方法是一层薄封装：
// 它们不关心图编排细节，只负责调用对应业务 Agent，并把结果合并回共享状态。
func (w *Workflow) researchNode(ctx context.Context, _ string) (string, error) {
	return "research_ready", w.runStatefulNode(ctx, w.ResearchMgr.Run, func(dst, src *state.AgentState) {
		dst.BullArguments = append([]string(nil), src.BullArguments...)
		dst.BearArguments = append([]string(nil), src.BearArguments...)
		dst.ResearchSummary = src.ResearchSummary
	})
}

func (w *Workflow) traderNode(ctx context.Context, _ string) (string, error) {
	return "trader_ready", w.runStatefulNode(ctx, w.Trader.Run, func(dst, src *state.AgentState) {
		dst.TraderDecision = src.TraderDecision
	})
}

func (w *Workflow) riskAnalystNode(ctx context.Context, _ string) (string, error) {
	riskCtx := risk.WithAnalysisPeriod(ctx, w.analysisPeriod)
	return "risk_analyst_ready", w.runStatefulNode(riskCtx, w.RiskAnalyst.Run, func(dst, src *state.AgentState) {
		dst.RiskAssessmentReport = src.RiskAssessmentReport
	})
}

func (w *Workflow) riskManagerNode(ctx context.Context, _ string) (string, error) {
	return "risk_manager_ready", w.runStatefulNode(ctx, w.RiskManager.Run, func(dst, src *state.AgentState) {
		dst.RiskReview = src.RiskReview
	})
}

func (w *Workflow) portfolioNode(ctx context.Context, _ string) (string, error) {
	return "portfolio_ready", w.runStatefulNode(ctx, w.Portfolio.Run, func(dst, src *state.AgentState) {
		dst.PortfolioDecision = src.PortfolioDecision
		dst.FinalDecision = src.FinalDecision
	})
}

func (w *Workflow) executionNode(ctx context.Context, _ string) (string, error) {
	if w.Execution == nil {
		return "execution_skipped", nil
	}
	return "execution_ready", w.runStatefulNode(ctx, w.Execution.Run, func(dst, src *state.AgentState) {
		dst.ExecutionResult = src.ExecutionResult
	})
}

func (w *Workflow) finalizeNode(ctx context.Context, _ string) (*state.AgentState, error) {
	ws, err := getWorkflowState(ctx)
	if err != nil {
		return nil, err
	}
	// finalize 节点把图内最终状态导出，作为整个 Workflow 的输出。
	return ws.snapshot(), nil
}

// runAnalystNode 专门给“并行分析师节点”使用。
// 重点是：每个分析师都拿到自己的局部 AgentState 副本，避免并发写共享状态。
// 分析完成后，再把本节点负责的字段合并回共享状态。
func (w *Workflow) runAnalystNode(
	ctx context.Context,
	runFn func(context.Context, *state.AgentState) error,
	mergeFn func(dst, src *state.AgentState),
) error {
	ws, err := getWorkflowState(ctx)
	if err != nil {
		return err
	}

	localState := &state.AgentState{Symbol: ws.symbol()}
	if err := runFn(ctx, localState); err != nil {
		return err
	}

	ws.merge(func(dst *state.AgentState) {
		mergeFn(dst, localState)
	})
	return nil
}

// runStatefulNode 给顺序阶段使用，例如 Research / Trader / Risk / Portfolio。
// 这些阶段需要读到前面阶段已经写入的结果，因此先从共享状态做一份快照，再运行 Agent。
func (w *Workflow) runStatefulNode(
	ctx context.Context,
	runFn func(context.Context, *state.AgentState) error,
	mergeFn func(dst, src *state.AgentState),
) error {
	ws, err := getWorkflowState(ctx)
	if err != nil {
		return err
	}

	localState := ws.snapshot()
	if err := runFn(ctx, localState); err != nil {
		return err
	}

	ws.merge(func(dst *state.AgentState) {
		mergeFn(dst, localState)
	})
	return nil
}

// getWorkflowState 是图节点访问共享状态的统一入口。
// compose.ProcessState 会从当前 Graph/Workflow 的 local state 中安全取出 workflowState。
func getWorkflowState(ctx context.Context) (*workflowState, error) {
	var ws *workflowState
	err := compose.ProcessState[*workflowState](ctx, func(_ context.Context, state *workflowState) error {
		ws = state
		return nil
	})
	if err != nil {
		return nil, err
	}
	if ws == nil {
		return nil, fmt.Errorf("workflow state is not initialized")
	}
	return ws, nil
}

func (ws *workflowState) initialize(input *state.AgentState) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	ws.agentState = cloneAgentState(input)
}

func (ws *workflowState) symbol() string {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	if ws.agentState == nil {
		return ""
	}
	return ws.agentState.Symbol
}

// snapshot 返回当前共享状态的深拷贝。
// 调用方可以安全读取和修改这份副本，而不会破坏图中真正的共享状态。
func (ws *workflowState) snapshot() *state.AgentState {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	return cloneAgentState(ws.agentState)
}

// merge 是唯一允许修改共享状态的入口之一。
// 外部传入一个小的合并函数，把局部节点结果精确写回共享状态。
func (ws *workflowState) merge(fn func(*state.AgentState)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.agentState == nil {
		ws.agentState = &state.AgentState{}
	}
	fn(ws.agentState)
}

// cloneAgentState 做最小必要的深拷贝。
// 目前只有 BullArguments / BearArguments 是 slice，需要额外复制底层数组；
// 其余字段是值类型或字符串，直接结构体拷贝即可。
func cloneAgentState(in *state.AgentState) *state.AgentState {
	if in == nil {
		return &state.AgentState{}
	}
	cloned := *in
	cloned.BullArguments = append([]string(nil), in.BullArguments...)
	cloned.BearArguments = append([]string(nil), in.BearArguments...)
	cloned.ResearchDebateTurns = append([]state.ResearchDebateTurn(nil), in.ResearchDebateTurns...)
	return &cloned
}
