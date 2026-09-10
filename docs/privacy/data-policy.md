# Data policy

This policy defines first-release defaults. The effective local configuration and Platform upload projection must be visible in the product.

The source preview has no uploader and sends no observations to Platform Demo. It implements host/process current
reads, six aggregate numeric host history metrics, and explicitly enabled local Docker inventory/resource readings.
Process and container observations are local-sensitive, current-memory-only and not upload-eligible in this preview.
Service and log collection below remain planned capabilities. The table describes first-release targets; future history
or upload projections need their own reviewed specifications before becoming active.

| Data class | Collected by default | Kept locally | Eligible for remote upload | Notes |
| --- | --- | --- | --- | --- |
| Aggregate CPU, memory, disk, network, uptime | Yes | Bounded trends | Yes | Base units, bounded dimensions |
| Filesystem capacity | Yes | Current + aggregate trends | Yes | Sanitized display label; no arbitrary paths remotely |
| Process identity/resource use | Bounded top-N | Current only | Bounded current snapshot | No argv, environment, executable path, user identity |
| Service identity/state | Platform capability | Current + transitions | Bounded current snapshot | Names treated as sensitive metadata |
| Container identity/state/resource use | When local Docker enabled | Current memory only | No in this preview | No environment, labels, mounts, command, raw socket; history/upload need a new reviewed spec |
| Log timestamps/severity/source/event-code/count | Explicit safe-source opt-in | Event codes in bounded memory; fixed source/severity rollups | No in Spec 009 | No bodies, identities or message fingerprints |
| Log message body | No | Local-only opt-in | No in first release | Per-source allowlist, redaction, 2 KiB record ceiling |
| Observer health and upload status | Yes | Current + bounded events | Yes | No credentials or raw exception text |
| Enrollment/connector credential | Required only for remote mode | OS-protected credential store | Authentication use only | Never returned by APIs/UI/logs/support bundle |

Default local history targets seven days or 250 MiB, whichever is reached first. Incremental maintenance reclaims old
rows and physical SQLite pages; the budget includes the active database and its WAL/SHM files. Temporary overshoot can
occur between bounded maintenance passes. Preserved corruption quarantines are not automatically deleted or counted as
active history. User-facing reduced/disabled retention configuration is planned; the preview uses fixed defaults.
Remote retention and deletion are owned by the receiving application and must be disclosed before enrollment.

Redaction is defense in depth, not a guarantee that arbitrary text is anonymous. For that reason, arbitrary logs and attributes remain excluded even when common secret patterns can be filtered.

## Spec 009 implementation boundary

Native metadata is in development, not enabled in preview 2. The accepted slice never acquires message bodies, even
when a current-snapshot caller requests them. Current codes/timestamps stay in one 200-record session ring; persisted
data contains only low-cardinality counts, coverage, latest attempt state and a private continuation checkpoint.
The checkpoint never reaches API/UI/diagnostics and travels to fixed native helpers through bounded private pipes.
Windows System/Application and the caller-accessible Linux system-journal view require explicit local configuration.
Linux inaccessible files may be omitted by the OS; coverage is not a claim about all machine events. No native log
metadata is upload-eligible. Raw-body, service-specific, macOS and remote-log behavior require later specifications.
