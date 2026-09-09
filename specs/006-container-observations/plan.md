# Implementation plan

Keep the existing host snapshot and numeric store unchanged. A small `internal/containerobs` module owns the Docker
transport, safe normalized inventory and immutable current cache. The foreground collection wrapper invokes it within
the existing single-flight loop, with its own deadline. HTTP reads its cache, never queries Docker on request.

The closed `observer-container-inventory/v1` schema is a specialized read model. Unlike legacy container rows it can
represent inventory without pretending missing CPU/memory values are zero. The reusable UI adds an optional
`getContainerInventory` method and independently loads that model; older data sources continue using legacy containers.

Work slices: (1) spec/ADR/contracts, (2) isolated collector/transport, (3) API/UI, (4) CLI/docs/real-engine CI integration,
(5) independent review and merge. OS services/logs/sensors follow later specs; packaging remains the next delivery milestone.
