# 任务目标
请参考 TradingAgents 项目（https://github.com/TauricResearch/TradingAgents）的多 Agent 协作交易架构，用 Go 语言实现一个类似的多 Agent 股票交易分析框架。

# 核心架构参照（TradingAgents 五层协作模型）
TradingAgents 将交易决策拆解为五个协作层：
1. 分析师团队：并行执行基本面、情绪、新闻、技术分析
2. 研究团队：多头 vs 空头辩论式研究
3. 交易员：综合决策并输出结构化交易建议
4. 风控团队：独立审核交易风险
5. 基金经理：最终审批执行

# 技术要求
- Go 1.21+
- 多 Agent 框架：github.com/cloudwego/eino (ADK 模块，v0.5.0+)
- 技术指标库：github.com/sdcoffey/techan
- 数据获取：先用模拟数据生成器，预留真实 API 接口（如 Yahoo Finance）
- 日志：logrus
- 配置：viper

# 功能模块（必须清晰划分）

## 1. Agent 定义层（agents/）
### 1.1 Analyst Agents（分析师团队，4个独立 Agent，并行执行）
- **FundamentalAnalystAgent**（基本面分析师）
    - 输入：股票代码
    - 工具：GetFinancialReports(symbol) -> 资产负债表、利润表、现金流量表
    - 输出：基本面分析报告（含估值判断、盈利趋势）
- **SentimentAnalystAgent**（情绪分析师）
    - 输入：股票代码
    - 工具：GetSocialSentiment(symbol) -> 社交媒体情绪评分
    - 输出：情绪分析报告（FOMO/恐慌/中性）
- **NewsAnalystAgent**（新闻分析师）
    - 输入：股票代码
    - 工具：GetNews(symbol, days) -> 新闻列表（支持时间分层：近3天权重最高，近30天权重最低）
    - 输出：新闻分析报告（事件冲击方向与强度）
- **TechnicalAnalystAgent**（技术分析师）
    - 输入：股票代码
    - 工具：GetKline(symbol, period) -> K线数据
    - 工具：CalculateRSI, CalculateMACD, CalculateBollingerBands（使用 techan 库）
    - 输出：技术分析报告（支撑位/阻力位/趋势信号）

### 1.2 Research Agents（研究团队，多空辩论）
- **BullResearcherAgent**（多头研究员）
    - 输入：四份分析师报告 + 股票代码
    - 输出：看涨论点（成长性、低估值、利好催化剂）
- **BearResearcherAgent**（空头研究员）
    - 输入：四份分析师报告 + 股票代码
    - 输出：看跌论点（风险因素、估值泡沫、潜在利空）
- **ResearchManagerAgent**（研究经理）
    - 职责：控制辩论轮数（可配置，默认2轮），综合多空观点，输出辩论总结

### 1.3 Trader Agent（交易员）
- 输入：多空辩论总结 + 历史交易记录
- 输出：结构化交易决策，包含：
    - direction: buy/sell/hold
    - strength: 1-10（信号强度）
    - confidence: 0-100（置信度）
    - position_size: 建议仓位（百分比）
    - reasoning: 决策依据

### 1.4 Risk Agent（风控团队）
- **RiskAnalystAgent**（风险分析师）
    - 输入：交易员决策 + 当前持仓 + 市场数据
    - 输出：风险评估报告（波动率/流动性/组合敞口/最大回撤预测）
- **RiskManagerAgent**（风险管理器）
    - 输入：风险评估报告 + 交易员决策
    - 输出：审核结论（approve/ modify / reject），若修改则给出修改建议

### 1.5 PortfolioManager Agent（基金经理）
- 输入：交易员决策 + 风控审核结论
- 输出：最终执行决定（执行/拒绝/调整后执行）+ 签字记录

## 2. 编排层（orchestrator/）
- 使用 Eino ADK 的 Graph 或 Supervisor 模式来编排上述 8 个 Agent
- 实现工作流：Analyst Team（并行）→ Research Team（辩论循环）→ Trader → Risk Team → Portfolio Manager
- 使用 Eino 的 Parallel Agent 实现分析师团队的并行执行
- 使用 Eino 的 Loop Agent 实现研究团队的多轮辩论
- 使用 Eino 的 Sequential Agent 实现层间串联
- 实现 AgentState 结构体，在各节点间传递累积的分析报告

## 3. 工具层（tools/）
- GetFinancialReports(symbol) -> 模拟财务数据（后续对接 FinnHub 或 Tushare）
- GetSocialSentiment(symbol) -> 模拟情绪数据（后续对接真实 API）
- GetNews(symbol, days) -> 模拟新闻数据（支持时间分层）
- GetKline(symbol, period) -> 模拟 K 线数据（后续对接真实 API）
- 技术指标计算：使用 techan 库

## 4. 状态管理层（state/）
- 定义 AgentState 结构体，包含所有分析报告、辩论记录、决策记录
- 支持状态持久化（可选：接入 Redis）
- 支持断点恢复

## 5. 主程序（main.go）
- 初始化配置（股票代码、辩论轮数、LLM 模型配置等）
- 构建 Eino Agent 工作流
- 支持两种运行模式：
    - 单次分析模式：执行一次完整分析并输出决策
    - 定时监听模式：每隔 N 秒获取最新 K 线，触发重新分析（实现 Agent Loop）
- 优雅退出（监听 SIGINT）

# 输出要求
- 生成完整的项目结构，包含上述所有 .go 文件
- 提供 go.mod 文件，包含所有依赖
- 提供 config.yaml 示例，包含：symbol, debate_rounds, loop_interval_seconds, llm_config (model, api_key)
- 提供 README.md，说明架构设计、运行方式和扩展指南

# 代码质量要求
- 使用 Eino ADK 的 Agent 抽象（实现 Name、Description、Run 方法）
- 每个 Agent 有独立的 System Prompt（定义角色行为）
- 使用 Eino 的工具抽象（Tool 接口）封装数据获取
- 错误处理完善，LLM 调用失败时降级处理
- 日志结构化，便于追踪决策链路

# 参考资源
- TradingAgents 原版架构：https://github.com/TauricResearch/TradingAgents
- Eino ADK 文档：https://www.cloudwego.io/zh/docs/eino/core_modules/eino_adk/
- Eino ADK 示例：https://github.com/cloudwego/eino-examples/tree/main/adk