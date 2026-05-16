package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

func Render(tmpl string, event string, payload map[string]any) string {
	if tmpl == "" {
		return event
	}
	out := strings.ReplaceAll(tmpl, "{{event}}", event)
	out = strings.ReplaceAll(out, "{{summary}}", Summary(payload))
	for k, v := range payload {
		out = strings.ReplaceAll(out, "{{"+k+"}}", fmt.Sprint(v))
	}
	return out
}

func Summary(payload map[string]any) string {
	for _, k := range []string{"summary", "message", "title", "name", "status", "reason"} {
		if v, ok := payload[k]; ok && fmt.Sprint(v) != "" {
			return fmt.Sprint(v)
		}
	}
	b, _ := json.Marshal(payload)
	return string(b)
}

func Match(pattern, event string) bool {
	if pattern == "*" || pattern == event {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(event, strings.TrimSuffix(pattern, "*"))
	}
	return false
}
