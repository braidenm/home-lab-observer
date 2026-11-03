package actionremote

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

type pairedPlatformInput struct {
	port           int
	binding        Binding
	action, target string
	operation      string
	credential     Credential
}

func (pairedPlatformInput) String() string   { return "PairedPlatformFixture([redacted])" }
func (pairedPlatformInput) GoString() string { return "PairedPlatformFixture([redacted])" }

var errPairedPlatformFixture = errors.New("paired_platform_fixture_refused")

func readPairedPlatformInput(r io.Reader) (pairedPlatformInput, error) {
	var result pairedPlatformInput
	if r == nil {
		return result, errPairedPlatformFixture
	}
	raw, err := io.ReadAll(io.LimitReader(r, 16385))
	defer clear(raw)
	if err != nil || len(raw) > 16384 || !utf8.Valid(raw) {
		return result, errPairedPlatformFixture
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return result, errPairedPlatformFixture
	}
	fields := make(map[string]json.RawMessage, 9)
	defer func() {
		for _, value := range fields {
			clear(value)
		}
	}()
	for decoder.More() {
		key, err := decoder.Token()
		name, valid := key.(string)
		if err != nil || !valid || fields[name] != nil {
			return result, errPairedPlatformFixture
		}
		switch name {
		case "format_version", "app_port", "server_id", "owner_user_id", "connector_instance_id", "action_id", "target_id", "operation", "action_secret":
		default:
			return result, errPairedPlatformFixture
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return result, errPairedPlatformFixture
		}
		fields[name] = value
	}
	closing, err := decoder.Token()
	var trailing any
	if err != nil || closing != json.Delim('}') || len(fields) != 9 || decoder.Decode(&trailing) != io.EOF || string(fields["format_version"]) != "1" {
		return result, errPairedPlatformFixture
	}
	port, err := strconv.Atoi(string(fields["app_port"]))
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != string(fields["app_port"]) {
		return result, errPairedPlatformFixture
	}
	text := func(name string) string {
		var value string
		if json.Unmarshal(fields[name], &value) != nil {
			return ""
		}
		return value
	}
	binding := Binding{text("server_id"), text("owner_user_id"), text("connector_instance_id")}
	action, target, operation := text("action_id"), text("target_id"), text("operation")
	credential, err := NewCredential(text("action_secret"))
	if err != nil || !validBinding(binding) || !id(action, "hla_") || !id(target, "hlt_") ||
		(operation != "CONTAINER_START" && operation != "CONTAINER_STOP" && operation != "CONTAINER_RESTART") {
		return result, errPairedPlatformFixture
	}
	return pairedPlatformInput{port, binding, action, target, operation, credential}, nil
}

func pairedPlatformAllowed(r *http.Request, in pairedPlatformInput) bool {
	if r == nil || r.URL == nil || r.URL.Scheme != "" || r.URL.Host != "" || r.URL.RawPath != "" ||
		r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.Fragment != "" ||
		len(r.Header.Values("Authorization")) != 1 || r.Header.Get("Authorization") != "ActionConnector "+in.credential.secret ||
		len(r.Header.Values("X-Home-Lab-Connector-Instance")) != 1 || r.Header.Get("X-Home-Lab-Connector-Instance") != in.binding.ConnectorInstanceID ||
		len(r.Header.Values("Accept")) != 1 || r.Header.Get("Accept") != "application/json" || len(r.Header.Values("Content-Encoding")) != 0 {
		return false
	}
	base := "/v1/connectors/home-lab/servers/" + in.binding.ServerID + "/container-actions"
	if r.Method == "GET" {
		return r.URL.Path == base+"/"+in.action+"/retained-status" && r.ContentLength == 0 && len(r.Header.Values("Content-Type")) == 0
	}
	return r.Method == "POST" && (r.URL.Path == base+"/claim" || r.URL.Path == base+"/current-check" ||
		r.URL.Path == base+"/"+in.action+"/report-result") && r.ContentLength > 0 && r.ContentLength <= 16384 &&
		len(r.Header.Values("Content-Type")) == 1 && r.Header.Get("Content-Type") == "application/json"
}

