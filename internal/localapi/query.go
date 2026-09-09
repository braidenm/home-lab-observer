package localapi

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/braidenm/home-lab-observer/internal/history"
	"github.com/braidenm/home-lab-observer/internal/series"
)

const maxRawQueryBytes = 4096

var sectionNames = []string{"overview", "filesystems", "processes", "services", "containers", "logs", "observer"}

type currentQuery struct {
	sections         map[string]bool
	processLimit     int
	containerLimit   int
	logLimit         int
	includeLogBodies bool
}

type seriesQuery struct {
	rangeValue series.Range
	metrics    []history.MetricID
}

func parseCurrentQuery(raw string) (currentQuery, error) {
	result := currentQuery{sections: make(map[string]bool), processLimit: 50, containerLimit: 100, logLimit: 50}
	values, err := parseQuery(raw, map[string]bool{"section": true, "process_limit": true, "container_limit": true, "log_limit": true, "include_log_bodies": true})
	if err != nil {
		return currentQuery{}, err
	}
	for _, section := range values["section"] {
		if !contains(sectionNames, section) || result.sections[section] {
			return currentQuery{}, errors.New("section must be a unique allowlisted value")
		}
		result.sections[section] = true
	}
	if len(result.sections) == 0 {
		for _, section := range sectionNames {
			result.sections[section] = true
		}
	}
	if result.processLimit, err = scalarInt(values, "process_limit", 1, 200, result.processLimit); err != nil {
		return currentQuery{}, err
	}
	if result.containerLimit, err = scalarInt(values, "container_limit", 1, 500, result.containerLimit); err != nil {
		return currentQuery{}, err
	}
	if result.logLimit, err = scalarInt(values, "log_limit", 0, 200, result.logLimit); err != nil {
		return currentQuery{}, err
	}
	if supplied := values["include_log_bodies"]; len(supplied) > 0 {
		if len(supplied) != 1 || (supplied[0] != "true" && supplied[0] != "false") {
			return currentQuery{}, errors.New("include_log_bodies must be true or false")
		}
		result.includeLogBodies = supplied[0] == "true"
	}
	return result, nil
}

func parseSeriesQuery(raw string) (seriesQuery, error) {
	values, err := parseQuery(raw, map[string]bool{"range": true, "metric": true})
	if err != nil {
		return seriesQuery{}, err
	}
	if len(values["range"]) != 1 {
		return seriesQuery{}, errors.New("range must occur exactly once")
	}
	rangeValue := series.Range(values["range"][0])
	if rangeValue != series.Range1Hour && rangeValue != series.Range6Hours && rangeValue != series.Range24Hours && rangeValue != series.Range7Days {
		return seriesQuery{}, errors.New("range is not allowlisted")
	}
	if len(values["metric"]) < 1 || len(values["metric"]) > 6 {
		return seriesQuery{}, errors.New("one to six metrics are required")
	}
	seen := make(map[history.MetricID]bool)
	metrics := make([]history.MetricID, 0, len(values["metric"]))
	for _, rawMetric := range values["metric"] {
		metric := history.MetricID(rawMetric)
		if !allowedMetric(metric) || seen[metric] {
			return seriesQuery{}, errors.New("metric must be a unique allowlisted value")
		}
		seen[metric] = true
		metrics = append(metrics, metric)
	}
	return seriesQuery{rangeValue: rangeValue, metrics: metrics}, nil
}

func parseQuery(raw string, allowed map[string]bool) (url.Values, error) {
	if len(raw) > maxRawQueryBytes {
		return nil, errors.New("query exceeds size limit")
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, errors.New("query is malformed")
	}
	for name, candidates := range values {
		if !allowed[name] || name == "" || len(candidates) == 0 {
			return nil, errors.New("query parameter is not supported")
		}
		for _, candidate := range candidates {
			if strings.TrimSpace(candidate) == "" || candidate != strings.TrimSpace(candidate) {
				return nil, errors.New("query value is empty or padded")
			}
		}
	}
	return values, nil
}

func scalarInt(values url.Values, name string, low, high, fallback int) (int, error) {
	candidates := values[name]
	if len(candidates) == 0 {
		return fallback, nil
	}
	if len(candidates) != 1 {
		return 0, errors.New(name + " must occur at most once")
	}
	value, err := strconv.Atoi(candidates[0])
	if err != nil || strconv.Itoa(value) != candidates[0] || value < low || value > high {
		return 0, errors.New(name + " is outside its allowed range")
	}
	return value, nil
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func allowedMetric(metric history.MetricID) bool {
	switch metric {
	case history.CPUUtilization, history.MemoryUtilization, history.FilesystemUtilization, history.NetworkReceiveRate, history.NetworkTransmitRate, history.ProcessCount:
		return true
	default:
		return false
	}
}

func stringPointer(value string) *string { return &value }
