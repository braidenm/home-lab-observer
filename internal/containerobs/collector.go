package containerobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxSupportedAPIMajor = 1
	maxSupportedAPIMinor = 45
	minSupportedAPIMajor = 1
	minSupportedAPIMinor = 41
	maxVersionBody       = 64 << 10
	maxInventoryBody     = 2 << 20
	maxStatsBody         = 256 << 10
	maxStatsFanout       = 4
	collectionTimeout    = 4 * time.Second
)

var containerIDPattern = regexp.MustCompile(`^[[:xdigit:]]{12,128}$`)

type Config struct {
	Endpoint string
	Now      func() time.Time
}

type Collector struct {
	client    *http.Client
	transport *http.Transport
	now       func() time.Time
	disabled  bool

	collectMu sync.Mutex
	cacheMu   sync.RWMutex
	current   Inventory
	priorMu   sync.Mutex
	priorCPU  map[string]cpuCounters
}

func New(config Config) (*Collector, error) {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	if config.Endpoint == "" {
		return &Collector{now: now, disabled: true, current: Disabled(), priorCPU: make(map[string]cpuCounters)}, nil
	}
	client, transport, err := newLocalClient(config.Endpoint)
	if err != nil {
		return nil, err
	}
	return &Collector{client: client, transport: transport, now: now, current: notRun("COLLECTOR_NOT_RUN"), priorCPU: make(map[string]cpuCounters)}, nil
}

func newCollectorForTest(client *http.Client, now func() time.Time) *Collector {
	if client.CheckRedirect == nil {
		client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	}
	return &Collector{client: client, now: now, current: notRun("COLLECTOR_NOT_RUN"), priorCPU: make(map[string]cpuCounters)}
}

func (c *Collector) Collect(ctx context.Context) Inventory {
	c.collectMu.Lock()
	defer c.collectMu.Unlock()
	if c.disabled {
		return c.Current()
	}
	cycleCtx, cancel := context.WithTimeout(ctx, collectionTimeout)
	defer cancel()
	result := c.collect(cycleCtx)
	c.cacheMu.Lock()
	c.current = cloneInventory(result)
	c.cacheMu.Unlock()
	return cloneInventory(result)
}

func (c *Collector) Current() Inventory {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return cloneInventory(c.current)
}

func (c *Collector) Close() error {
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
	return nil
}

func (c *Collector) collect(ctx context.Context) Inventory {
	version, engineOS, err := c.negotiateVersion(ctx)
	if err != nil {
		return failedInventory(err)
	}
	containers, err := c.listContainers(ctx, version)
	if err != nil {
		return failedInventory(err)
	}
	total := len(containers)
	if len(containers) > MaxContainers {
		containers = containers[:MaxContainers]
	}
	defer c.pruneCPU(containers)

	items := make([]Container, len(containers))
	jobs := make(chan int)
	var workers sync.WaitGroup
	workerCount := min(maxStatsFanout, len(containers))
	for worker := 0; worker < workerCount; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				items[index] = c.observeContainer(ctx, version, engineOS, containers[index])
			}
		}()
	}
	for index := range containers {
		select {
		case jobs <- index:
		case <-ctx.Done():
			for remaining := index; remaining < len(containers); remaining++ {
				items[remaining] = normalizedContainer(containers[remaining], nil, nil, reasonPointer(reasonCode(ctx.Err())))
			}
			close(jobs)
			workers.Wait()
			return successfulInventory(c.now(), items, total, total > len(items))
		}
	}
	close(jobs)
	workers.Wait()
	sort.Slice(items, func(left, right int) bool { return items[left].IDAlias < items[right].IDAlias })
	return successfulInventory(c.now(), items, total, total > len(items))
}

type engineContainer struct {
	ID    string   `json:"Id"`
	Names []string `json:"Names"`
	Image string   `json:"Image"`
	State string   `json:"State"`
}

type engineVersion struct {
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
	OS            string `json:"Os"`
}

type engineStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  *uint64  `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage *uint64 `json:"system_cpu_usage"`
		OnlineCPUs  uint64  `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage *uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage *uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage *uint64 `json:"usage"`
	} `json:"memory_stats"`
}

type cpuCounters struct {
	total    uint64
	system   uint64
	capacity uint64
}

func (c *Collector) negotiateVersion(ctx context.Context) (string, string, error) {
	var response engineVersion
	if err := c.getJSON(ctx, "/version", maxVersionBody, &response); err != nil {
		return "", "", err
	}
	major, minor, ok := parseAPIVersion(response.APIVersion)
	if !ok || lessVersion(major, minor, minSupportedAPIMajor, minSupportedAPIMinor) {
		return "", "", protocolError{code: "API_VERSION_UNSUPPORTED", unsupported: true}
	}
	minimumMajor, minimumMinor := 0, 0
	if response.MinAPIVersion != "" {
		var validMinimum bool
		minimumMajor, minimumMinor, validMinimum = parseAPIVersion(response.MinAPIVersion)
		if !validMinimum || lessVersion(maxSupportedAPIMajor, maxSupportedAPIMinor, minimumMajor, minimumMinor) {
			return "", "", protocolError{code: "API_VERSION_UNSUPPORTED", unsupported: true}
		}
	}
	if lessVersion(maxSupportedAPIMajor, maxSupportedAPIMinor, major, minor) {
		major, minor = maxSupportedAPIMajor, maxSupportedAPIMinor
	}
	if response.MinAPIVersion != "" && lessVersion(major, minor, minimumMajor, minimumMinor) {
		return "", "", protocolError{code: "API_VERSION_UNSUPPORTED", unsupported: true}
	}
	return "v" + strconv.Itoa(major) + "." + strconv.Itoa(minor), strings.ToLower(strings.TrimSpace(response.OS)), nil
}

func (c *Collector) listContainers(ctx context.Context, version string) ([]engineContainer, error) {
	var response []engineContainer
	if err := c.getJSON(ctx, "/"+version+"/containers/json?all=1", maxInventoryBody, &response); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(response))
	for _, item := range response {
		if !containerIDPattern.MatchString(item.ID) {
			return nil, protocolError{code: "INVALID_RESPONSE"}
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, protocolError{code: "INVALID_RESPONSE"}
		}
		seen[item.ID] = struct{}{}
	}
	return response, nil
}

func (c *Collector) observeContainer(ctx context.Context, version, engineOS string, source engineContainer) Container {
	if normalizeState(source.State) != StateRunning {
		return normalizedContainer(source, nil, nil, reasonPointer("NOT_RUNNING"))
	}
	if engineOS != "linux" {
		return normalizedContainer(source, nil, nil, reasonPointer("ENGINE_OS_UNSUPPORTED"))
	}
	var stats engineStats
	path := "/" + version + "/containers/" + url.PathEscape(source.ID) + "/stats?one-shot=true&stream=false"
	if err := c.getJSON(ctx, path, maxStatsBody, &stats); err != nil {
		return normalizedContainer(source, nil, nil, reasonPointer(reasonCode(err)))
	}
	alias := aliasFor(source.ID)
	c.priorMu.Lock()
	previous, hasPrevious := c.priorCPU[alias]
	c.priorMu.Unlock()
	cpu := cpuPercent(&stats, previous, hasPrevious)
	if current, ok := currentCPUCounters(&stats); ok {
		c.priorMu.Lock()
		c.priorCPU[alias] = current
		c.priorMu.Unlock()
	}
	return normalizedContainer(source, &stats, cpu, nil)
}

