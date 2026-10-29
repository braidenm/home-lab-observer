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

// Fixed synthetic identities only. Separate from the stopped numeric fixture.
const richServerID = "srv_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const richGrant = "hle_CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
const richCredential = "hlc_DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"

type richCounts struct {
	Connector  string `json:"connector_id"`
	Sequence   int64  `json:"sequence"`
	Digest     string `json:"sha256"`
	Accepted   uint64 `json:"accepted"`
	Duplicates uint64 `json:"duplicates"`
	Lost       bool   `json:"lost_ack"`
	Large      bool   `json:"large_body"`
	Foreign    bool   `json:"foreign_process"`
}

type richReceiver struct {
	mu    sync.Mutex
	state richCounts
	save  func(richCounts) error
	hold  func() bool
}

func (a *richReceiver) handler() http.Handler {
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
		if r.Method != http.MethodPut || r.URL.Path != "/v1/connectors/home-lab/servers/"+richServerID+"/snapshot" {
			http.Error(w, "fixture refusal", 404)
			return
		}
		if a.state.Connector == "" || r.Header.Get("Authorization") != "Connector "+richCredential {
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
		if err != nil || combinedprojection.Validate(body, richServerID) != nil {
			http.Error(w, "fixture refusal", 400)
			return
		}
		var d struct {
			Workload struct {
				Policy struct {
					Processes  bool `json:"processes_enabled"`
					Containers bool `json:"containers_enabled"`
				} `json:"policy"`
				Containers struct {
					Quality struct {
						State string `json:"state"`
					} `json:"quality"`
				} `json:"containers"`
				Processes struct {
					Items []struct {
						Name string `json:"name"`
					} `json:"items"`
				} `json:"processes"`
			} `json:"workload_inventory"`
		}
		if json.Unmarshal(body, &d) != nil || !d.Workload.Policy.Processes || d.Workload.Policy.Containers || d.Workload.Containers.Quality.State != "DISABLED" {
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
			next.Sequence = sequence
			next.Digest = text
			next.Accepted++
		}
		next.Large = next.Large || len(body) > 16*1024
		for _, row := range d.Workload.Processes.Items {
			if strings.HasPrefix(row.Name, "hlofixture") {
				next.Foreign = true
			}
		}
		lose := !next.Lost || a.hold != nil && a.hold()
		next.Lost = true
		if a.save == nil || a.save(next) != nil {
			http.Error(w, "fixture refusal", 500)
			return
		}
		a.state = next
		if lose {
			// Commit synthetic receipt, then lose the response. No body is persisted.
			if h, ok := w.(http.Hijacker); ok {
				c, _, err := h.Hijack()
				if err == nil {
					c.Close()
					return
				}
			}
			http.Error(w, "fixture response loss", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"server_id":"%s","sequence":%d}`, richServerID, sequence)
	})
}

func (a *richReceiver) exchange(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 513))
	defer clear(body)
	var v struct {
		Secret     string `json:"enrollment_secret"`
		Connector  string `json:"connector_instance_id"`
		Profile    string `json:"collection_profile"`
		Definition string `json:"collection_definition_sha256"`
	}
	if err != nil || len(body) > 512 || json.Unmarshal(body, &v) != nil || v.Secret != richGrant || !connectedprofile.ValidConnectorID(v.Connector) || v.Profile != connectedprofile.RichProcessesProfile || v.Definition != connectedprofile.RichProcessesDefinitionSHA256 || a.state.Connector != "" {
		http.Error(w, "fixture refusal", 400)
		return
	}
	canonical, _ := json.Marshal(v)
	if !bytes.Equal(body, canonical) {
		http.Error(w, "fixture refusal", 400)
		return
	}
	next := richCounts{Connector: v.Connector}
	if a.save == nil || a.save(next) != nil {
		http.Error(w, "fixture refusal", 500)
		return
	}
	a.state = next
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"server_id":"%s","connector_secret":"%s","collection_profile":"%s","collection_definition_sha256":"%s"}`, richServerID, richCredential, connectedprofile.RichProcessesProfile, connectedprofile.RichProcessesDefinitionSHA256)
}
