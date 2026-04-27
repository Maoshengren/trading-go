# trading-go

`trading-go` 是一个 Go 实现的多 Agent 交易分析与执行框架，参考 TradingAgents 的分层协作模型，当前主要面向 `BTCUSDT` 等 Binance USDⓈ-M Futures testnet 场景，也保留 Longbridge 股票/基金账户能力。

系统已经打通从分析、研究辩论、交易建议、风控审核、基金经理审批到订单执行的完整链路。

## 架构概览

当前主流程由 `orchestrator/` 中的 `compose.Workflow` 编排：

```text
Analyst Team -> Research Team -> Trader -> Risk Team -> PortfolioManager -> ExecutionAgent -> Report
```

各阶段职责：

- `Analyst Team`：生成分析报告。当前最完整的是 `TechnicalAnalystAgent`，支持多周期 K 线和本地技术指标。
- `Research Team`：`BullResearcherAgent` 与 `BearResearcherAgent` 多轮辩论，`ResearchManagerAgent` 汇总研究结论。
- `TraderAgent`：基于研究总结、分析报告和历史成交记录输出结构化交易建议。
- `RiskAnalystAgent`：基于交易员建议、当前持仓和市场数据输出风险评估报告。
- `RiskManagerAgent`：输出 `approve / modify / reject` 风控审核结论。
- `PortfolioManagerAgent`：最终审批，输出明确的结构化执行计划。
- `ExecutionAgent`：按最终审批计划执行下单、减仓、止盈止损保护单或 dry-run 计划。

最终真正拍板的是 `PortfolioManagerAgent`；`ExecutionAgent` 不再问大模型，只做规则化执行。

## 决策格式

`TraderAgent` 输出交易建议：

```json
{
  "direction": "buy|sell|hold",
  "strength": 1,
  "confidence": 80,
  "position_size": 0.2,
  "reasoning": "..."
}
```

`RiskManagerAgent` 输出风控审核：

```json
{
  "conclusion": "approve|modify|reject",
  "suggestion": "..."
}
```

`PortfolioManagerAgent` 输出最终执行计划：

```json
{
  "execution": "执行|拒绝|调整后执行",
  "action": "open|increase|reduce|close|hold|reject",
  "direction": "buy|sell|hold",
  "approved_position_size": 0.1,
  "approved_position_ratio": 0.1,
  "target_position_qty": 0.1,
  "summary": "最终执行结论与签字记录"
}
```

字段含义：

- `execution`：审批结论。
- `action`：具体动作，例如开仓、加仓、减仓、平仓、拒绝。
- `direction`：订单方向，合约减空仓通常是 `buy`，减多仓通常是 `sell`。
- `approved_position_ratio`：批准的账户仓位比例。
- `target_position_qty`：合约目标持仓数量，例如 `BTCUSDT` 减仓到 `0.10 BTC`。
- `approved_position_size`：兼容字段，新逻辑优先使用 `target_position_qty` 和 `approved_position_ratio`。

`ExecutionAgent` 输出执行结果，包含订单 ID、数量、方向、reduce-only、止盈止损订单状态等。

## 目录结构

- `agents/analyst/`
  分析师实现。`technical.go` 负责多周期 K 线、技术指标 payload 和技术报告。
- `agents/research/`
  多空研究员和研究经理，包含辩论上下文与轮次记录。
- `agents/trader/`
  交易员，输出结构化交易建议。
- `agents/risk/`
  风险分析师与风险管理器。
- `agents/portfolio/`
  基金经理最终审批。
- `agents/execution/`
  订单执行代理，负责真实下单、减仓、止盈止损保护单和 dry-run。
- `orchestrator/`
  `compose.Workflow` 编排入口。
- `state/`
  `AgentState` 以及各阶段共享状态结构。
- `internal/prompts/`
  各 Agent prompt 统一管理。
- `internal/logx/`
  结构化日志和 trace log hook。
- `internal/reports/`
  Markdown 报告持久化、索引和合并报告。
- `tools/core/`
  Provider 接口、通用类型和错误定义。
- `tools/marketdata/binance/`
  Binance USDⓈ-M Futures 行情、持仓、历史成交、下单、撤单、调杠杆实现。
