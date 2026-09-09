package localapi

import "testing"

func TestCurrentQueryDefaultsAndStrictValidation(t *testing.T) {
	query, err := parseCurrentQuery("")
	if err != nil {
		t.Fatal(err)
	}
	if query.processLimit != 50 || query.containerLimit != 100 || query.logLimit != 50 || len(query.sections) != 7 {
		t.Fatalf("unexpected defaults: %+v", query)
	}

	valid, err := parseCurrentQuery("section=overview&section=processes&process_limit=17&container_limit=1&log_limit=0&include_log_bodies=false")
	if err != nil || len(valid.sections) != 2 || valid.processLimit != 17 || valid.logLimit != 0 {
		t.Fatalf("valid query rejected: query=%+v err=%v", valid, err)
	}

	for _, raw := range []string{
		"unknown=value", "section=processes&section=processes", "section=unknown", "section=",
		"process_limit=0", "process_limit=201", "process_limit=01", "process_limit=1&process_limit=2",
		"container_limit=501", "log_limit=-1", "include_log_bodies=TRUE", "include_log_bodies=true&include_log_bodies=false",
		"section=overview;process_limit=1", "section=%zz", "section=%20overview", "section=overview&" + oversizedValue(),
	} {
		if _, err := parseCurrentQuery(raw); err == nil {
			t.Errorf("accepted invalid current query %q", raw)
		}
	}
}

func TestSeriesQueryRequiresClosedUniqueValues(t *testing.T) {
	valid, err := parseSeriesQuery("range=6h&metric=cpu.utilization.percent&metric=process.count")
	if err != nil || valid.rangeValue != "6h" || len(valid.metrics) != 2 {
		t.Fatalf("valid query rejected: query=%+v err=%v", valid, err)
	}
	for _, raw := range []string{
		"", "range=1h", "metric=process.count", "range=forever&metric=process.count",
		"range=1h&range=6h&metric=process.count", "range=1h&metric=arbitrary",
		"range=1h&metric=process.count&metric=process.count", "range=1h&metric=process.count&sql=select",
	} {
		if _, err := parseSeriesQuery(raw); err == nil {
			t.Errorf("accepted invalid series query %q", raw)
		}
	}
}

func TestContainerQueryIsClosedAndBounded(t *testing.T) {
	for raw, want := range map[string]int{"": 100, "limit=1": 1, "limit=500": 500} {
		query, err := parseContainerQuery(raw)
		if err != nil || query.limit != want {
			t.Fatalf("query %q = %+v, err=%v", raw, query, err)
		}
	}
	for _, raw := range []string{"limit=0", "limit=501", "limit=01", "limit=-1", "limit=1&limit=2", "limit=1&extra=value", "limit=", "limit=%20", "limit=1&" + oversizedValue()} {
		if _, err := parseContainerQuery(raw); err == nil {
			t.Errorf("accepted invalid container query %q", raw)
		}
	}
}

func oversizedValue() string {
	value := make([]byte, maxRawQueryBytes+1)
	for index := range value {
		value[index] = 'x'
	}
	return string(value)
}
