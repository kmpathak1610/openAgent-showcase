package runtime

import "time"

// Guardrails for autonomous execution — Phase 7/8 production hardening
const (
	ExecutionTimeout    = 5 * time.Minute
	MaxIterations       = 10
	MaxToolCalls        = 10
	MaxDelegationDepth  = 5
	MaxRetries          = 3
	MaxDecisionRetries  = 2
)
