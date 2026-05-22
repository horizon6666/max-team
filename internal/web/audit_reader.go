package web

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

type AuditEvent struct {
	Timestamp time.Time      `json:"ts"`
	Type      string         `json:"type"`
	Agent     string         `json:"agent"`
	Data      map[string]any `json:"data"`
}

func ReadAuditLog(filepath, agentFilter, typeFilter string, limit int) []AuditEvent {
	f, err := os.Open(filepath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []AuditEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var evt AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
			continue
		}
		if agentFilter != "" && evt.Agent != agentFilter {
			continue
		}
		if typeFilter != "" && evt.Type != typeFilter {
			continue
		}
		all = append(all, evt)
	}

	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all
}
