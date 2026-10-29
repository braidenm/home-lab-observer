# Dormant fixed Docker workers — source layer 2 of 3

Date: 2026-10-30. Status: implemented locally; independent exact-layer review and exact-head CI pending.

## Outcome and boundary

Build on the implemented [bounded data plane/storage](docker-data-plane.md) with three fixed headless commands:
`observer-connected-rich-docker-collector`, `observer-connected-rich-docker-reader` and
`observer-connected-rich-docker-uploader`. Implement their complete startup, projection, delivery and enrollment
composition without an install command, published bundle, systemd resources or supported owner entry point. The third
source layer must supply complete privileged installation/packaging; these layers share one
[installed acceptance target](installable-docker-observation.md). Source availability is not permission to construct
an unsupported installation, widen an existing credential or call a draft definition approved.

## Required behavior

- Use the distinct draft `RICH_PROCESSES_DOCKER_V1` configuration/policy/definition, fixed names, resolved IDs and
  separate reader/frame identity. Existing numeric and process-only definitions, resources and default APIs stay
  unchanged. Use fresh kind2/schema3 enrollment state; never copy credentials, sequences, pending bodies or ledgers.
- Compose the existing bounded GET-only Docker adapter in the credential-free reader with fixed socket/path checks,
  joined cycle deadline and typed frame publication. Keep acquisition failures distinct from healthy-empty output.
  No listener, generic proxy, daemon changes, control or arbitrary endpoint is added. An ordinary Docker socket is
  root-equivalent authority even though this code issues GETs and later installation binds it read-only.
- Reader startup waits for the existing request/response/commit barrier before first Engine I/O or frame publication.
  Hash the versioned reader-role envelope, not the uploader's raw configuration; both root audits, exact PID/invocation
  and mount/socket admission remain the later installer's responsibility. Inert port fields grant no authority and
  are never probed. See [ADR 026](../../docs/adr/026-docker-reader-startup-barrier.md).
- Collector consumes only fresh validated frames and emits one combined process/container cycle; uploader has only
  the approved handoff, durable ledger and authenticated allowlisted transport. Closed environment, identity,
  namespace, syscall, descriptor, credential and source-policy checks remain fail-closed. Collector runtime masks
  both activation directories. The uploader cannot acquire the frame or Docker socket.
- New enrollment/transport APIs are named and require the independently approved exact profile/definition echo.
  Old constructors remain strict. Merely implementing a client does not deploy or authorize the matching server writer.

## Evidence required

Run native Windows/Linux full Go tests/vet, focused Linux race tests for changed runtime packages, all three new command
tests and dependency closures, and existing contracts/schema/foundation checks. The required Linux root reader socket
fixture proves bounded local metadata/ACL/link/replacement behavior without a real daemon. Preserve layer1's real frame
DAC/ENOSPC proof. No source fixture substitutes for actual packaged Ubuntu/Docker sandbox, lifecycle/reboot/resource,
server reader-before-writer, immutable release or owner/delegate acceptance.

## Local source evidence

- The 53 implementation/test/workflow files are exact Git blobs from reviewed full-composition source
  `70cadaf9b6bfcfd8542291385571ffa50ef8ce5b`, stacked over source layer1 `b6e6054e6ff350dd7517a34980628799fbbc5c1a`.
  No installer, unit/resource, bundle/bootstrap or packer source changed; no build reference targets a missing command.
- Go1.27.3 Windows and Ubuntu WSL full `go test -p=2 ./...`/`go vet -p=2 ./...` pass, including the actual Linux command,
  dependency-closure, namespace and reader barrier tests. Focused Linux race checks cover the eight changed internal
  runtime packages and all three worker commands; these are source tests, not installed sandbox acceptance.
- The actual root socket-witness fixture passes three times with separate fixed synthetic inputs, including approved
  socket identity, ownership/mode/link/ACL drift and replacement refusal. No real Docker daemon was accessed.
- Node22.22.3 unchanged contracts pass 80/80 plus seven schemas, 17 valid/13 invalid fixtures and real Windows
  collect-once validation. Foundation43, actionlint1.7.12, local Markdown links, exact-copy and diff checks pass.

No current worker, installation, service or public release selects this dormant runtime through a supported entry
point. Exact-layer review/CI and every installed/VM/release/hosted/owner gate remain separate.
