package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"trading-go/agents/execution"
	"trading-go/config"
	"trading-go/internal/llm"
	"trading-go/internal/logx"
	"trading-go/orchestrator"
	"trading-go/state"
	"trading-go/tools"
)

func main() {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		panic(err)
	}

	logger, err := logx.Setup(cfg.Log)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := logx.Close(); err != nil {
			logger.WithError(err).Warn("failed to close log resources")
		}
	}()

	provider, providerName, err := tools.NewProvider(cfg.MarketData.Provider, cfg.MarketData.DefaultRegion)
	if err != nil {
		logger.WithError(err).Fatal("failed to initialize market data provider")
	}
	tools.SetDefaultProvider(provider)
	if accountProvider, ok := provider.(tools.AccountProvider); ok {
		tools.SetDefaultAccountProvider(accountProvider)
	}
	if tradeProvider, ok := provider.(tools.TradeProvider); ok {
		tools.SetDefaultTradeProvider(tradeProvider)
	}
	defer func() {
		if err := tools.CloseDefaultProvider(); err != nil {
			logger.WithError(err).Warn("failed to close market data provider")
		}
	}()
	logger.WithField("market_data_provider", providerName).Info("market data provider initialized")

	if err := llm.Init(context.Background(), cfg.LLM); err != nil {
		logger.WithError(err).Fatal("failed to initialize llm")
		panic(err)
	}

	flow := orchestrator.NewWorkflow(
		cfg.DebateRounds,
		cfg.Analysts.Enabled,
		cfg.MarketData.AnalysisPeriod,
		&execution.Agent{
			Enabled:                 cfg.Execution.Enabled,
			DryRun:                  cfg.Execution.DryRun,
			OrderType:               cfg.Execution.OrderType,
			TimeInForce:             cfg.Execution.TimeInForce,
			Leverage:                cfg.Execution.Leverage,
			PricePeriod:             cfg.Execution.PricePeriod,
			ProtectiveOrdersEnabled: cfg.Execution.ProtectiveOrdersEnabled,
			TakeProfitPercent:       cfg.Execution.TakeProfitPercent,
			StopLossPercent:         cfg.Execution.StopLossPercent,
		},
	)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cfg.Mode {
	case "loop":
		runLoop(ctx, flow, cfg, logger)
	default:
		runSingle(ctx, flow, cfg, logger)
	}
}

func runSingle(ctx context.Context, flow *orchestrator.Workflow, cfg *config.Config, logger *logrus.Logger) {
	s, runCtx, err := runOnce(ctx, flow, cfg.Symbol)
	if err != nil {
		logger.WithContext(runCtx).WithError(err).Error("workflow run failed")
		return
	}
	logger.WithContext(runCtx).WithField("final_decision", s.FinalDecision).Info("single analysis finished")
}

func runLoop(ctx context.Context, flow *orchestrator.Workflow, cfg *config.Config, logger *logrus.Logger) {
	ticker := time.NewTicker(time.Duration(cfg.LoopIntervalSeconds) * time.Second)
	defer ticker.Stop()

	logger.WithField("interval_seconds", cfg.LoopIntervalSeconds).Info("loop mode started")
	if s, runCtx, err := runOnce(ctx, flow, cfg.Symbol); err != nil {
		logger.WithContext(runCtx).WithError(err).Error("workflow run failed")
	} else {
		logger.WithContext(runCtx).WithField("final_decision", s.FinalDecision).Info("loop analysis finished")
	}
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown signal received, exiting loop")
			return
		case <-ticker.C:
			s, runCtx, err := runOnce(ctx, flow, cfg.Symbol)
			if err != nil {
				logger.WithContext(runCtx).WithError(err).Error("workflow run failed")
				continue
			}
			logger.WithContext(runCtx).WithField("final_decision", s.FinalDecision).Info("loop analysis finished")
		}
	}
}

func runOnce(ctx context.Context, flow *orchestrator.Workflow, symbol string) (*state.AgentState, context.Context, error) {
	s := &state.AgentState{Symbol: symbol}
	runCtx := withTraceID(ctx)
	if err := flow.Run(runCtx, s); err != nil {
		return nil, runCtx, err
	}
	return s, runCtx, nil
}

func withTraceID(ctx context.Context) context.Context {
	traceID := newTraceID(time.Now())
	return logx.WithTraceID(ctx, traceID)
}

func newTraceID(now time.Time) string {
	return fmt.Sprintf("trace-%s-%d", now.Format("20060102150405"), now.UnixNano())
}
