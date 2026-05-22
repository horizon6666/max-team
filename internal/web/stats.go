package web

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

type Stats struct {
	TotalLLMCalls    int                `json:"total_llm_calls"`
	TotalToolExecs   int                `json:"total_tool_execs"`
	TotalInputTokens int64              `json:"total_input_tokens"`
	TotalOutputTokens int64             `json:"total_output_tokens"`
	ByAgent          map[string]*AgentStats `json:"by_agent"`
	ToolUsage        map[string]int     `json:"tool_usage"`
	TokenTrend       []TokenDataPoint   `json:"token_trend"`
}

type AgentStats struct {
	LLMCalls     int   `json:"llm_calls"`
	ToolExecs    int   `json:"tool_execs"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	Errors       int   `json:"errors"`
}

type TokenDataPoint struct {
	Time         string `json:"time"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

func ComputeStats(filepath string) *Stats {
	f, err := os.Open(filepath)
	if err != nil {
		return &Stats{
			ByAgent:   make(map[string]*AgentStats),
			ToolUsage: make(map[string]int),
		}
	}
	defer f.Close()

	stats := &Stats{
		ByAgent:   make(map[string]*AgentStats),
		ToolUsage: make(map[string]int),
	}

	hourBuckets := make(map[string]*TokenDataPoint)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var evt AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
			continue
		}

		as := stats.ByAgent[evt.Agent]
		if as == nil {
			as = &AgentStats{}
			stats.ByAgent[evt.Agent] = as
		}

		switch evt.Type {
		case "llm_call":
			stats.TotalLLMCalls++
			as.LLMCalls++
			if it, ok := evt.Data["input_tokens"].(float64); ok {
				stats.TotalInputTokens += int64(it)
				as.InputTokens += int64(it)
			}
			if ot, ok := evt.Data["output_tokens"].(float64); ok {
				stats.TotalOutputTokens += int64(ot)
				as.OutputTokens += int64(ot)
			}

			hourKey := evt.Timestamp.Truncate(time.Hour).Format("2006-01-02 15:04")
			bucket := hourBuckets[hourKey]
			if bucket == nil {
				bucket = &TokenDataPoint{Time: hourKey}
				hourBuckets[hourKey] = bucket
			}
			if it, ok := evt.Data["input_tokens"].(float64); ok {
				bucket.InputTokens += int64(it)
			}
			if ot, ok := evt.Data["output_tokens"].(float64); ok {
				bucket.OutputTokens += int64(ot)
			}

		case "tool_exec":
			stats.TotalToolExecs++
			as.ToolExecs++
			if toolName, ok := evt.Data["tool"].(string); ok {
				stats.ToolUsage[toolName]++
			}
			if success, ok := evt.Data["success"].(bool); ok && !success {
				as.Errors++
			}

		case "error":
			as.Errors++
		}
	}

	for _, bucket := range hourBuckets {
		stats.TokenTrend = append(stats.TokenTrend, *bucket)
	}

	return stats
}
