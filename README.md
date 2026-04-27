# trading-go

基于 Go 实现的多 Agent 股票交易分析框架，参考 TradingAgents 的五层协作模型，当前已具备从分析、辩论、交易决策、风控审核到基金经理最终审批的一条完整运行链路。

## 架构概览

系统按五层协作模型组织：

1. `Analyst Team`
   - `FundamentalAnalystAgent`
   - `SentimentAnalystAgent`
   - `NewsAnalystAgent`
   - `TechnicalAnalystAgent`
2. `Research Team`
   - `BullResearcherAgent`
   - `BearResearcherAgent`
   - `ResearchManagerAgent`
3. `Trader Agent`
4. `Risk Team`
   - `RiskAnalystAgent`
   - `RiskManagerAgent`
5. `PortfolioManager Agent`

编排层位于 `orchestrator/`，当前使用 `compose.Workflow` 把五层串成：

`Analyst Team (parallel) -> Research -> Trader -> Risk -> PortfolioManager`

运行过程中会生成：
- 结构化日志：`logs/trading-go.log`
- 按 trace 拆分的日志：`logs/traces/`
- Markdown 报告归档：`reports/<trace_id>-<symbol>/`

## 目录说明

- `agents/`
  业务 Agent 实现，按 analyst / research / trader / risk / portfolio 划分。
- `orchestrator/`
  五层工作流编排。
- `tools/`
  统一工具入口。
- `tools/core/`
  共享类型、provider 接口、错误定义。
- `tools/marketdata/longbridge/`
  长桥行情、持仓、历史成交、下单/撤单实现。
- `tools/technical/`
  技术指标、技术快照、连续指标序列。
- `internal/prompts/`
  统一管理各类 Agent prompt。
- `internal/logx/`
  日志初始化、trace log hook。
- `internal/reports/`
  报告持久化与合并索引。
- `state/`
  `AgentState` 和跨节点累积状态。
- `config/`
  `viper` 配置加载。

## 已实现能力

- 四层以上主链路打通：分析 -> 研究辩论 -> 交易员 -> 风控 -> 基金经理
- Technical analyst 使用真实 K 线 + 本地技术指标计算
- Research team 支持多轮多空辩论，并保留辩论链
- Trader 输入包含研究总结和历史交易记录
- Risk 输入包含交易员决策、当前持仓和市场数据
- Portfolio manager 输入包含交易员决策、风控审核结论、风险报告和账户快照
- Longbridge OpenAPI 接入：
  - K 线行情
  - 当前持仓 / 账户快照
  - 历史成交记录 / 当日成交记录
  - 下单 / 撤单
- 报告按阶段持久化为 Markdown，方便后续整理和复盘

## 运行前准备

### 1. 配置 `.env`

复制模板并填写你自己的本地凭证：

```bash
cp .env.example .env
```

关键配置：

```env
LONGBRIDGE_APP_KEY=
LONGBRIDGE_APP_SECRET=
LONGBRIDGE_ACCOUNT_MODE=paper
LONGBRIDGE_ACCESS_TOKEN_PAPER=
LONGBRIDGE_ACCESS_TOKEN_LIVE=
```

说明：
- `LONGBRIDGE_APP_KEY` / `LONGBRIDGE_APP_SECRET`：模拟仓和实盘共用
- `LONGBRIDGE_ACCOUNT_MODE`：`paper` 或 `live`
- 程序会优先读取对应模式的 token
- 若模式 token 为空，则回退到 `LONGBRIDGE_ACCESS_TOKEN`

### 2. 配置 `config.yaml`

示例字段：

```yaml
symbol: NVDA
debate_rounds: 2
loop_interval_seconds: 1800
mode: single

analyst_config:
  enabled: [technical]

market_data:
  provider: auto
  default_region: US

log_config:
  level: info
  path: logs/trading-go.log
  trace_dir: logs/traces

llm_config:
  provider: deepseek
  model: deepseek-v4-flash
  api_key: YOUR_KEY
  base_url: https://api.deepseek.com/v1
  temperature: 0.2
```

## 运行方式

### 单次分析

```bash
go run .
```

### 定时循环分析

把 `config.yaml` 中的 `mode` 改成：

```yaml
mode: loop
```

程序会按 `loop_interval_seconds` 周期重新分析。

## 输出结果

运行一次后，你通常会看到三类结果：

1. 控制台 / 总日志
   - `logs/trading-go.log`
2. 单次 trace 日志
   - `logs/traces/<trace_id>.log`
3. 报告归档
   - `reports/<trace_id>-<symbol>/00-index.md`
   - `reports/<trace_id>-<symbol>/combined.md`
   - `reports/<trace_id>-<symbol>/analysts/...`
   - `reports/<trace_id>-<symbol>/research/...`
   - `reports/<trace_id>-<symbol>/trading/...`
   - `reports/<trace_id>-<symbol>/risk/...`
   - `reports/<trace_id>-<symbol>/portfolio/...`

## 开发与测试

运行全量测试：

```bash
GOCACHE=$(pwd)/.gocache go test ./...
```

如果只想看某个模块：

```bash
GOCACHE=$(pwd)/.gocache go test ./agents/...
GOCACHE=$(pwd)/.gocache go test ./tools/...
```

## 当前已知限制

- `GetFinancialReports` / `GetSocialSentiment` / `GetNews` 仍未接入真实 provider，实现上还有继续补齐空间
- 状态恢复与外部持久化（如 Redis checkpoint）尚未完成
- 当前编排使用 `compose.Workflow`，不是完全按 PRD 指定的 ADK `Parallel/Loop/Sequential Agent` 组合实现

## 扩展建议

- 为 fundamental / sentiment / news 增加真实或 mock provider，补齐四分析师默认可运行能力
- 接入 checkpoint store，补断点恢复
- 将交易能力进一步封装成 Agent 可直接调用的 Eino tools
- 给 PortfolioManager 输出增加结构化执行计划，便于后续自动下单
