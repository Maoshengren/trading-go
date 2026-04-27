package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type LLMConfig struct {
	Provider    string  `mapstructure:"provider"`
	Model       string  `mapstructure:"model"`
	APIKey      string  `mapstructure:"api_key"`
	BaseURL     string  `mapstructure:"base_url"`
	Temperature float32 `mapstructure:"temperature"`
}

type MarketDataConfig struct {
	Provider        string                 `mapstructure:"provider"`
	DefaultRegion   string                 `mapstructure:"default_region"`
	KLineTimeframes []KLineTimeframeConfig `mapstructure:"kline_timeframes"`
}

type KLineTimeframeConfig struct {
	Name          string `mapstructure:"name"`
	Period        string `mapstructure:"period"`
	Bars          int    `mapstructure:"bars"`
	RecentBars    int    `mapstructure:"recent_bars"`
	IndicatorBars int    `mapstructure:"indicator_bars"`
}

type LogConfig struct {
	Level    string `mapstructure:"level"`
	Path     string `mapstructure:"path"`
	TraceDir string `mapstructure:"trace_dir"`
}

type AnalystConfig struct {
	Enabled []string `mapstructure:"enabled"`
}

type ExecutionConfig struct {
	Enabled                 bool    `mapstructure:"enabled"`
	DryRun                  bool    `mapstructure:"dry_run"`
	OrderType               string  `mapstructure:"order_type"`
	TimeInForce             string  `mapstructure:"time_in_force"`
	Leverage                int     `mapstructure:"leverage"`
	PricePeriod             string  `mapstructure:"price_period"`
	ProtectiveOrdersEnabled bool    `mapstructure:"protective_orders_enabled"`
	TakeProfitPercent       float64 `mapstructure:"take_profit_percent"`
	StopLossPercent         float64 `mapstructure:"stop_loss_percent"`
}

type Config struct {
	Symbol              string           `mapstructure:"symbol"`
	DebateRounds        int              `mapstructure:"debate_rounds"`
	LoopIntervalSeconds int              `mapstructure:"loop_interval_seconds"`
	Mode                string           `mapstructure:"mode"`
	Analysts            AnalystConfig    `mapstructure:"analyst_config"`
	Execution           ExecutionConfig  `mapstructure:"execution_config"`
	MarketData          MarketDataConfig `mapstructure:"market_data"`
	Log                 LogConfig        `mapstructure:"log_config"`
	LLM                 LLMConfig        `mapstructure:"llm_config"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetDefault("symbol", "AAPL")
	v.SetDefault("debate_rounds", 2)
	v.SetDefault("loop_interval_seconds", 60)
	v.SetDefault("mode", "single")
	v.SetDefault("analyst_config.enabled", []string{"fundamental", "sentiment", "news", "technical"})
	v.SetDefault("execution_config.enabled", true)
	v.SetDefault("execution_config.dry_run", false)
	v.SetDefault("execution_config.order_type", "market")
	v.SetDefault("execution_config.time_in_force", "GTC")
	v.SetDefault("execution_config.leverage", 1)
	v.SetDefault("execution_config.price_period", "1m")
	v.SetDefault("execution_config.protective_orders_enabled", true)
	v.SetDefault("execution_config.take_profit_percent", 0.015)
	v.SetDefault("execution_config.stop_loss_percent", 0.008)
	v.SetDefault("market_data.provider", "auto")
	v.SetDefault("market_data.default_region", "US")
	v.SetDefault("market_data.kline_timeframes", []map[string]any{
		{"name": "execution", "period": "15m", "bars": 120, "recent_bars": 80, "indicator_bars": 20},
		{"name": "trend", "period": "1h", "bars": 240, "recent_bars": 80, "indicator_bars": 30},
		{"name": "macro", "period": "4h", "bars": 180, "recent_bars": 60, "indicator_bars": 30},
	})
	v.SetDefault("log_config.level", "info")
	v.SetDefault("log_config.path", "logs/trading-go.log")
	v.SetDefault("log_config.trace_dir", "logs/traces")
	v.SetDefault("llm_config.provider", "openai")
	v.SetDefault("llm_config.model", "gpt-4o-mini")
	v.SetDefault("llm_config.temperature", float32(0.2))

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	cfg.Analysts.Enabled = normalizeAnalystNames(cfg.Analysts.Enabled)
	return &cfg, nil
}

func normalizeAnalystNames(items []string) []string {
	if len(items) == 0 {
		return []string{"fundamental", "sentiment", "news", "technical"}
	}

	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		name := strings.ToLower(strings.TrimSpace(item))
		switch name {
		case "fundamental", "sentiment", "news", "technical":
		default:
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}

	if len(result) == 0 {
		return []string{"technical"}
	}
	return result
}
