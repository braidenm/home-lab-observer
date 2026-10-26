package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/braidenm/home-lab-observer/internal/remoteprojection"
)

// Synthetic VM-only receiver. It persists counters, never observation bodies.
type activationReceiver struct {
	mu                   sync.Mutex
	enrolled             *atomic.Bool
	lastSequence         int64
	lastDigest           [32]byte
	accepted, duplicates uint64
	write                func(activationCounts) error
}

type activationCounts struct {
	Sequence   int64  `json:"sequence"`
	Accepted   uint64 `json:"accepted"`
	Duplicates uint64 `json:"duplicates"`
	UsefulHost bool   `json:"useful_host"`
}

func (a *activationReceiver) handler(enrollment http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == path {
			enrollment.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodPut || r.URL.Path != "/v1/connectors/home-lab/servers/"+serverID+"/snapshot" || r.URL.RawQuery != "" {
			http.Error(w, "fixture refusal", http.StatusNotFound)
			return
		}
		if !a.enrolled.Load() || r.Header.Get("Authorization") != "Connector "+credential {
			http.Error(w, "fixture refusal", http.StatusUnauthorized)
			return
		}
		sequence, err := strconv.ParseInt(r.Header.Get("X-Connector-Sequence"), 10, 64)
		if err != nil || sequence <= 0 || strconv.FormatInt(sequence, 10) != r.Header.Get("X-Connector-Sequence") {
			http.Error(w, "fixture refusal", 400)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, remoteprojection.MaxBytes+1))
		if err != nil || remoteprojection.ValidateNative(body, serverID) != nil {
			http.Error(w, "fixture refusal", 400)
			return
		}
		digest := sha256.Sum256(body)
		a.mu.Lock()
		defer a.mu.Unlock()
		if sequence < a.lastSequence || (sequence == a.lastSequence && digest != a.lastDigest) {
			http.Error(w, "fixture refusal", 409)
			return
		}
		if sequence == a.lastSequence {
			a.duplicates++
		} else {
			a.lastSequence = sequence
			a.lastDigest = digest
			a.accepted++
		}
		var fields struct {
			Host struct {
				CPU struct {
					Quality struct {
						State string `json:"state"`
					} `json:"quality"`
				} `json:"cpu"`
				Memory struct {
					Quality struct {
						State string `json:"state"`
					} `json:"quality"`
					Total *uint64 `json:"total_bytes"`
				} `json:"memory"`
				Uptime struct {
					Quality struct {
						State string `json:"state"`
					} `json:"quality"`
				} `json:"uptime"`
			} `json:"host"`
		}
		if json.Unmarshal(body, &fields) != nil {
			http.Error(w, "fixture refusal", 400)
			return
		}
		useful := fields.Host.CPU.Quality.State == "AVAILABLE" && (fields.Host.Memory.Quality.State == "AVAILABLE" || fields.Host.Memory.Quality.State == "DEGRADED") && fields.Host.Memory.Total != nil && *fields.Host.Memory.Total > 0 && fields.Host.Uptime.Quality.State == "AVAILABLE"
		if a.write != nil && a.write(activationCounts{a.lastSequence, a.accepted, a.duplicates, useful}) != nil {
			http.Error(w, "fixture refusal", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"server_id":"%s","sequence":%d}`, serverID, sequence)
	})
}
