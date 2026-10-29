# ADR 026: Reuse the paused startup rendezvous for the Docker reader

Date: 2026-10-29

Status: Accepted engineering decision; dormant package only, not release or enrollment activation approval.

## Context

[ADR 025](025-isolated-installed-docker-observation.md) adds a fixed socket reader, but the existing descriptor
audit deliberately rejects every socket. Auditing after sampling starts would race legitimate Engine sockets;
loosening that audit would fail to prove inherited descriptor closure before the first Engine request.

## Decision

Reuse the existing bounded canonical `connectedactivation` request, response and commit records. The reader gets
one additional read-only `/activation` mount sourced from the separate root-owned `reader-activation` runtime
directory, root:FrameGID 0750, containing only root:FrameGID 0640 `request.json` and `commit.json`. It has no uploader
activation mount, writable root authority or generic message/command channel. Response uses its existing private
status directory and current invocation with a fresh retained challenge.

Identity/policy/primitive checks finish and close all socket descriptors before the response. Engine I/O and frame
publication remain blocked until the matching commit. Root audits the exact paused PID/invocation, mount bindings
and pinned approved socket both before request publication and after the fresh response before commit. Request
`ConfigSHA256` hashes the canonical `observer-connected-docker-reader-startup/v1` envelope, fixed role
`rich-docker-reader`, and exact installed configuration; an uploader's raw-config challenge cannot enter the reader
domain. Both activation directories are masked from the collector despite its service-only frame group.
Request codec port fields remain fixed inert 1024/1024 values, never
network targets or probes; any other reader values refuse. Final live checks retain exact invocation/socket/mount
identity without rerunning a socket-free audit during ordinary sampling.

## Alternatives and consequences

A second permit schema duplicates the already-reviewed nonce/challenge lifecycle. Sleep-based inspection, accepting
arbitrary Unix descriptors, SIGSTOP timing and daemon modification do not provide this closed startup proof and
are rejected. Old profiles, codecs and audits are unchanged. The new draft definition includes this reader barrier;
actual packaged VM replay, inherited-socket, cancellation and cleanup proof remains required before release.
