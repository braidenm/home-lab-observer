# Disposable first-install acceptance (not yet executed)

This fixture is a gate for the stopped Linux connected installer, not an owner-host
installation recipe. A helper passing on WSL or a hosted CI runner does **not**
substitute for the packaged Ubuntu 24.04/systemd 255 VM run and independent review.
Do not run the installer or this fixture on the Home Lab host. Do not transfer a
real enrollment grant, connector credential, user data or server snapshot into the
guest. The fixture receiver contains a synthetic one-use value only and never
contacts the Platform Demo service.

## VM admission and budget

- Obtain a current Ubuntu 24.04 amd64 cloud image and verify its published
  SHA-256 from Canonical before use. Record image URL, checksum and date in the
  PR evidence. Never use or mutate the Platform CI base image.
- Create a dedicated copy-on-write 12 GiB overlay and isolated cloud-init seed
  in a uniquely named temporary directory. Give QEMU/KVM 2 vCPU and 3 GiB RAM.
  Wait for the Platform CI guest to release host capacity. Disable every VM NIC
  (`-nic none`); use serial console for interaction. No virtiofs, 9p, host bind
  mounts, production credentials or SSH port forwarding.
- Build the exact three role binaries from the reviewed PR head with matching
  embedded `connectedpack --mode identity` records, then run `connectedpack
  --mode pack`. Record source commit, artifact SHA-256 and manifest SHA-256.
  Transfer only the checked archive, checksums and the compiled synthetic
  receiver on a **read-only virtual CD**. Mount that CD read-only inside the
  guest, verify the checksums again and copy the archive into the guest's own
  ext4 filesystem. Extract into a fresh, root-owned 0755 directory and verify
  the manifest again. No host-shared filesystem is permitted.

One way to produce the three-binary bundle from the reviewed checkout on a
Linux/amd64 build machine with Go 1.27 is below. It creates a new private
staging directory and leaves it intact for audit; do not use a production
credential or overwrite a prior release. `connectedpack` checks exact members
and writes the archive plus checksums last.

```sh
set -eu
CANARY_BUILD_DIR="$(mktemp -d)"
mkdir "$CANARY_BUILD_DIR/binaries"
CANARY_COMMIT="$(git rev-parse HEAD)"
CANARY_VERSION='0.1.0-canary.1'
CGO_ENABLED=0 go build -trimpath -buildvcs=false -o "$CANARY_BUILD_DIR/connectedpack" ./cmd/connectedpack
for CANARY_ROLE in collector uploader install; do
  CANARY_IDENTITY="$("$CANARY_BUILD_DIR/connectedpack" --mode identity --role "$CANARY_ROLE" --version "$CANARY_VERSION" --commit "$CANARY_COMMIT")"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags "-X main.releaseIdentity=$CANARY_IDENTITY" -o "$CANARY_BUILD_DIR/binaries/observer-connected-$CANARY_ROLE" "./cmd/observer-connected-$CANARY_ROLE"
done
"$CANARY_BUILD_DIR/connectedpack" --mode pack --version "$CANARY_VERSION" --commit "$CANARY_COMMIT" --binaries "$CANARY_BUILD_DIR/binaries" --output "$CANARY_BUILD_DIR/release"
(cd "$CANARY_BUILD_DIR/release" && sha256sum --check --strict SHA256SUMS)
printf 'Fixture bundle: %s\n' "$CANARY_BUILD_DIR/release"
```

Build the receiver separately with `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go
build -trimpath -o <new-staging-path>/connected-first-install-receiver
./scripts/fixtures/connected-first-install`. Transfer it on the same read-only
virtual media, but **not** as a member of the verified connected bundle.

## Guest-only network and enrollment

Inside this isolated guest only, alias `93.184.216.34/32` to loopback and map
`app.braidenmiller.com` to that alias in guest `/etc/hosts`. Confirm that the
guest has no external NIC or default route. Generate a throwaway CA and a leaf
certificate with DNS SAN `app.braidenmiller.com`, trust that CA only in the
guest's `/etc/ssl/certs/ca-certificates.crt`, and start the checked-in
`receiver.go` compiled for Linux/amd64 on `93.184.216.34:443`. The receiver
accepts one synthetic enrollment POST and returns a synthetic connector secret;
it does not implement snapshot ingestion or make outbound requests. The fixed
public address is local to the VM and must never be routed from the host.

The synthetic IDs and terminal grant are intentionally fixed:

```text
server: srv_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
grant:  hle_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
```

The grant is entered only at the installer's no-echo terminal prompt, never on
argv, in environment or shell history. The exact command inside the admitted
guest is `observer-connected-install install <verified-guest-bundle-directory>
<manifest-sha256> <synthetic-server-id>`. The install binary must report
`INSTALLED_PENDING_ACCEPTANCE`; that is *not* permission to start a worker.

## Required evidence, each on a fresh overlay

1. Before grant entry, prove wrong image/systemd, foreign principals, existing
   unit/target and bad bundle refuse without a PREPARING marker or account edits.
2. Successful stopped install: validate exact installed config/manifest and
   ownership/mode/no-ACL of every fixed role; dedicated locked, non-login users
   and groups; same ledger inode before/after promotion; no unknown bundle
   members; exact systemd fragments, no drop-ins, `LoadState=loaded`,
   `UnitFileState=disabled`, `ActiveState=inactive` for both workers; no worker
   PID, listener, outbound request or enrollment transient left. Inspect the
   actual systemd mount/namespace and credential exposure properties. Capture
   only redacted assertions, never the synthetic grant or credential bytes.
3. Reboot the guest and prove both workers still disabled/inactive, metadata
   and ledger identities unchanged, and retry refuses rather than adopting.
4. Separate fresh overlays: inject a refused pre-publish target, file/parent
   sync failure and an interrupted enrollment; prove PREPARING/residue remains,
   no installed config or active/enabled worker appears, and retry refuses.
   A synthetic-root fault test is supplementary; it does not prove a VM crash
   boundary. Record the exact interruption point and guest journal evidence.
5. Repeat with an abrupt VM power cut before final installed metadata and after
   successful stopped install. After reboot, assert no worker was activated;
   incomplete state remains recovery-required, while a completed install stays
   disabled/inactive. Never reuse an interrupted overlay for another case.

Do not delete any overlay or image until its resolved absolute path is confirmed
inside the dedicated temporary VM directory and the evidence is captured. Report
each case as pass, fail or not executed; do not upgrade helper evidence into full
VM acceptance. Activation and owner-server installation remain separate gates.
