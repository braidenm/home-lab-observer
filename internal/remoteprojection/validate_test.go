package remoteprojection

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/braidenm/home-lab-observer/internal/observation"
)

func TestValidateEncoderOutput(t *testing.T) {
	for _, osName := range []string{"linux", "windows", "darwin"} {
		for _, count := range []int{0, 1, MaxFilesystems} {
			for _, available := range []bool{false, true} {
				raw, id := fixture()
				id.OS = osName
				raw.ObservedAt = time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)
				raw.Quality.DurationMS = 10000
				raw.CPU.Data = &observation.CPU{LogicalCPUs: 4096, UsagePercent: 100}
				raw.Memory.Data = &observation.Memory{TotalBytes: MaxExactInteger, UsedBytes: MaxExactInteger,
					SwapTotalBytes: MaxExactInteger, SwapUsedBytes: MaxExactInteger}
				raw.Uptime.Data.Seconds = MaxExactInteger
				fs := make([]observation.Filesystem, count)
				for i := range fs {
					fs[i] = observation.Filesystem{TotalBytes: MaxExactInteger, UsedBytes: MaxExactInteger}
				}
				raw.Filesystems.Data = &fs
				raw.Filesystems.Quality = observation.SectionQuality{Samples: count, Total: count}
				if !available {
					raw.CPU.State = observation.Unavailable
				}
				data, err := Encode(raw, id)
				if err != nil {
					t.Fatal(err)
				}
				before := bytes.Clone(data)
				if err := Validate(data, id.SourceID); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, data) {
					t.Fatal("input mutated")
				}
			}
		}
	}
}

func TestValidateRejectsAmbiguousAndIneligibleDocuments(t *testing.T) {
	raw, id := fixture()
	encoded, err := Encode(raw, id)
	if err != nil {
		t.Fatal(err)
	}
	base := string(encoded)
	cases := map[string]string{
		"empty": "", "null": "null", "array": "[]", "trailing value": base + "{}",
		"whitespace": base + "\n", "BOM": "\ufeff" + base,
		"oversize":     strings.Repeat(" ", MaxBytes+1),
		"deep nesting": strings.Repeat("[", 2000) + strings.Repeat("]", 2000),
	}
	for name, pair := range map[string][2]string{
		"unknown root":           {`"schema_version":`, `"private_secret":"synthetic-canary","schema_version":`},
		"duplicate root":         {`"duration_ms":125`, `"duration_ms":0,"duration_ms":125`},
		"case alias":             {`"duration_ms":`, `"DURATION_MS":`},
		"escaped key":            {`"duration_ms":`, `"duration_\u006ds":`},
		"missing key":            {`"duration_ms":125,`, ""},
		"null duration":          {`"duration_ms":125`, `"duration_ms":null`},
		"float integer":          {`"duration_ms":125`, `"duration_ms":125.0`},
		"duration high":          {`"duration_ms":125`, `"duration_ms":10001`},
		"duration low":           {`"duration_ms":125`, `"duration_ms":-1`},
		"schema":                 {`home-lab-server-snapshot/v1`, `home-lab-server-snapshot/v2`},
		"profile":                {`numeric-host/v1`, `numeric-host/v2`},
		"version":                {`0.1.0-preview.3`, `synthetic-private-version`},
		"invalid time":           {`2026-09-10T12:00:00Z`, `not-a-time`},
		"time offset":            {`2026-09-10T12:00:00Z`, `2026-09-10T12:00:00+00:00`},
		"zero time":              {`2026-09-10T12:00:00Z`, `0001-01-01T00:00:00Z`},
		"fraction spelling":      {`2026-09-10T12:00:00Z`, `2026-09-10T12:00:00.0Z`},
		"unknown nested":         {`"kernel":`, `"private_secret":"synthetic-canary","kernel":`},
		"duplicate nested":       {`"logical_cpu_count":8`, `"logical_cpu_count":1,"logical_cpu_count":8`},
		"cpu zero":               {`"logical_cpu_count":8`, `"logical_cpu_count":0`},
		"cpu high":               {`"logical_cpu_count":8`, `"logical_cpu_count":4097`},
		"percent high":           {`"cpu_usage_percent":12.5`, `"cpu_usage_percent":101`},
		"percent negative":       {`"cpu_usage_percent":12.5`, `"cpu_usage_percent":-1`},
		"percent overflow":       {`"cpu_usage_percent":12.5`, `"cpu_usage_percent":1e999`},
		"uptime precision":       {`"uptime_seconds":86400`, `"uptime_seconds":9007199254740992`},
		"uint overflow":          {`"uptime_seconds":86400`, `"uptime_seconds":18446744073709551616`},
		"memory total":           {`"memory_total_bytes":8192`, `"memory_total_bytes":9007199254740992`},
		"memory used":            {`"memory_used_bytes":4096`, `"memory_used_bytes":8193`},
		"swap total":             {`"swap_total_bytes":1024`, `"swap_total_bytes":9007199254740992`},
		"swap used":              {`"swap_used_bytes":128`, `"swap_used_bytes":1025`},
		"fs total":               {`"total_bytes":10000`, `"total_bytes":9007199254740992`},
		"fs used":                {`"used_bytes":5000`, `"used_bytes":10001`},
		"fs negative":            {`"used_bytes":5000`, `"used_bytes":-1`},
		"fs unknown":             {`"key":"filesystem-01"`, `"secret":"synthetic-canary","key":"filesystem-01"`},
		"fs alias":               {`filesystem-01`, `private-device`},
		"fs title":               {`Filesystem 1`, `private-mount`},
		"kernel":                 {`not collected`, `private-kernel`},
		"OS":                     {`Linux`, `other`},
		"load":                   {`"load_1":null`, `"load_1":0`},
		"excluded reason":        {`NOT_UPLOAD_ELIGIBLE`, `NONE`},
		"excluded false zero":    {`"available":false`, `"available":true`},
		"excluded missing items": {`,"items":[]`, ``},
		"excluded null items":    {`"items":[]`, `"items":null`},
		"excluded actual item":   {`"items":[]`, `"items":[{}]`},
		"invalid UTF8":           {`not collected`, "not collected\xff"},
	} {
		changed := strings.Replace(base, pair[0], pair[1], 1)
		if changed == base {
			t.Fatalf("ineffective case %s", name)
		}
		cases[name] = changed
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) { assertValidationFailure(t, []byte(data), id.SourceID) })
	}
	for _, expected := range []string{"", "synthetic-canary", "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} {
		assertValidationFailure(t, encoded, expected)
	}
}

