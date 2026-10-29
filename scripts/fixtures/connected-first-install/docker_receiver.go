package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/braidenm/home-lab-observer/internal/combinedprojection"
	"github.com/braidenm/home-lab-observer/internal/connectedprofile"
)

// Synthetic no-NIC VM identities only; never a hosted endpoint or credential.
const dockerServerID = "srv_cccccccccccccccccccccccccccccccc"
const dockerGrant = "hle_EEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEEE"
const dockerCredential = "hlc_FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"

type dockerCounts struct {
	Connector        string `json:"connector_id"`
	Sequence         int64  `json:"sequence"`
	Digest           string `json:"sha256"`
	Accepted         uint64 `json:"accepted"`
	Duplicates       uint64 `json:"duplicates"`
	Lost             bool   `json:"lost_ack"`
	Large            bool   `json:"large_body"`
	Foreign          bool   `json:"foreign_process"`
	Running          bool   `json:"running_container"`
	ExitedNull       bool   `json:"exited_null"`
	Warmed           bool   `json:"warmed_metrics"`
	Unavailable      bool   `json:"unavailable"`
	UnavailableCount uint64 `json:"unavailable_count"`
}

type dockerReceiver struct {
	mu    sync.Mutex
	state dockerCounts
	save  func(dockerCounts) error
	hold  func() bool
}

func validDockerCounts(s dockerCounts) bool {
	if !connectedprofile.ValidConnectorID(s.Connector) || s.Sequence < 0 || s.Accepted > 10000 || s.Duplicates > 10000 || s.UnavailableCount > s.Accepted || s.Unavailable != (s.UnavailableCount > 0) {
		return false
	}
	if s.Sequence == 0 {
		return s.Accepted == 0 && s.Duplicates == 0 && s.Digest == "" && !s.Lost && !s.Large && !s.Foreign && !s.Running && !s.ExitedNull && !s.Warmed
	}
	digest, err := hex.DecodeString(s.Digest)
	return err == nil && len(digest) == 32 && hex.EncodeToString(digest) == s.Digest && s.Accepted > 0 && s.Lost && (!s.Warmed || s.Running)
}

func (a *dockerReceiver) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != connectedprofile.Hostname || r.URL.RawQuery != "" {
			http.Error(w, "fixture refusal", 404)
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if r.Method == http.MethodPost && r.URL.Path == path {
			a.exchange(w, r)
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/v1/connectors/home-lab/servers/"+dockerServerID+"/snapshot" {
			http.Error(w, "fixture refusal", 404)
			return
		}
		a.snapshot(w, r)
	})
}

