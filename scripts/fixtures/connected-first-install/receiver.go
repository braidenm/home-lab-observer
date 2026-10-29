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
	"net"
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
	if len(os.Args) != 5 || os.Args[1] != "93.184.216.34:443" || (os.Args[4] != "normal" && os.Args[4] != "interrupt" && os.Args[4] != "activation" && os.Args[4] != "rich-processes" && os.Args[4] != "rich-docker" && !bootstrapMode(os.Args[4])) {
		fmt.Fprintln(os.Stderr, "FIXTURE_ARGS_REFUSED")
		os.Exit(22)
	}
	var consumed atomic.Bool
	interrupt := os.Args[4] == "interrupt"
	handler := fixtureHandler(&consumed, interrupt)
	bootstrap := bootstrapMode(os.Args[4])
	if bootstrap {
		download, err := loadBootstrapReceiver(os.Args[4])
		if err != nil {
			fmt.Fprintln(os.Stderr, "FIXTURE_BOOTSTRAP_REFUSED")
			os.Exit(1)
		}
		handler = download.handler(handler)
		trap, err := net.Listen("tcp4", "93.184.216.34:80")
		if err != nil {
			fmt.Fprintln(os.Stderr, "FIXTURE_HTTP_TRAP_FAILED")
			os.Exit(1)
		}
		defer trap.Close()
		go func() {
			if download.serveHTTPTrap(trap) != nil {
				fmt.Fprintln(os.Stderr, "FIXTURE_HTTP_TRAP_FAILED")
				os.Exit(1)
			}
		}()
	}
	if os.Args[4] == "activation" {
		admitted, err := restoreFixtureAdmission()
		if err != nil {
			fmt.Fprintln(os.Stderr, "FIXTURE_ADMISSION_REFUSED")
			os.Exit(1)
		}
		consumed.Store(admitted)
		handler = fixtureHandlerWithAdmission(&consumed, false, persistFixtureAdmission)
		observed := &activationReceiver{enrolled: &consumed, write: func(counts activationCounts) error {
			data, err := json.Marshal(counts)
			if err != nil {
				return err
			}
			if err := os.WriteFile("/var/lib/hlo-first-install-fixture/receiver-counts.next", data, 0600); err != nil {
				return err
			}
			return os.Rename("/var/lib/hlo-first-install-fixture/receiver-counts.next", "/var/lib/hlo-first-install-fixture/receiver-counts.json")
		}}
		handler = observed.handler(handler)
	}
	if os.Args[4] == "rich-processes" {
		observed, err := restoreRichReceiver()
		if err != nil {
			fmt.Fprintln(os.Stderr, "FIXTURE_RICH_ADMISSION_REFUSED")
			os.Exit(1)
		}
		handler = observed.handler()
	}
	if os.Args[4] == "rich-docker" {
		observed, err := restoreDockerReceiver()
		if err != nil {
			fmt.Fprintln(os.Stderr, "FIXTURE_DOCKER_ADMISSION_REFUSED")
			os.Exit(1)
		}
		handler = observed.handler()
	}
	dns, err := net.ListenPacket("udp4", "127.0.0.1:53")
	if err != nil {
		fmt.Fprintln(os.Stderr, "FIXTURE_DNS_LISTEN_FAILED")
		os.Exit(1)
	}
	defer dns.Close()
	go func() {
		if serveDNSFor(dns, bootstrap) != nil {
			fmt.Fprintln(os.Stderr, "FIXTURE_DNS_FAILED")
			os.Exit(1)
		}
	}()
	server := &http.Server{
		Addr:              os.Args[1],
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		MaxHeaderBytes:    4096,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
		Handler:           handler,
	}
	if err := server.ListenAndServeTLS(os.Args[2], os.Args[3]); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "FIXTURE_LISTEN_FAILED")
		os.Exit(1)
	}
}

func fixtureHandler(consumed *atomic.Bool, interrupt bool) http.Handler {
	return fixtureHandlerWithAdmission(consumed, interrupt, nil)
}

func fixtureHandlerWithAdmission(consumed *atomic.Bool, interrupt bool, persist func(string) error) http.Handler {
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
		if persist != nil && persist(input.ConnectorID) != nil {
			http.Error(w, "fixture refusal", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"server_id":"`+serverID+`","connector_secret":"`+credential+`"}`)
	})
}
