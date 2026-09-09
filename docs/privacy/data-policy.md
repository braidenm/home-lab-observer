# Data policy

This policy defines first-release defaults. The effective local configuration and Platform upload projection must be visible in the product.

| Data class | Collected by default | Kept locally | Eligible for remote upload | Notes |
| --- | --- | --- | --- | --- |
| Aggregate CPU, memory, disk, network, uptime | Yes | Bounded trends | Yes | Base units, bounded dimensions |
| Filesystem capacity | Yes | Current + aggregate trends | Yes | Sanitized display label; no arbitrary paths remotely |
| Process identity/resource use | Bounded top-N | Current only | Bounded current snapshot | No argv, environment, executable path, user identity |
| Service identity/state | Platform capability | Current + transitions | Bounded current snapshot | Names treated as sensitive metadata |
| Container identity/state/resource use | When local Docker enabled | Current + aggregate trends | Bounded current snapshot | No environment, labels, mounts, command, raw socket |
| Log timestamps/severity/source/fingerprint/count | Enabled safe sources | Bounded rollups | Future bounded metadata contract | No body in the current snapshot |
| Log message body | No | Local-only opt-in | No in first release | Per-source allowlist, redaction, 2 KiB record ceiling |
| Observer health and upload status | Yes | Current + bounded events | Yes | No credentials or raw exception text |
| Enrollment/connector credential | Required only for remote mode | OS-protected credential store | Authentication use only | Never returned by APIs/UI/logs/support bundle |

Default local history is limited to seven days or 250 MiB, whichever is reached first. Users can reduce or disable history. Remote retention and deletion are owned by the receiving application and must be disclosed before enrollment.

Redaction is defense in depth, not a guarantee that arbitrary text is anonymous. For that reason, arbitrary logs and attributes remain excluded even when common secret patterns can be filtered.