func (a *dockerReceiver) exchange(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 513))
	defer clear(body)
	var v struct {
		Secret     string `json:"enrollment_secret"`
		Connector  string `json:"connector_instance_id"`
		Profile    string `json:"collection_profile"`
		Definition string `json:"collection_definition_sha256"`
	}
	if err != nil || len(body) > 512 || json.Unmarshal(body, &v) != nil || v.Secret != dockerGrant || !connectedprofile.ValidConnectorID(v.Connector) || v.Profile != connectedprofile.RichProcessesDockerProfile || v.Definition != connectedprofile.RichProcessesDockerDefinitionSHA256 || a.state.Connector != "" {
		http.Error(w, "fixture refusal", 400)
		return
	}
	canonical, _ := json.Marshal(v)
	if !bytes.Equal(body, canonical) {
		http.Error(w, "fixture refusal", 400)
		return
	}
	next := dockerCounts{Connector: v.Connector}
	if a.save == nil || a.save(next) != nil {
		http.Error(w, "fixture refusal", 500)
		return
	}
	a.state = next
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"server_id":"%s","connector_secret":"%s","collection_profile":"%s","collection_definition_sha256":"%s"}`, dockerServerID, dockerCredential, connectedprofile.RichProcessesDockerProfile, connectedprofile.RichProcessesDockerDefinitionSHA256)
}

type dockerObservations struct {
	Workload struct {
		Policy struct {
			Processes  bool `json:"processes_enabled"`
			Containers bool `json:"containers_enabled"`
		} `json:"policy"`
		Processes struct {
			Items []struct {
				Name string `json:"name"`
			} `json:"items"`
		} `json:"processes"`
		Containers struct {
			Quality struct {
				State string `json:"state"`
			} `json:"quality"`
			Items []struct {
				Name    string   `json:"name"`
				State   string   `json:"state"`
				CPU     *float64 `json:"cpu_percent"`
				Memory  *uint64  `json:"memory_bytes"`
				Metrics string   `json:"metrics_state"`
			} `json:"items"`
		} `json:"containers"`
	} `json:"workload_inventory"`
}

func (a *dockerReceiver) snapshot(w http.ResponseWriter, r *http.Request) {
	if a.state.Connector == "" || r.Header.Get("Authorization") != "Connector "+dockerCredential {
		http.Error(w, "fixture refusal", 401)
		return
	}
	sequence, err := strconv.ParseInt(r.Header.Get("X-Connector-Sequence"), 10, 64)
	if err != nil || sequence <= 0 || strconv.FormatInt(sequence, 10) != r.Header.Get("X-Connector-Sequence") {
		http.Error(w, "fixture refusal", 400)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, combinedprojection.MaxBytes+1))
	defer clear(body)
	if err != nil || bytes.Contains(body, []byte("hlo-docker-private-canary")) || combinedprojection.Validate(body, dockerServerID) != nil {
		http.Error(w, "fixture refusal", 400)
		return
	}
	var observed dockerObservations
	if json.Unmarshal(body, &observed) != nil || !observed.Workload.Policy.Processes || !observed.Workload.Policy.Containers {
		http.Error(w, "fixture refusal", 400)
		return
	}
	digest := sha256.Sum256(body)
	text := hex.EncodeToString(digest[:])
	if sequence < a.state.Sequence || sequence == a.state.Sequence && text != a.state.Digest {
		http.Error(w, "fixture refusal", 409)
		return
	}
	next := a.state
	if sequence == next.Sequence {
		next.Duplicates++
	} else {
		next.Sequence, next.Digest = sequence, text
		next.Accepted++
		recordDockerObservations(&next, observed, len(body))
	}
	lose := !next.Lost || a.hold != nil && a.hold()
	next.Lost = true
	if !validDockerCounts(next) || a.save == nil || a.save(next) != nil {
		http.Error(w, "fixture refusal", 500)
		return
	}
	a.state = next
	if lose {
		if h, ok := w.(http.Hijacker); ok {
			if c, _, err := h.Hijack(); err == nil {
				c.Close()
				return
			}
		}
		http.Error(w, "fixture response loss", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"server_id":"%s","sequence":%d}`, dockerServerID, sequence)
}

func recordDockerObservations(next *dockerCounts, d dockerObservations, size int) {
	next.Large = next.Large || size > 16*1024
	for _, row := range d.Workload.Processes.Items {
		next.Foreign = next.Foreign || strings.HasPrefix(row.Name, "hlofixture")
	}
	switch d.Workload.Containers.Quality.State {
	case "UNAVAILABLE", "PERMISSION_DENIED", "UNSUPPORTED":
		next.UnavailableCount++
		next.Unavailable = true
	}
	for _, row := range d.Workload.Containers.Items {
		if row.Name == "hlo-fixture-running" && row.State == "running" {
			next.Running = true
			next.Warmed = next.Warmed || row.CPU != nil && row.Memory != nil && row.Metrics == "AVAILABLE"
		}
		if row.Name == "hlo-fixture-exited" && row.State == "exited" && row.CPU == nil && row.Memory == nil && row.Metrics == "NOT_RUNNING" {
			next.ExitedNull = true
		}
	}
}
