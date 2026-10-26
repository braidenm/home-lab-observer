package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

const bootstrapRoot = "/var/lib/hlo-first-install-fixture"
const bootstrapPayload = "/mnt/hlo-first-install"

func bootstrapMode(mode string) bool {
	switch mode {
	case "bootstrap-helper-corrupt", "bootstrap-corrupt", "bootstrap-downgrade", "bootstrap-loop", "bootstrap-valid":
		return true
	}
	return false
}

type bootstrapCounts struct {
	Helpers     int `json:"helpers"`
	Archives    int `json:"archives"`
	Redirects   int `json:"redirects"`
	Enrollments int `json:"enrollments"`
	HTTPAccepts int `json:"http_accepts"`
	Refused     int `json:"refused"`
}

type bootstrapReceiver struct {
	mu              sync.Mutex
	mode, version   string
	helper, archive []byte
	counts          bootstrapCounts
	persist         func(bootstrapCounts) error
}

func (b *bootstrapReceiver) count(update func(*bootstrapCounts)) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	next := b.counts
	update(&next)
	for _, value := range []int{next.Helpers, next.Archives, next.Redirects, next.Enrollments, next.HTTPAccepts, next.Refused} {
		if value < 0 || value > 128 {
			return errors.New("fixture count limit")
		}
	}
	if err := b.persist(next); err != nil {
		return err
	}
	b.counts = next
	return nil
}

func loadBootstrapReceiver(mode string) (*bootstrapReceiver, error) {
	manifest, err := os.ReadFile(bootstrapPayload + "/connected-manifest.json")
	if err != nil {
		return nil, err
	}
	var identity struct {
		Version string `json:"version"`
	}
	if len(manifest) > 16<<10 || json.Unmarshal(manifest, &identity) != nil || !regexp.MustCompile(`^0\.(0|[1-9][0-9]{0,3})\.(0|[1-9][0-9]{0,3})-canary\.[1-9][0-9]{0,3}$`).MatchString(identity.Version) || !bootstrapMode(mode) {
		return nil, errors.New("fixture identity")
	}
	b := &bootstrapReceiver{mode: mode, version: identity.Version}
	read := func(name string, max int64) ([]byte, error) {
		file, err := os.Open(bootstrapPayload + "/" + name)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, max+1))
		if err != nil || len(data) < 1 || int64(len(data)) > max {
			return nil, errors.New("fixture asset")
		}
		return data, nil
	}
	if b.helper, err = read("install-connected-linux.sh", 64<<10); err != nil {
		return nil, err
	}
	if b.archive, err = read("home-lab-observer-connected_"+b.version+"_linux_amd64.tar.gz", 256<<20); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(bootstrapRoot + "/bootstrap-counts.json")
	if err == nil {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if len(data) > 1024 || decoder.Decode(&b.counts) != nil || decoder.Decode(new(any)) != io.EOF {
			return nil, errors.New("fixture counts")
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	b.persist = func(counts bootstrapCounts) error {
		data, err := json.Marshal(counts)
		if err != nil {
			return err
		}
		if err := os.WriteFile(bootstrapRoot+"/bootstrap-counts.next", data, 0600); err != nil {
			return err
		}
		return os.Rename(bootstrapRoot+"/bootstrap-counts.next", bootstrapRoot+"/bootstrap-counts.json")
	}
	return b, b.count(func(*bootstrapCounts) {})
}

func (b *bootstrapReceiver) handler(enrollment http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refuse := func() {
			if b.count(func(c *bootstrapCounts) { c.Refused++ }) != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}
		if r.TLS == nil || r.TLS.ServerName != r.Host || r.URL.RawQuery != "" || r.URL.RawPath != "" || len(r.Header.Values("Authorization")) != 0 || len(r.Header.Values("Cookie")) != 0 || len(r.Header.Values("Proxy-Authorization")) != 0 {
			refuse()
			return
		}
		if r.Host == dnsHostname && r.URL.Path == path && r.Method == http.MethodPost {
			if b.count(func(c *bootstrapCounts) { c.Enrollments++ }) != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			enrollment.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet {
			refuse()
			return
		}
		prefix := "/braidenm/home-lab-observer/releases/download/v" + b.version + "/"
		archiveName := "home-lab-observer-connected_" + b.version + "_linux_amd64.tar.gz"
		var body []byte
		var update func(*bootstrapCounts)
		switch {
		case r.Host == "github.com" && r.URL.Path == prefix+"install-connected-linux.sh":
			body, update = b.helper, func(c *bootstrapCounts) { c.Helpers++ }
			if b.mode == "bootstrap-helper-corrupt" {
				body = append([]byte(nil), body...)
				body[len(body)-1] ^= 1
			}
		case r.Host == "github.com" && r.URL.Path == prefix+archiveName:
			location := "https://release-assets.githubusercontent.com/hlo-fixture/" + archiveName
			if b.mode == "bootstrap-downgrade" {
				location = "http://release-assets.githubusercontent.com/hlo-fixture/" + archiveName
			}
			if b.mode == "bootstrap-loop" {
				location = "https://github.com" + prefix + archiveName
			}
			if b.count(func(c *bootstrapCounts) { c.Redirects++ }) != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Location", location)
			w.WriteHeader(http.StatusFound)
			return
		case r.Host == "release-assets.githubusercontent.com" && r.URL.Path == "/hlo-fixture/"+archiveName && (b.mode == "bootstrap-valid" || b.mode == "bootstrap-corrupt"):
			body, update = b.archive, func(c *bootstrapCounts) { c.Archives++ }
			if b.mode == "bootstrap-corrupt" {
				body = append([]byte(nil), body...)
				body[len(body)-1] ^= 1
			}
		default:
			refuse()
			return
		}
		if b.count(update) != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(body)
	})
}

type bootstrapHTTPListener struct {
	net.Listener
	receiver *bootstrapReceiver
}

func (l *bootstrapHTTPListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if err := l.receiver.count(func(c *bootstrapCounts) { c.HTTPAccepts++ }); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func (b *bootstrapReceiver) serveHTTPTrap(listener net.Listener) error {
	server := &http.Server{ReadHeaderTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second, MaxHeaderBytes: 4096,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) })}
	return server.Serve(&bootstrapHTTPListener{Listener: listener, receiver: b})
}