- `tools/marketdata/longbridge/`
  Longbridge 行情、账户、持仓、成交和交易接口。
- `tools/technical/`
  本地技术指标、技术快照、连续指标序列。
- `tools/`
  对业务侧暴露的统一 facade。

## 配置

复制模板：

```bash
cp .env.example .env
cp config.yaml.example config.yaml
```

`.env` 保存本地凭证，不进入 git。只跑公开行情和技术分析时，Binance Futures 私有 key 可以为空；需要持仓、历史成交、下单、撤单、调杠杆时需要配置：

```env
BINANCE_FUTURES_API_KEY=
BINANCE_FUTURES_API_SECRET=
BINANCE_FUTURES_TESTNET=true
BINANCE_DEFAULT_QUOTE_ASSET=USDT
```

`config.yaml` 也不进入 git。当前推荐的 BTC 合约多周期配置：

```yaml
symbol: BTCUSDT
mode: single
loop_interval_seconds: 900

analyst_config:
  enabled: [technical]

market_data:
  provider: binance_futures
  default_region: US
  kline_timeframes:
    - name: execution
      period: 15m
      bars: 192
      recent_bars: 96
      indicator_bars: 32
    - name: trend
      period: 1h
      bars: 240
      recent_bars: 96
      indicator_bars: 40
    - name: macro
      period: 4h
      bars: 180
      recent_bars: 72
      indicator_bars: 40
```

第一条 `kline_timeframes[0]` 是主交易周期，后续周期只作为趋势过滤和关键价位参考。系统不再使用独立 `analysis_period`。

执行配置：

```yaml
execution_config:
  enabled: true
  dry_run: true
  order_type: market
  time_in_force: GTC
  leverage: 3
  price_period: 1m
  protective_orders_enabled: true
  take_profit_percent: 0.015
  stop_loss_percent: 0.008
```

`dry_run: false` 会真实调用交易接口。建议先在 Binance Futures testnet 验证。

## 运行

推荐使用 Makefile。`make run` 和 `make start` 会以 `output/` 作为运行工作目录，因此日志和报告都会写到 `output/` 下。

```bash
make build
make run
```

后台运行：

```bash
make start
make status
make logs
make stop
```

Makefile 行为：

- 编译产物：`output/trading-go`
- 运行配置：启动前复制 `config.yaml` 到 `output/config.yaml`
- stdout/stderr：`output/trading-go.stdout.log`
- pid 文件：`output/trading-go.pid`
- 应用日志：`output/logs/`
- Markdown 报告：`output/reports/`

如果直接执行 `go run .`，相对路径会落在项目根目录，例如 `logs/` 和 `reports/`。交付和长期运行建议使用 `make start`。

## 输出

每次 workflow 会生成：

- 总日志：`output/logs/trading-go.log`
- trace 日志：`output/logs/traces/<trace_id>.log`
- 报告目录：`output/reports/<trace_id>-<symbol>/`

报告目录通常包含：

- `00-index.md`
- `combined.md`
- `analysts/`
- `research/`
- `trading/`
- `risk/`
- `portfolio/`
- `execution/`

`combined.md` 适合快速复盘整轮 agent 链路。

## 开发

全量测试：

```bash
make test
```

或直接运行：

```bash
GOCACHE=$(pwd)/.gocache go test ./...
```

常用模块测试：

```bash
GOCACHE=$(pwd)/.gocache go test ./agents/...
GOCACHE=$(pwd)/.gocache go test ./tools/...
GOCACHE=$(pwd)/.gocache go test ./tools/marketdata/binance
```

## 当前限制

- `FundamentalAnalystAgent`、`SentimentAnalystAgent`、`NewsAnalystAgent` 的真实 provider 仍未完整接入。
- 状态恢复和 checkpoint 还没有落地，当前每轮是独立运行。
- 顶层编排使用 `compose.Workflow`，不是完整 ADK `Parallel / Loop / Sequential Agent` 组合。
- 自动执行已经支持 Binance Futures testnet，但真实交易前仍需要额外风控保护，例如最大单笔 notional、最大日内下单次数、正式环境保护开关。