func (c *Collector) getJSON(ctx context.Context, path string, maxBytes int64, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker"+path, nil)
	if err != nil {
		return protocolError{code: "INVALID_REQUEST"}
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		return protocolError{code: "REDIRECT_REJECTED"}
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return protocolError{code: "PERMISSION_DENIED", permission: true}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return protocolError{code: "ENGINE_RESPONSE_ERROR"}
	}
	if response.ContentLength > maxBytes {
		return protocolError{code: "RESPONSE_TOO_LARGE"}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return protocolError{code: "ENGINE_UNAVAILABLE"}
	}
	if int64(len(body)) > maxBytes {
		return protocolError{code: "RESPONSE_TOO_LARGE"}
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	if err := decoder.Decode(destination); err != nil {
		return protocolError{code: "INVALID_RESPONSE"}
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return protocolError{code: "INVALID_RESPONSE"}
		}
		return protocolError{code: "INVALID_RESPONSE"}
	}
	return nil
}

func normalizedContainer(source engineContainer, stats *engineStats, cpu *float64, failure *string) Container {
	item := Container{
		IDAlias:      aliasFor(source.ID),
		Name:         sanitizeMetadata(firstName(source.Names), 128, "container"),
		Image:        sanitizeMetadata(source.Image, 256, "unknown"),
		State:        normalizeState(source.State),
		MetricsState: MetricsUnavailable,
		ReasonCode:   cloneString(failure),
	}
	if stats == nil {
		if item.State != StateRunning {
			item.MetricsState = MetricsNotRunning
		}
		if item.ReasonCode == nil {
			item.ReasonCode = reasonPointer("STATS_UNAVAILABLE")
		}
		return item
	}
	item.CPUPercent = cloneFloat(cpu)
	item.MemoryBytes = cloneUint64(stats.MemoryStats.Usage)
	switch {
	case item.CPUPercent != nil && item.MemoryBytes != nil:
		item.MetricsState = MetricsAvailable
		item.ReasonCode = nil
	case item.CPUPercent != nil || item.MemoryBytes != nil:
		item.MetricsState = MetricsPartial
		item.ReasonCode = reasonPointer("METRIC_COUNTER_INVALID")
	default:
		item.ReasonCode = reasonPointer("METRIC_COUNTER_INVALID")
	}
	return item
}

func cpuPercent(stats *engineStats, prior cpuCounters, hasPrior bool) *float64 {
	current, ok := currentCPUCounters(stats)
	if !ok {
		return nil
	}
	var previousTotal uint64
	if stats.PreCPUStats.CPUUsage.TotalUsage != nil {
		previousTotal = *stats.PreCPUStats.CPUUsage.TotalUsage
	}
	var previousSystem uint64
	if stats.PreCPUStats.SystemUsage != nil {
		previousSystem = *stats.PreCPUStats.SystemUsage
	}
	previousMissing := stats.PreCPUStats.SystemUsage == nil || previousSystem == 0 || stats.PreCPUStats.CPUUsage.TotalUsage == nil
	if previousMissing && hasPrior {
		previousTotal, previousSystem = prior.total, prior.system
	}
	if previousMissing && !hasPrior {
		return nil
	}
	if current.system <= previousSystem || current.total < previousTotal {
		return nil
	}
	cpuDelta := current.total - previousTotal
	systemDelta := current.system - previousSystem
	value := float64(cpuDelta) / float64(systemDelta) * 100
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
		return nil
	}
	return &value
}

func currentCPUCounters(stats *engineStats) (cpuCounters, bool) {
	if stats.CPUStats.SystemUsage == nil {
		return cpuCounters{}, false
	}
	capacity := stats.CPUStats.OnlineCPUs
	if capacity == 0 {
		capacity = uint64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	}
	if capacity == 0 || capacity > 4096 {
		return cpuCounters{}, false
	}
	if stats.CPUStats.CPUUsage.TotalUsage == nil {
		return cpuCounters{}, false
	}
	return cpuCounters{total: *stats.CPUStats.CPUUsage.TotalUsage, system: *stats.CPUStats.SystemUsage, capacity: capacity}, true
}

