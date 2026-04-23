# trading-go

基于 Go 实现的多 Agent 股票交易分析框架骨架，参考 TradingAgents 的五层协作模型。

## 当前骨架

- `agents/`：分析师、研究员、交易员、风控、基金经理 Agent 占位实现
- `orchestrator/`：工作流编排占位（后续替换为 Eino ADK Graph/Supervisor）
- `tools/`：模拟数据工具与技术指标函数占位
- `state/`：`AgentState` 与决策结构体
- `config/`：配置加载（Viper）
- `main.go`：单次模式 / 循环模式入口与优雅退出

## 快速开始

1. 修改 `config.yaml` 中的参数（`symbol`、`debate_rounds`、`loop_interval_seconds`、`llm_config`）。
2. 运行：

```bash
go run .
```

默认执行单次分析；将 `mode` 设置为 `loop` 可进入定时监听模式。

## 后续扩展建议

- 将 `orchestrator` 切换为 Eino ADK 的 Parallel / Loop / Sequential 组合编排。
- 为每个 Agent 增加独立 Prompt、LLM 调用和降级策略。
- 将 `tools/` 的模拟实现替换为真实行情 / 财务 / 新闻数据源。
- 在 `state/` 中接入持久化（如 Redis）和断点恢复机制。