func TestValidateUnavailableRejectsPayloadAndWrongReason(t *testing.T) {
	raw, id := fixture()
	raw.CPU.State = observation.Unavailable
	data, err := Encode(raw, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, replacement := range []string{
		`"available":false,"reason_code":"HOST_DATA_UNAVAILABLE","uptime_seconds":0`,
		`"available":false,"reason_code":"private-canary"`,
		`"available":false`,
		`"available":false,"available":false,"reason_code":"HOST_DATA_UNAVAILABLE"`,
	} {
		mutated := strings.Replace(string(data), `"available":false,"reason_code":"HOST_DATA_UNAVAILABLE"`, replacement, 1)
		assertValidationFailure(t, []byte(mutated), id.SourceID)
	}
}

func TestValidateFilesystemInventoryShape(t *testing.T) {
	raw, id := fixture()
	data, err := Encode(raw, id)
	if err != nil {
		t.Fatal(err)
	}
	item := `{"key":"filesystem-01","title":"Filesystem 1","total_bytes":10000,"used_bytes":5000}`
	for _, replacement := range []string{
		`null`,
		`[` + strings.TrimSuffix(strings.Repeat(item+",", MaxFilesystems+1), ",") + `]`,
	} {
		mutated := strings.Replace(string(data), "["+item+"]", replacement, 1)
		if mutated == string(data) {
			t.Fatal("ineffective filesystem mutation")
		}
		assertValidationFailure(t, []byte(mutated), id.SourceID)
	}
}

func assertValidationFailure(t *testing.T, data []byte, expected string) {
	t.Helper()
	err := Validate(data, expected)
	if !errors.Is(err, ErrValidation) || err.Error() != "remote_projection_invalid_document" {
		t.Fatal("invalid document accepted or noncanonical error returned")
	}
}

func FuzzValidate(f *testing.F) {
	raw, id := fixture()
	data, _ := Encode(raw, id)
	f.Add(data)
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, data []byte) {
		err := Validate(data, id.SourceID)
		if err != nil && err != ErrValidation {
			t.Fatal("noncanonical error")
		}
	})
}
