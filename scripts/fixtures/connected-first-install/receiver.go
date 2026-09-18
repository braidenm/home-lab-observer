// This synthetic enrollment receiver runs only inside a network-isolated,
// disposable acceptance VM. It makes no outbound requests and records no grant.
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"
)

const serverID = "srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const grant = "hle_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const credential = "hlc_BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
const path = "/v1/connectors/home-lab/enrollments:exchange"

type request struct {
	Secret      string `json:"enrollment_secret"`
	ConnectorID string `json:"connector_instance_id"`
}

func main() {
	if len(os.Args) != 5 || os.Args[1] != "93.184.216.34:443" || (os.Args[4] != "normal" && os.Args[4] != "interrupt") {
		fmt.Fprintln(os.Stderr, "FIXTURE_ARGS_REFUSED")
		os.Exit(22)
	}
	var consumed atomic.Bool
	interrupt := os.Args[4] == "interrupt"
	server := &http.Server{
		Addr:              os.Args[1],
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		MaxHeaderBytes:    4096,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
		Handler:           fixtureHandler(&consumed, interrupt),
	}
	if err := server.ListenAndServeTLS(os.Args[2], os.Args[3]); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "FIXTURE_LISTEN_FAILED")
		os.Exit(1)
	}
}

func fixtureHandler(consumed *atomic.Bool, interrupt bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path || r.Method != http.MethodPost || r.URL.RawQuery != "" {
			http.Error(w, "fixture refusal", http.StatusNotFound)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 513))
		if err != nil || len(body) > 512 {
			http.Error(w, "fixture refusal", http.StatusBadRequest)
			return
		}
		var input request
		if interrupt {
			// The interruption case must never consume a grant, even if the
			// guest driver is delayed after observing durable PREPARING.
			<-r.Context().Done()
			return
		}
		if json.Unmarshal(body, &input) != nil ||
			input.Secret != grant || len(input.ConnectorID) != 38 ||
			!bytes.HasPrefix([]byte(input.ConnectorID), []byte("agent_")) ||
			consumed.Swap(true) {
			http.Error(w, "fixture refusal", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"server_id":"`+serverID+`","connector_secret":"`+credential+`"}`)
	})
}
