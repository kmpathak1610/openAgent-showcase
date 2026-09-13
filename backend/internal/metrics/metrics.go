package metrics

import (
	"sync/atomic"
	"time"
)

var (
	RequestsTotal int64
	AgentRunsTotal int64
	AgentRunsFailed int64
	TasksTotal int64
	TasksCompleted int64
	ToolExecutionsTotal int64
	ToolExecutionsFailed int64
	ApprovalsPending int64
	WSConnections int64
	BrowserSessions int64
	BrowserSessionFailures int64
	BrowserTimeouts int64
	BrowserToolCalls int64
	BrowserToolFailures int64
	BrowserPagesVisited int64
	BrowserDownloads int64
	BrowserAuthFailures int64
	AssistantSessions int64
	AssistantRuns int64
	AssistantToolCalls int64
	AssistantAgentCreations int64
	AssistantTaskCreations int64
	AssistantTeamCreations int64
	AssistantFailures int64
	// latency buckets in ms
	latencySum int64
	latencyCount int64
)

func IncRequests() { atomic.AddInt64(&RequestsTotal, 1) }
func IncAgentRuns() { atomic.AddInt64(&AgentRunsTotal, 1) }
func IncAgentRunsFailed() { atomic.AddInt64(&AgentRunsFailed, 1) }
func IncTasks() { atomic.AddInt64(&TasksTotal, 1) }
func IncTasksCompleted() { atomic.AddInt64(&TasksCompleted, 1) }
func IncToolExecutions() { atomic.AddInt64(&ToolExecutionsTotal, 1) }
func IncToolFailed() { atomic.AddInt64(&ToolExecutionsFailed, 1) }
func IncWS() { atomic.AddInt64(&WSConnections, 1) }
func DecWS() { atomic.AddInt64(&WSConnections, -1) }
func IncBrowserSessions() { atomic.AddInt64(&BrowserSessions, 1) }
func IncBrowserSessionFailures() { atomic.AddInt64(&BrowserSessionFailures, 1) }
func IncBrowserTimeouts() { atomic.AddInt64(&BrowserTimeouts, 1) }
func IncBrowserToolCalls() { atomic.AddInt64(&BrowserToolCalls, 1) }
func IncBrowserToolFailures() { atomic.AddInt64(&BrowserToolFailures, 1) }
func IncBrowserPagesVisited() { atomic.AddInt64(&BrowserPagesVisited, 1) }
func IncBrowserDownloads() { atomic.AddInt64(&BrowserDownloads, 1) }
func IncBrowserAuthFailures() { atomic.AddInt64(&BrowserAuthFailures, 1) }
func IncAssistantSessions() { atomic.AddInt64(&AssistantSessions, 1) }
func IncAssistantRuns() { atomic.AddInt64(&AssistantRuns, 1) }
func IncAssistantToolCalls() { atomic.AddInt64(&AssistantToolCalls, 1) }
func IncAssistantAgentCreations() { atomic.AddInt64(&AssistantAgentCreations, 1) }
func IncAssistantTaskCreations() { atomic.AddInt64(&AssistantTaskCreations, 1) }
func IncAssistantTeamCreations() { atomic.AddInt64(&AssistantTeamCreations, 1) }
func IncAssistantFailures() { atomic.AddInt64(&AssistantFailures, 1) }
func ObserveLatency(d time.Duration) {
	atomic.AddInt64(&latencySum, d.Milliseconds())
	atomic.AddInt64(&latencyCount, 1)
}
func AvgLatency() float64 {
	c := atomic.LoadInt64(&latencyCount)
	if c==0 { return 0 }
	return float64(atomic.LoadInt64(&latencySum)) / float64(c)
}
