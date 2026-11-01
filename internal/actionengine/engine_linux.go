//go:build linux

package actionengine

import (
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/braidenm/home-lab-observer/internal/actioncore"
)

type Engine struct {
	endpoint  socketEndpoint
	client    *http.Client
	transport *http.Transport
	mu        sync.Mutex
	closed    bool
	lifetime  context.Context
	cancel    context.CancelFunc
	active    sync.WaitGroup
}

var _ actioncore.Engine = (*Engine)(nil)

// Open never discovers/adopts a socket or consults Docker environment/context.
// Its fixed rootful namespace is not installed or activated by this library.
func Open(ctx context.Context, approved SocketWitness) (*Engine, error) {
	if approved.GroupID == 0 || approved.Inode == 0 || !directory("/", 0) || !directory("/engine", 0) {
		return nil, ErrUnsafe
	}
	return openEndpoint(ctx, socketEndpoint{path: socketPath, owner: 0, approved: approved})
}

// Shared internal construction takes an already captured endpoint expectation.
// Public Open fixes the path/rootful owner/ancestry; private Unix fixtures supply
// their own protected directory and captured owner, never a validation-skip flag.
func openEndpoint(ctx context.Context, endpoint socketEndpoint) (*Engine, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	if err := endpoint.check(); err != nil {
		return nil, err
	}
	a := &Engine{endpoint: endpoint}
	a.lifetime, a.cancel = context.WithCancel(context.Background())
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	a.transport = &http.Transport{
		Proxy: nil, Protocols: protocols, DisableKeepAlives: true, DisableCompression: true,
		MaxConnsPerHost: 2, MaxResponseHeaderBytes: 16 * 1024, ResponseHeaderTimeout: 45 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" || address != "docker:80" || endpoint.check() != nil {
				return nil, ErrUnsafe
			}
			conn, err := dialer.DialContext(ctx, "unix", endpoint.path)
			if err != nil {
				return nil, ErrUnavailable
			}
			if endpoint.check() != nil {
				conn.Close()
				return nil, ErrUnsafe
			}
			return conn, nil
		},
	}
	a.client = &http.Client{Transport: a.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	body, status, err := a.request(ctx, "GET", "/version", 4*time.Second, versionLimit)
	if err == nil && status == http.StatusOK {
		err = supportedVersion(body)
	} else if err == nil {
		err = ErrUnavailable
	}
	if err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

func (a *Engine) Inspect(ctx context.Context, id string) (actioncore.State, error) {
	if !validID(id) {
		return actioncore.UnknownState, ErrInvalid
	}
	body, status, err := a.request(ctx, "GET", "/v1.45/containers/"+id+"/json", 4*time.Second, inspectLimit)
	if err != nil {
		return actioncore.UnknownState, err
	}
	if status == http.StatusNotFound {
		return actioncore.UnknownState, nil
	}
	if status != http.StatusOK {
		return actioncore.UnknownState, ErrUnavailable
	}
	return inspectState(body, id)
}
func (a *Engine) Start(ctx context.Context, id string) error { return a.mutate(ctx, id, "start", 0) }
func (a *Engine) Stop(ctx context.Context, id string, grace uint8) error {
	return a.mutate(ctx, id, "stop", grace)
}
func (a *Engine) Restart(ctx context.Context, id string, grace uint8) error {
	return a.mutate(ctx, id, "restart", grace)
}
func (a *Engine) mutate(ctx context.Context, id, operation string, grace uint8) error {
	if !validID(id) {
		return ErrInvalid
	}
	path := "/v1.45/containers/" + id + "/"
	switch operation {
	case "start":
		if grace != 0 {
			return ErrInvalid
		}
		path += "start"
	case "stop", "restart":
		if grace < 1 || grace > 30 {
			return ErrInvalid
		}
		path += operation + "?t=" + strconv.Itoa(int(grace))
	default:
		return ErrInvalid
	}
	_, status, err := a.request(ctx, "POST", path, 45*time.Second, 0)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return ErrUnavailable
	}
	return nil
}

func (a *Engine) request(parent context.Context, method, path string, timeout time.Duration, limit int64) ([]byte, int, error) {
	if a == nil || parent == nil || parent.Err() != nil {
		return nil, 0, ErrUnavailable
	}
	a.mu.Lock()
	if a.closed || a.client == nil {
		a.mu.Unlock()
		return nil, 0, ErrUnavailable
	}
	a.active.Add(1)
	a.mu.Unlock()
	defer a.active.Done()
	ctx, cancel := context.WithTimeout(parent, timeout)
	stop := context.AfterFunc(a.lifetime, cancel)
	defer func() { stop(); cancel() }()
	if a.endpoint.check() != nil {
		return nil, 0, ErrUnsafe
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://docker"+path, nil)
	if err != nil {
		return nil, 0, ErrUnavailable
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "observer-action-engine/v1")
	response, err := a.client.Do(req)
	if err != nil {
		return nil, 0, ErrUnavailable
	}
	defer response.Body.Close()
	if ctx.Err() != nil {
		return nil, 0, ErrUnavailable
	}
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return nil, 0, ErrUnavailable
	}
	if method == "POST" || response.StatusCode != http.StatusOK {
		// Never decode or drain raw daemon errors; connections cannot be reused.
		return nil, response.StatusCode, nil
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit || ctx.Err() != nil {
		return nil, 0, ErrUnavailable
	}
	return body, response.StatusCode, nil
}

func (a *Engine) Close() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	a.closed = true
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
	a.active.Wait()
	if a.transport != nil {
		a.transport.CloseIdleConnections()
	}
	return nil
}
