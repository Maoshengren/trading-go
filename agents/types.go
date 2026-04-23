package agents

import "trading-go/state"

type Agent interface {
	Name() string
	Description() string
	Run(*state.AgentState) error
}
