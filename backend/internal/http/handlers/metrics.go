package handlers

import (
	"fmt"
	"net/http"
	"openagent/internal/metrics"
)

func Metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP http_requests_total Total HTTP requests\n")
	fmt.Fprintf(w, "# TYPE http_requests_total counter\n")
	fmt.Fprintf(w, "http_requests_total %d\n", metrics.RequestsTotal)
	fmt.Fprintf(w, "# HELP agent_runs_total Total agent runs\n")
	fmt.Fprintf(w, "agent_runs_total %d\n", metrics.AgentRunsTotal)
	fmt.Fprintf(w, "agent_runs_failed %d\n", metrics.AgentRunsFailed)
	fmt.Fprintf(w, "# HELP tasks_total Total tasks\n")
	fmt.Fprintf(w, "tasks_total %d\n", metrics.TasksTotal)
	fmt.Fprintf(w, "tasks_completed %d\n", metrics.TasksCompleted)
	fmt.Fprintf(w, "# HELP tool_executions_total Total tool executions\n")
	fmt.Fprintf(w, "tool_executions_total %d\n", metrics.ToolExecutionsTotal)
	fmt.Fprintf(w, "tool_executions_failed %d\n", metrics.ToolExecutionsFailed)
	fmt.Fprintf(w, "# HELP ws_connections Current WS connections\n")
	fmt.Fprintf(w, "ws_connections %d\n", metrics.WSConnections)
	fmt.Fprintf(w, "# HELP browser_sessions Total browser sessions\n")
	fmt.Fprintf(w, "browser_sessions %d\n", metrics.BrowserSessions)
	fmt.Fprintf(w, "browser_session_failures %d\n", metrics.BrowserSessionFailures)
	fmt.Fprintf(w, "browser_tool_calls %d\n", metrics.BrowserToolCalls)
	fmt.Fprintf(w, "browser_tool_failures %d\n", metrics.BrowserToolFailures)
	fmt.Fprintf(w, "browser_pages_visited %d\n", metrics.BrowserPagesVisited)
	fmt.Fprintf(w, "browser_downloads %d\n", metrics.BrowserDownloads)
	fmt.Fprintf(w, "browser_timeouts %d\n", metrics.BrowserTimeouts)
	fmt.Fprintf(w, "browser_auth_failures %d\n", metrics.BrowserAuthFailures)
	fmt.Fprintf(w, "# HELP assistant_sessions Total assistant sessions\n")
	fmt.Fprintf(w, "assistant_sessions %d\n", metrics.AssistantSessions)
	fmt.Fprintf(w, "assistant_runs %d\n", metrics.AssistantRuns)
	fmt.Fprintf(w, "assistant_tool_calls %d\n", metrics.AssistantToolCalls)
	fmt.Fprintf(w, "assistant_agent_creations %d\n", metrics.AssistantAgentCreations)
	fmt.Fprintf(w, "assistant_task_creations %d\n", metrics.AssistantTaskCreations)
	fmt.Fprintf(w, "assistant_team_creations %d\n", metrics.AssistantTeamCreations)
	fmt.Fprintf(w, "assistant_failures %d\n", metrics.AssistantFailures)
	fmt.Fprintf(w, "# HELP http_latency_avg_ms Average latency\n")
	fmt.Fprintf(w, "http_latency_avg_ms %f\n", metrics.AvgLatency())
}
