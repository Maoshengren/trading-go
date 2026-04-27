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

type KLineTimeframeConfig struct {
	Name          string
	Period        string
	Bars          int
	RecentBars    int
	IndicatorBars int
}

type technicalPayload struct {
	Symbol     string               `json:"symbol"`
	Timeframes []technicalTimeframe `json:"timeframes"`
}

type technicalTimeframe struct {
	Name            string                    `json:"name"`
	Period          string                    `json:"period"`
	BarCount        int                       `json:"bar_count"`
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
	agent        adk.Agent
	boundTools   []toolcomponent.BaseTool
	systemPrompt string
	timeframes   []KLineTimeframeConfig
}

// NewTechnicalAnalystAgent 创建技术分析 Agent，并绑定行情与指标工具。
func NewTechnicalAnalystAgent(timeframes []KLineTimeframeConfig) (*TechnicalAnalystAgent, error) {
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
		agent:        agent,
		boundTools:   ts,
		systemPrompt: prompts.TechnicalAnalystSystem,
		timeframes:   normalizeKLineTimeframes(timeframes),
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

func (a *TechnicalAnalystAgent) SetTimeframes(timeframes []KLineTimeframeConfig) {
	a.timeframes = normalizeKLineTimeframes(timeframes)
}

// Run 执行技术分析：拉取 K 线、计算指标、调用 ADK 生成报告、回写状态。
func (a *TechnicalAnalystAgent) Run(ctx context.Context, s *state.AgentState) error {
	if err := a.ensureInitialized(); err != nil {
		return err
	}

	timeframes, err := a.loadTimeframes(ctx, s.Symbol)
	if err != nil {
		return err
	}
	primary := timeframes[0]

	logx.Logger(ctx).WithFields(logrus.Fields{
		"agent":           a.Name(),
		"symbol":          s.Symbol,
		"kline_count":     primary.BarCount,
		"timeframe_count": len(timeframes),
		"primary_period":  primary.Period,
	}).Info("start running technical analysis")

	payload, err := buildTechnicalPayload(s.Symbol, timeframes)
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
		a.timeframes = normalizeKLineTimeframes(a.timeframes)
		return nil
	}
	b, err := NewTechnicalAnalystAgent(a.timeframes)
	if err != nil {
		return err
	}
	*a = *b
	return nil
}

func (a *TechnicalAnalystAgent) loadTimeframes(ctx context.Context, symbol string) ([]technicalTimeframe, error) {
	configs := normalizeKLineTimeframes(a.timeframes)
	out := make([]technicalTimeframe, 0, len(configs))
	for _, cfg := range configs {
		// Each configured timeframe is fetched and summarized independently.
		// The first item is the primary trading timeframe; later items provide trend context.
		klines, err := tools.GetKlineWithLimit(symbol, cfg.Period, cfg.Bars)
		if err != nil {
			logx.Logger(ctx).WithError(err).WithFields(logrus.Fields{"agent": a.Name(), "period": cfg.Period}).Error("failed to fetch klines")
			return nil, err
		}
		snapshot, err := tools.BuildTechnicalSnapshot(klines)
		if err != nil {
			logx.Logger(ctx).WithError(err).WithFields(logrus.Fields{"agent": a.Name(), "period": cfg.Period}).Error("failed to build technical snapshot")
			return nil, err
		}
		indicatorSeries, err := tools.BuildTechnicalIndicatorSeries(klines, cfg.IndicatorBars)
		if err != nil {
			return nil, fmt.Errorf("build technical indicator series for %s: %w", cfg.Period, err)
		}
		out = append(out, technicalTimeframe{
			Name:            cfg.Name,
			Period:          cfg.Period,
			BarCount:        len(klines),
			Snapshot:        snapshot,
			RecentBars:      buildRecentTechnicalBars(klines, cfg.RecentBars),
			IndicatorSeries: buildTechnicalIndicatorPayloadSeries(indicatorSeries),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no technical timeframes configured")
	}
	return out, nil
}

func buildTechnicalPayload(symbol string, timeframes []technicalTimeframe) (string, error) {
	if len(timeframes) == 0 {
		return "", fmt.Errorf("no technical timeframes provided")
	}
	payload := technicalPayload{
		Symbol:     strings.ToUpper(strings.TrimSpace(symbol)),
		Timeframes: timeframes,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal technical payload: %w", err)
	}
	return string(data), nil
}

func normalizeKLinePeriod(period string) string {
	period = strings.ToLower(strings.TrimSpace(period))
	if period == "" {
		return "1d"
	}
	return period
}

func defaultTechnicalTimeframes() []KLineTimeframeConfig {
	return []KLineTimeframeConfig{{
		Name:          "execution",
		Period:        "15m",
		Bars:          technicalPayloadRecentBars,
		RecentBars:    technicalPayloadRecentBars,
		IndicatorBars: technicalPayloadIndicatorSeries,
	}}
}

func normalizeKLineTimeframes(items []KLineTimeframeConfig) []KLineTimeframeConfig {
	if len(items) == 0 {
		return defaultTechnicalTimeframes()
	}
	out := make([]KLineTimeframeConfig, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		period := normalizeKLinePeriod(item.Period)
		if _, ok := seen[period]; ok {
			continue
		}
		seen[period] = struct{}{}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			if i == 0 {
				name = "primary"
			} else {
				name = period
			}
		}
		bars := item.Bars
		if bars <= 0 {
			bars = technicalPayloadRecentBars
		}
		recentBars := item.RecentBars
		if recentBars <= 0 || recentBars > bars {
			recentBars = minInt(bars, technicalPayloadRecentBars)
		}
		indicatorBars := item.IndicatorBars
		if indicatorBars <= 0 || indicatorBars > bars {
			indicatorBars = minInt(bars, technicalPayloadIndicatorSeries)
		}
		out = append(out, KLineTimeframeConfig{
			Name:          name,
			Period:        period,
			Bars:          bars,
			RecentBars:    recentBars,
			IndicatorBars: indicatorBars,
		})
	}
	if len(out) == 0 {
		return defaultTechnicalTimeframes()
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
