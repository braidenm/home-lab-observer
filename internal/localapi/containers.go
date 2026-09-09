package localapi

import (
	"net/http"
	"time"

	"github.com/braidenm/home-lab-observer/internal/containerobs"
)

const containerResponseLimit = 1_048_576

func (h *handler) containers(w http.ResponseWriter, r *http.Request) {
	query, err := parseContainerQuery(r.URL.RawQuery)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "INVALID_QUERY", "Invalid query")
		return
	}
	inventory := containerobs.Disabled()
	if h.config.ContainerSource != nil {
		inventory = h.config.ContainerSource.Current()
	}
	inventory = projectContainerInventory(inventory, query.limit, h.config.Now())
	h.writeJSON(w, r, http.StatusOK, inventory, containerResponseLimit)
}

func projectContainerInventory(inventory containerobs.Inventory, limit int, now time.Time) containerobs.Inventory {
	inventory.SchemaVersion = containerobs.SchemaVersion
	inventory.Policy = containerobs.Policy{ReadOnly: true, DataClassification: "LOCAL_SENSITIVE", RemoteUploadEligible: false}
	items := inventory.Items
	if items == nil {
		items = []containerobs.Container{}
	}
	total := max(inventory.TotalCount, len(items))
	if len(items) > containerobs.MaxContainers {
		items = items[:containerobs.MaxContainers]
		inventory.Truncated = true
	}
	if len(items) > limit {
		items = items[:limit]
		inventory.Truncated = true
	}
	inventory.Items = cloneContainers(items)
	inventory.TotalCount = total
	inventory.ReturnedCount = len(items)
	inventory.Truncated = inventory.Truncated || total > len(items)
	if inventory.SupportState == "SUPPORTED" && inventory.ObservedAt != nil && isStale(*inventory.ObservedAt, now) {
		inventory.Freshness = "STALE"
		if inventory.ReasonCode == nil {
			inventory.ReasonCode = stringPointer("LATEST_SAMPLE_STALE")
		}
	}
	if inventory.ReasonCode != nil {
		inventory.ReasonCode = stringPointer(*inventory.ReasonCode)
	}
	if inventory.ObservedAt != nil {
		observedAt := inventory.ObservedAt.UTC()
		inventory.ObservedAt = &observedAt
	}
	return inventory
}

func cloneContainers(items []containerobs.Container) []containerobs.Container {
	result := make([]containerobs.Container, len(items))
	for index, item := range items {
		result[index] = item
		if item.CPUPercent != nil {
			value := *item.CPUPercent
			result[index].CPUPercent = &value
		}
		if item.MemoryBytes != nil {
			value := *item.MemoryBytes
			result[index].MemoryBytes = &value
		}
		if item.ReasonCode != nil {
			result[index].ReasonCode = stringPointer(*item.ReasonCode)
		}
	}
	return result
}
