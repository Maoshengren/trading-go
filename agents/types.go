package agents

import (
	"context"

	"trading-go/state"
)

type Agent interface {
	Name() string
	Description() string
	Run(context.Context, *state.AgentState) error
}
