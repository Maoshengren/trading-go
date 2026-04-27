package analyst

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	toolcomponent "github.com/cloudwego/eino/components/tool"
	"github.com/sirupsen/logrus"

	"trading-go/internal/logx"
	"trading-go/internal/prompts"
	"trading-go/state"
	"trading-go/tools"
)

const technicalPayloadRecentBars = 120
const technicalPayloadIndicatorSeries = 20

type technicalPayload struct {
	Symbol          string                    `json:"symbol"`
	AnalysisPeriod  string                    `json:"analysis_period"`
	Snapshot        *tools.TechnicalSnapshot  `json:"snapshot"`
	RecentBars      []technicalPayloadBar     `json:"recent_bars"`
	IndicatorSeries []technicalIndicatorPoint `json:"indicator_series"`
}

type technicalPayloadBar struct {
	Time   string  `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

type technicalIndicatorPoint struct {
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

// TechnicalAnalystAgent 负责技术面分析：从 K 线与指标计算得到交易信号描述。
type TechnicalAnalystAgent struct {
	agent          adk.Agent
	boundTools     []toolcomponent.BaseTool
	systemPrompt   string
	analysisPeriod string
}

// NewTechnicalAnalystAgent 创建技术分析 Agent，并绑定行情与指标工具。
func NewTechnicalAnalystAgent(period string) (*TechnicalAnalystAgent, error) {
	klineTool, err := buildKlineTool()
	if err != nil {
		return nil, err
	}
	indicatorsTool, err := buildTechnicalIndicatorsTool()
	if err != nil {
		return nil, err
	}
	ts := []toolcomponent.BaseTool{klineTool, indicatorsTool}
	agent, err := newReportADKAgent("TechnicalAnalystAgent", "Analyze price trend and technical signals.", prompts.TechnicalAnalystSystem, ts)
	if err != nil {
		return nil, err
	}
	return &TechnicalAnalystAgent{
		agent:          agent,
		boundTools:     ts,
		systemPrompt:   prompts.TechnicalAnalystSystem,
		analysisPeriod: normalizeAnalysisPeriod(period),
	}, nil
}

func (a *TechnicalAnalystAgent) Name() string { return "TechnicalAnalystAgent" }

func (a *TechnicalAnalystAgent) Description() string {
	return "Analyze price trend and technical signals."
}

func (a *TechnicalAnalystAgent) SystemPrompt() string { return a.systemPrompt }

func (a *TechnicalAnalystAgent) Tools() []toolcomponent.BaseTool {
	return a.boundTools
}

func (a *TechnicalAnalystAgent) SetAnalysisPeriod(period string) {
	a.analysisPeriod = normalizeAnalysisPeriod(period)
}

// Run 执行技术分析：拉取 K 线、计算指标、调用 ADK 生成报告、回写状态。
func (a *TechnicalAnalystAgent) Run(ctx context.Context, s *state.AgentState) error {
	if err := a.ensureInitialized(); err != nil {
		return err
	}

	klines, snapshot, err := a.loadSnapshot(ctx, s.Symbol)
	if err != nil {
		return err
	}

	logx.Logger(ctx).WithFields(logrus.Fields{"agent": a.Name(), "symbol": s.Symbol, "kline_count": len(klines)}).Info("start running technical analysis")

	payload, err := buildTechnicalPayload(s.Symbol, a.analysisPeriod, klines, snapshot)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Error("failed to build technical payload")
		return err
	}
	out, err := runADKReport(ctx, a.agent, payload)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Error("failed to generate technical report")
		return err
	}

	s.TechnicalReport = out
	return nil
}

func (a *TechnicalAnalystAgent) ensureInitialized() error {
	if a.agent != nil {
		if a.analysisPeriod == "" {
			a.analysisPeriod = normalizeAnalysisPeriod("")
		}
		return nil
	}
	b, err := NewTechnicalAnalystAgent(a.analysisPeriod)
	if err != nil {
		return err
	}
	*a = *b
	return nil
}

func (a *TechnicalAnalystAgent) loadSnapshot(ctx context.Context, symbol string) ([]tools.KLine, *tools.TechnicalSnapshot, error) {
	klines, err := tools.GetKline(symbol, a.analysisPeriod)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Error("failed to fetch klines")
		return nil, nil, err
	}

	snapshot, err := tools.BuildTechnicalSnapshot(klines)
	if err != nil {
		logx.Logger(ctx).WithError(err).WithField("agent", a.Name()).Error("failed to build technical snapshot")
		return nil, nil, err
	}
	return klines, snapshot, nil
}

func buildTechnicalPayload(symbol, period string, klines []tools.KLine, snapshot *tools.TechnicalSnapshot) (string, error) {
	indicatorSeries, err := tools.BuildTechnicalIndicatorSeries(klines, technicalPayloadIndicatorSeries)
	if err != nil {
		return "", fmt.Errorf("build technical indicator series: %w", err)
	}
	payload := technicalPayload{
		Symbol:          strings.ToUpper(strings.TrimSpace(symbol)),
		AnalysisPeriod:  period,
		Snapshot:        snapshot,
		RecentBars:      buildRecentTechnicalBars(klines, technicalPayloadRecentBars),
		IndicatorSeries: buildTechnicalIndicatorPayloadSeries(indicatorSeries),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal technical payload: %w", err)
	}
	return string(data), nil
}

func normalizeAnalysisPeriod(period string) string {
	period = strings.ToLower(strings.TrimSpace(period))
	if period == "" {
		return "1d"
	}
	return period
}

func buildRecentTechnicalBars(klines []tools.KLine, limit int) []technicalPayloadBar {
	if limit <= 0 || len(klines) == 0 {
		return nil
	}
	start := len(klines) - limit
	if start < 0 {
		start = 0
	}

	bars := make([]technicalPayloadBar, 0, len(klines)-start)
	for _, k := range klines[start:] {
		bars = append(bars, technicalPayloadBar{
			Time:   k.Time.UTC().Format(time.RFC3339),
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  k.Close,
			Volume: k.Volume,
		})
	}
	return bars
}

func buildTechnicalIndicatorPayloadSeries(points []tools.TechnicalIndicatorPoint) []technicalIndicatorPoint {
	if len(points) == 0 {
		return nil
	}
	items := make([]technicalIndicatorPoint, 0, len(points))
	for _, p := range points {
		items = append(items, technicalIndicatorPoint{
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
	return items
}