func (c *Collector) pruneCPU(containers []engineContainer) {
	running := make(map[string]struct{}, len(containers))
	for _, item := range containers {
		if normalizeState(item.State) == StateRunning {
			running[aliasFor(item.ID)] = struct{}{}
		}
	}
	c.priorMu.Lock()
	defer c.priorMu.Unlock()
	for alias := range c.priorCPU {
		if _, ok := running[alias]; !ok {
			delete(c.priorCPU, alias)
		}
	}
}

func successfulInventory(observed time.Time, items []Container, total int, truncated bool) Inventory {
	state := CollectionOK
	var reason *string
	for index := range items {
		if items[index].MetricsState != MetricsAvailable && items[index].ReasonCode != nil && *items[index].ReasonCode != "NOT_RUNNING" {
			state = CollectionPartial
			reason = reasonPointer("STATS_PARTIAL")
			break
		}
	}
	observed = observed.UTC()
	return Inventory{SchemaVersion: SchemaVersion, ObservedAt: &observed, SupportState: SupportSupported, CollectionState: state, Freshness: FreshnessCurrent, ReasonCode: reason, TotalCount: total, ReturnedCount: len(items), Truncated: truncated, Items: items, Policy: dataPolicy()}
}

func failedInventory(err error) Inventory {
	support := SupportUnavailable
	if isPermission(err) {
		support = SupportPermissionDenied
	} else if isUnsupported(err) {
		support = SupportUnsupported
	}
	reason := reasonCode(err)
	return Inventory{SchemaVersion: SchemaVersion, SupportState: support, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown, ReasonCode: &reason, Items: []Container{}, Policy: dataPolicy()}
}

func notRun(reason string) Inventory {
	return Inventory{SchemaVersion: SchemaVersion, SupportState: SupportUnavailable, CollectionState: CollectionNotRun, Freshness: FreshnessUnknown, ReasonCode: &reason, Items: []Container{}, Policy: dataPolicy()}
}

type protocolError struct {
	code        string
	permission  bool
	unsupported bool
}

func (e protocolError) Error() string { return e.code }

func reasonCode(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "DEADLINE_EXCEEDED"
	}
	if errors.Is(err, context.Canceled) {
		return "COLLECTION_CANCELED"
	}
	if isPermission(err) {
		return "PERMISSION_DENIED"
	}
	var typed protocolError
	if errors.As(err, &typed) {
		return typed.code
	}
	return "ENGINE_UNAVAILABLE"
}

func isPermission(err error) bool {
	var typed protocolError
	return (errors.As(err, &typed) && typed.permission) || errors.Is(err, fs.ErrPermission)
}

func isUnsupported(err error) bool {
	var typed protocolError
	return errors.As(err, &typed) && typed.unsupported
}

func parseAPIVersion(value string) (int, int, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, false
	}
	for _, part := range parts {
		for _, character := range part {
			if character < '0' || character > '9' {
				return 0, 0, false
			}
		}
	}
	major, errMajor := strconv.Atoi(parts[0])
	minor, errMinor := strconv.Atoi(parts[1])
	return major, minor, errMajor == nil && errMinor == nil && major >= 0 && minor >= 0
}

func lessVersion(major, minor, otherMajor, otherMinor int) bool {
	return major < otherMajor || (major == otherMajor && minor < otherMinor)
}

func aliasFor(id string) string {
	hash := sha256.Sum256([]byte(id))
	return "ctr_" + hex.EncodeToString(hash[:8])
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	copy := append([]string(nil), names...)
	sort.Strings(copy)
	return strings.TrimPrefix(copy[0], "/")
}

func normalizeState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case StateCreated:
		return StateCreated
	case StateRunning:
		return StateRunning
	case StatePaused:
		return StatePaused
	case StateRestarting:
		return StateRestarting
	case StateRemoving:
		return StateRemoving
	case StateExited:
		return StateExited
	case StateDead:
		return StateDead
	default:
		return StateUnknown
	}
}

func reasonPointer(value string) *string { return &value }
