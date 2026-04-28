package main

import (
	"strconv"
	"strings"
)

// Metric is a cheap, flat representation of a Prometheus metric line
// that a Vue component can render directly without re-parsing the
// exposition format in JavaScript.
type Metric struct {
	Name   string            `json:"name"`
	Type   string            `json:"type,omitempty"`
	Help   string            `json:"help,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}

// parseMetricsText walks Prometheus text exposition output line by
// line. We deliberately keep the parser minimal: we do not need
// histograms' bucket math nor SUMMARY quantiles for the UI, the
// plain "name{labels} value" samples are enough to power gauges
// and sparklines. Unparseable lines are silently ignored so we
// stay forward-compatible with server metric additions.
func parseMetricsText(raw string) []Metric {
	out := []Metric{}
	var typeMap = map[string]string{}
	var helpMap = map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# TYPE") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				typeMap[parts[2]] = parts[3]
			}
			continue
		}
		if strings.HasPrefix(line, "# HELP") {
			parts := strings.SplitN(line, " ", 4)
			if len(parts) == 4 {
				helpMap[parts[2]] = parts[3]
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		name, labels, value, ok := parseSample(line)
		if !ok {
			continue
		}
		out = append(out, Metric{
			Name:   name,
			Type:   typeMap[name],
			Help:   helpMap[name],
			Labels: labels,
			Value:  value,
		})
	}
	return out
}

func parseSample(line string) (string, map[string]string, float64, bool) {
	idx := strings.IndexAny(line, " \t")
	if idx < 0 {
		return "", nil, 0, false
	}
	head := line[:idx]
	tail := strings.TrimSpace(line[idx:])
	if space := strings.IndexAny(tail, " \t"); space >= 0 {
		tail = tail[:space]
	}
	value, err := strconv.ParseFloat(tail, 64)
	if err != nil {
		return "", nil, 0, false
	}
	name := head
	labels := map[string]string{}
	if brace := strings.IndexByte(head, '{'); brace >= 0 && strings.HasSuffix(head, "}") {
		name = head[:brace]
		labelRaw := head[brace+1 : len(head)-1]
		labels = parseLabels(labelRaw)
	}
	return name, labels, value, true
}

func parseLabels(raw string) map[string]string {
	out := map[string]string{}
	if raw == "" {
		return out
	}
	i := 0
	for i < len(raw) {
		// key
		eq := strings.IndexByte(raw[i:], '=')
		if eq < 0 {
			break
		}
		key := raw[i : i+eq]
		i += eq + 1
		if i >= len(raw) || raw[i] != '"' {
			break
		}
		i++
		var b strings.Builder
		for i < len(raw) {
			ch := raw[i]
			if ch == '\\' && i+1 < len(raw) {
				b.WriteByte(raw[i+1])
				i += 2
				continue
			}
			if ch == '"' {
				i++
				break
			}
			b.WriteByte(ch)
			i++
		}
		out[key] = b.String()
		if i < len(raw) && raw[i] == ',' {
			i++
		}
	}
	return out
}