func pairedInputJSON() string {
	raw, _ := json.Marshal(map[string]any{
		"format_version": 1, "app_port": 12345,
		"server_id": "srv_" + strings.Repeat("1", 32), "owner_user_id": "usr_" + strings.Repeat("1", 32),
		"connector_instance_id": "agent_" + strings.Repeat("1", 32), "action_id": "hla_" + strings.Repeat("1", 32),
		"target_id": "hlt_" + strings.Repeat("1", 32), "operation": "CONTAINER_START",
		"action_secret": "hlac_" + strings.Repeat("1", 32) + "." + base64.RawURLEncoding.EncodeToString(make([]byte, 32)),
	})
	return string(raw)
}

func TestPairedPlatformFixtureInputIsClosedAndBounded(t *testing.T) {
	base := pairedInputJSON()
	if _, err := readPairedPlatformInput(strings.NewReader(base)); err != nil {
		t.Fatal("valid synthetic paired input refused")
	}
	for _, raw := range []string{
		base + "{}", strings.TrimSuffix(base, "}") + `,"other":true}`,
		strings.TrimSuffix(base, "}") + `,"app_port":12345}`,
		strings.Replace(base, `"app_port":12345`, `"app_port":70000`, 1),
		strings.Replace(base, `"app_port":12345`, `"app_port":12345.0`, 1),
		strings.Replace(base, `"format_version":1`, `"format_version":2`, 1),
		strings.Replace(base, "CONTAINER_START", "EXEC", 1),
		strings.Replace(base, "hlac_", "hle_", 1),
		strings.Repeat(" ", 16385), string([]byte{0xff}),
	} {
		if _, err := readPairedPlatformInput(strings.NewReader(raw)); err == nil {
			t.Fatal("unsupported paired input accepted")
		}
	}
}

func TestPairedPlatformForwardingAllowlistIsExact(t *testing.T) {
	in, err := readPairedPlatformInput(strings.NewReader(pairedInputJSON()))
	if err != nil {
		t.Fatal("synthetic fixture")
	}
	base := "/v1/connectors/home-lab/servers/" + in.binding.ServerID + "/container-actions"
	for _, entry := range []struct{ method, path string }{
		{"POST", base + "/claim"}, {"POST", base + "/current-check"},
		{"POST", base + "/" + in.action + "/report-result"}, {"GET", base + "/" + in.action + "/retained-status"},
	} {
		request := func() *http.Request {
			var r *http.Request
			if entry.method == "POST" {
				r = httptest.NewRequest(entry.method, entry.path, strings.NewReader("{}"))
				r.Header.Set("Content-Type", "application/json")
			} else {
				r = httptest.NewRequest(entry.method, entry.path, nil)
			}
			r.Header.Set("Authorization", "ActionConnector "+in.credential.secret)
			r.Header.Set("X-Home-Lab-Connector-Instance", in.binding.ConnectorInstanceID)
			r.Header.Set("Accept", "application/json")
			return r
		}
		if !pairedPlatformAllowed(request(), in) {
			t.Fatal("exact client request refused")
		}
		for _, mutate := range []func(*http.Request){
			func(r *http.Request) { r.Method = "DELETE" },
			func(r *http.Request) { r.URL.Path += "/other" },
			func(r *http.Request) { r.URL.RawQuery = "other=1" },
			func(r *http.Request) { r.URL.RawPath = r.URL.Path },
			func(r *http.Request) { r.Header.Add("Authorization", "other") },
			func(r *http.Request) { r.Header.Set("X-Home-Lab-Connector-Instance", "other") },
			func(r *http.Request) { r.ContentLength = 16385 },
		} {
			r := request()
			mutate(r)
			if pairedPlatformAllowed(r, in) {
				t.Fatal("request escaped the closed forwarding boundary")
			}
		}
	}
}
