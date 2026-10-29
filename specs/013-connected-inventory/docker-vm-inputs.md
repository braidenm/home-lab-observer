# Docker VM input verification

Date: 2026-10-29. These are verified public fixture inputs, not a Docker-rich package/VM/release acceptance record.
No package was installed, binary executed, daemon started or host network configuration changed during verification.

## Signed Docker repository chain

The public key at `https://download.docker.com/linux/ubuntu/gpg` has the primary fingerprint independently pinned in
[official Docker documentation](https://github.com/docker/docs/blob/be718ce24184c8994a42bb327c5ac97c7c669f10/engine/install/ubuntu.md#L94-L103):
`9DC858229FC7DD38854AE2D88D81803C0EBFCD88`. The Noble `InRelease` signature verified with signing subkey
`D3306A018370199E527AE7997EA0A9C3F273FCD8`, bound to that primary. No user/global keyring was used.
The verified release identifies Origin Docker, Label Docker CE, Suite noble and date `2026-10-29T15:24:54Z`.
Docker does not publish a Codename field here; the exact signed Suite and stable/amd64 path are checked instead.

| Verified input | SHA-256 |
| --- | --- |
| Docker ASCII public key | `1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570` |
| Noble InRelease | `a3527dcfdddfeef84a3a0c2cb3deeb6e17086641b40ee72cb9ed653de910b6c4` |
| Signature-verified Release plaintext | `9a58f0abb88596ef0317a1e7e6fd9d4ef1c835d39491a9478f03acf7c52b692b` |
| Signed `stable/binary-amd64/Packages`, 489,419 bytes | `2fc6d42b1c6bb45bf45e92e70d800bc7ac77d057e21c4c8c1b205ccf8d8e1258` |

These exact amd64 records were unique in the verified Packages index. Each completed download matched its signed
filename, version, size and SHA-256 before extraction. Downloads were HTTPS-only, without redirects or retries,
bounded to the signed size, and retained in a newly created private local directory.

| Package / version | Bytes | SHA-256 |
| --- | ---: | --- |
| `docker-ce` / `5:29.9.2-1~ubuntu.24.04~noble` | 24,558,208 | `95ddf7a2d8b8c0eb82d8597d4d604e294dcf741081f2e228524d5ab9bfd7c8ae` |
| `docker-ce-cli` / `5:29.9.2-1~ubuntu.24.04~noble` | 17,595,236 | `74747b5c99000279b9b2c7eb82543b4121d8436a58aa3be7e78ac6681d74794f` |
| `containerd.io` / `2.4.2-3~ubuntu.24.04~noble` | 23,271,468 | `f811ab57d1f4bb0cce19df7d526da8c711d8ebea0447cc83da1999188e664122` |

Package URLs are under the fixed official prefix
`https://download.docker.com/linux/ubuntu/dists/noble/pool/stable/amd64/`, with the corresponding exact versioned
filenames. No static archive, rootless extras, Compose, Buildx or registry image is admitted by this receipt.

## Existing Canonical base and dependency research

The already accepted Ubuntu24.04 release-20260911 base has image SHA-256
`612b2c0cc1bc413a6cb8c38fd611794caf0f2b436c50013d8b3794db12ad7354`.
The public sibling SHA256SUMS hash `89be81c6f31ffcd63e9df433e868fc2a651475a45aeb993c0dbd2707747b0185` exactly
matches the previously signature-verified local Canonical record; its detached signature hash is
`6157c3d73e35044f21308b3daea9619e0f44895289911f004b291bfb3541e1b5`. That same signed checksum file binds the
package manifest SHA-256 `6ae2bf581f0e8babf5d6e440e53a243d51a697d975dbffec2742e9cd4564f72f` at
`https://cloud-images.ubuntu.com/releases/noble/release-20260911/ubuntu-24.04-server-cloudimg-amd64.manifest`.

The verified manifest contains libc6 `2.39-0ubuntu8.9`, libsystemd0 `255.4-1ubuntu8.17`, libseccomp2
`2.5.5-1ubuntu3.1`, iptables `1.8.10-3ubuntu2`, nftables `1.0.9-1ubuntu0.1` and AppArmor
`4.0.1really4.0.1-0ubuntu0.24.04.7`, satisfying the declared direct Docker/runtime dependency floors.
Read-only ELF inspection of the verified extracted binaries found CLI/containerd requiring libc, dockerd requiring
libsystemd+libc, runc requiring libseccomp+libc, and a static containerd-shim-runc-v2. No binary was executed.

## Source-built synthetic image input

The existing `scripts/fixtures/docker-observation` Go sleeper was built with the verified Go1.27.3 runtime,
`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`, `-trimpath -buildvcs=false -ldflags=-buildid=`. The executable is
2,075,292 bytes, SHA-256 `c57242b228d5fd6cc2174aa4c10b604111cb97af04ece219c7ebf2ef29aa163a`.
The independently pinned canonical USTAR input is 2,078,720 bytes, SHA-256
`a43627cdcc1c60a07582ba9805197fe5c78420dc17a1a7e3899901a67d822c56`. Its only member is `fixture`,
root:root, mode0555, timestamp0; canonical reconstruction rejects hidden data/extra members/links.
The source is unchanged from the accepted parent; final artifact preparation must rebuild from the exact
frozen source and confirm these pins before the VM runs. No host Docker daemon or external image was used.

Inside the guest only, Docker imports this file with fixed USER65532:65532 and ENTRYPOINT `/fixture`.
The resulting image configuration is checked in memory. The input hashes are not a claim of a reproducible
Docker image/config digest. One running and one deliberately stopped instance supply the observation workload.

This is input/dependency research, not executed guest loader or AppArmor/seccomp evidence. The actual no-NIC guest
must still validate its package/library/tool state and complete real daemon/container startup without weakening
security or fetching dependencies. API negotiation, resource bounds, four-role package, image input and both
overlay outcomes remain pending in [the manual proof plan](docker-vm-proof.md).
