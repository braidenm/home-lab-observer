# Actual Go client / Platform HTTP acceptance

2026-11-03. **Proposed test-only implementation plan; exact paired review before code.**
Consumes the four frozen worker routes; no production API, trust option, role, framework or action is added.

## Proof boundary and normal CI

Platform's existing configured provider cohort already runs a real RANDOM_PORT servlet server. Reuse that
same context and existing fully asserted worker HTTP journey. Its ordinary required execution is unchanged:
no new full context, JUnit method/tier/tag, conditional skip, disabled assertion or coverage/time exception.

An ADDITIONAL explicitly requested native acceptance step may run at the end of that journey, using a fresh
synthetic owner/server/credential/approval/request fixture, not its already completed/cooled-down action.
Absence of the native request runs the normal journey and makes NO Go-client acceptance claim. A requested
probe must actually complete and verify its database effects or FAIL; it cannot silently fall back to Java.
No current PR gate or installed-profile proof is satisfied merely by selecting this mode.
The existing journey is parameterized over START/STOP/RESTART: request at most THREE sequential native children,
one fresh fixture per operation, never a parallel pool, retry or additional unbounded repetition.

Use one closed nonsecret test-only environment value HLO_PAIRED_CLIENT_ACCEPTANCE_V1: absent means not
requested; otherwise exactly32 lowercase hex selects a prepared artifact directory, never a command/path/URL.
The fixed directory is the app module's build/paired-container-client/<id>, below its verified project root.
Only the fixed OS-native platform-container-client.test[.exe] and its fixed manifest can execute. Reject
symlinks/reparse/foreign paths, absent/changed artifacts, unknown manifest fields or source/binary hash mismatch.
Never select an arbitrary executable, shell, arguments, inherited proxy/trust override or production origin.
The preparation step records the pinned Go version, reviewed Observer source-closure digest and binary SHA256.
Those nonsecret receipts are test traceability, NOT a production attestation or a privileged installer.
The manifest has exactly format_version, go_version, source_sha256, binary_sha256 and executable. Pin the
reviewed source digest in the JVM helper after Go-source freeze; do not trust a manifest's self-asserted source.
Source closure is go.mod/go.sum and all regular files under internal/actioncore, internal/actionprotocol and
internal/actionremote, with no links. Hash sorted ASCII relative-path TAB raw-SHA256 LF records. Verify the
closure unchanged before/after compilation; Go-version admission is exactly the reviewed1.27.3 toolchain.

## Private process protocol

Build the standard Go test binary with go test -c and an explicit paired_platform_acceptance build tag.
Only its exact TestPairedPlatformClientAcceptance runs, with fixed test timeout flags. Ordinary Go tests do
not invoke that stdin-consuming test; pure fixture/allowlist rejection tests remain in the ordinary suite.
Artifact metadata is manifest.json. The fixed flags are -test.run=^TestPairedPlatformClientAcceptance$
and -test.timeout=40s, with no verbose output. After actual cleanup the child prints only
HLO_PAIRED_CLIENT_ACCEPTANCE_PASS followed by the standard Go PASS status.

JVM sends one closed JSON object through the child's PRIVATE stdin, then closes it: format_version1,
app_port, server_id, owner_user_id, connector_instance_id, action_id, target_id, operation and action_secret.
Cap input at16384 bytes, reject unknown/duplicate/trailing/invalid values, and never persist/echo this body.
The target is ALWAYS http://127.0.0.1:<captured actual servlet port>; no hostname/URL is supplied by input.
Only synthetic credentials issued by this same test server are used. No production account, token or endpoint.

The child clears inherited network/trust/provider configuration and holds only this one synthetic binding.
It emits one finite success marker after every assertion and cleanup, plus normal finite Go test status.
JVM bounds stdout, discards raw stderr, validates the closed marker/exit result and never includes raw output
or the input in a failure. Timeout/output overflow terminates and JOINS only this owned test process and any
owned descendants; no detached reader/process is called joined merely because cancellation was requested.
Fixed child40s and outer45s bounds do not alter runtime limits. Cleanup makes bounded terminate/join attempts;
failure returns a finite NOT_JOINED result and preserves diagnostic ownership, never success or a false guarantee
that an uncooperative process terminated. No later probe starts after any cleanup failure.

## Actual TLS client, bounded forwarder, actual app

The Go fixture owns one loopback TLS httptest server with its test certificate. Use the EXISTING PRIVATE
newClient fixture-root seam, not a new exported transport, InsecureSkipVerify or production trust override.
Its handler is a bounded standard ReverseProxy with Rewrite to ONLY the captured app loopback port.
No fabricated success body, permission, timestamp, claim or result response is permitted: actual app responses
pass through unchanged. Reject every method/path except the four exact routes of this exact synthetic server/
action, all query/escaped-path variants, wrong instance/auth headers, oversized body or unexpected requests.
Preserve the Go client's actual auth/header/body; never insert missing proof to make a test pass.

Disable ambient proxies, redirects, compression and keepalive; bound headers/body, dial/response time and
connections. Forwarder errors are a finite failure, never a synthetic protocol-success response. Join/cancel
all requests and close owned client/server/transport before emitting success. No public listener or general proxy.

Actual Client Claim must bind the expected request/action and nonce; both Check1/Check2 replies must bind fresh
nonces/full receipt/lease; Report and equal retry must return the same original receipt; Retained must match
SUCCEEDED/RELEASED and the same claim/result. Reported local outcomes are synthetic fixture evidence: no Engine
or journal is present in this client-only proof. JVM independently checks exactly three actual Axon facts and
matching authoritative lane/cooldown, using fresh unique IDs. No raw secret/body enters test artifacts/logs.

## Ownership, acceptance and limitations

Root owns the private Go fixture/allowlist/input tests and compiled artifact/source receipt. Platform author
owns the optional extra inside the existing journey, exact artifact admission, stdin/process lifecycle and
database assertions. Commit this plan FIRST, then freeze/review both halves before running the paired probe.
Prove malformed/oversized input, unsupported operation/path/method/headers, artifact/hash/exit/output/timeout
refusal and no false success marker. Re-run ordinary relevant suites with no requested native mode as well.

This proves actual Go Client plus actual Platform HTTP/SQL/authorization compatibility only. It does NOT prove
RunWorker installed role, Unix broker, Engine effect, systemd isolation, release identity or live permissions.
Those paired worker/VM/package/disposable start-stop-restart gates remain required before buttons/activation.

Primary references checked for pinned Go1.27.3: [ReverseProxy Rewrite](https://pkg.go.dev/net/http/httputil#ReverseProxy),
[loopback TLS test server](https://pkg.go.dev/net/http/httptest#NewTLSServer), and
[compile-only test binary](https://pkg.go.dev/cmd/go#hdr-Test_packages). No dependency is added.
