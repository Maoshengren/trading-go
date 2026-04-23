package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"trading-go/config"
	"trading-go/orchestrator"
	"trading-go/state"
)

func main() {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})

	cfg, err := config.Load("config.yaml")
	if err != nil {
		logger.WithError(err).Fatal("failed to load config")
	}

	flow := orchestrator.NewWorkflow(cfg.DebateRounds)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cfg.Mode {
	case "loop":
		runLoop(ctx, flow, cfg, logger)
	default:
		runSingle(flow, cfg, logger)
	}
}

func runSingle(flow *orchestrator.Workflow, cfg *config.Config, logger *logrus.Logger) {
	s := &state.AgentState{Symbol: cfg.Symbol}
	if err := flow.Run(s); err != nil {
		logger.WithError(err).Error("workflow run failed")
		return
	}
	logger.WithField("final_decision", s.FinalDecision).Info("single analysis finished")
}

func runLoop(ctx context.Context, flow *orchestrator.Workflow, cfg *config.Config, logger *logrus.Logger) {
	ticker := time.NewTicker(time.Duration(cfg.LoopIntervalSeconds) * time.Second)
	defer ticker.Stop()

	logger.WithField("interval_seconds", cfg.LoopIntervalSeconds).Info("loop mode started")
	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown signal received, exiting loop")
			return
		case <-ticker.C:
			s := &state.AgentState{Symbol: cfg.Symbol}
			if err := flow.Run(s); err != nil {
				logger.WithError(err).Error("workflow run failed")
				continue
			}
			logger.WithField("final_decision", s.FinalDecision).Info("loop analysis finished")
		}
	}
}
