package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type LLMConfig struct {
	Model  string `mapstructure:"model"`
	APIKey string `mapstructure:"api_key"`
}

type Config struct {
	Symbol              string    `mapstructure:"symbol"`
	DebateRounds        int       `mapstructure:"debate_rounds"`
	LoopIntervalSeconds int       `mapstructure:"loop_interval_seconds"`
	Mode                string    `mapstructure:"mode"`
	LLM                 LLMConfig `mapstructure:"llm_config"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetDefault("symbol", "AAPL")
	v.SetDefault("debate_rounds", 2)
	v.SetDefault("loop_interval_seconds", 60)
	v.SetDefault("mode", "single")
	v.SetDefault("llm_config.model", "gpt-4o-mini")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
