# Linux libsystemd binding

Status: Bounded native-binding slice. This package is not linked into the portable observer, does not provide the
helper entrypoint/process transport, and does not establish a public support claim by itself.

`internal/journalnative` is built only for Linux amd64/arm64 and pins purego v0.10.0. Production opens only the fixed
SONAME `libsystemd.so.0`, resolves an exact symbol allowlist, and calls `sd_journal_open` with only
`SD_JOURNAL_LOCAL_ONLY | SD_JOURNAL_SYSTEM`. It accepts no library, path, namespace, field, match, or source input.
Missing libraries/symbols and loader registration failures return fixed unavailable errors without raw loader text.
The eventual separate helper owns this dynamic-loader dependency; the primary observer remains free of it.

The Go signatures mirror systemd v255 `sd-journal.h`: journal handles and C pointers use `uintptr`, C `int` uses
`int32`, `uint64_t` uses `uint64`, and `size_t` uses `uintptr`. Symbols are resolved before purego registration so a
missing symbol cannot trigger `RegisterLibFunc`'s panic path. The journal handle remains on its creating locked OS
thread through close; a wrong-thread method fails closed and a wrong-thread close does not invoke native code.

Only realtime, cursor, `PRIORITY`, and `MESSAGE_ID` are acquired. `sd_journal_get_data` returns borrowed
`FIELD=value` memory, so the binding validates the exact code-owned prefix and length before taking an owned copy.
It sets a small data threshold as a decompression hint but still applies the explicit 4 KiB value limit because
systemd documents the threshold as non-authoritative. Missing fields map only from `-ENOENT`; oversized/compressed
fields map to the fixed oversized-field sentinel; all other errno values reduce to code-owned permission,
invalid-cursor, unavailable, or read-failed errors without formatting native text.

`sd_journal_get_cursor` returns libc-allocated text. The binding scans at most 16 KiB plus one terminator, copies only
after finding a terminator, and calls the resolved `free` on every non-null result. Cursor input is private, bounded,
NUL-free, and never parsed or logged. Seek remains a positioning hint: the kernel performs seek, next, then
`sd_journal_test_cursor`; this binding does not substitute string equality.

Deterministic tests use an injected native-call table to prove fixed open flags, exact fields, prefix/size bounds,
owned copies, cursor freeing, errno reduction, close-once behavior, and wrong-thread refusal without opening any host
journal. A Linux-only integration test uses a test-compiled `sd_journal_open_directory` seam only when
`OBSERVER_TEST_SYNTHETIC_JOURNAL_DIR` names an owned synthetic journal directory. GitHub-hosted Ubuntu 24.04 CI pins
the installed `systemd-journal-remote` package to that run's verified systemd-255 candidate and converts three fixed
journal-export records into one bounded journal file. The integration test reads only that directory, proves realtime,
priority/message-ID normalization, an oversized selected-field discard, exact cursor continuation and the fixed loader
failure mapping. A body canary is present but never selected or returned. Production never contains an exported directory
opener and the test never falls back to a host journal. Helper/process identity, timeout and release-package evidence
remain separate required slices before declaring the helper supported.

Primary ABI references verified 2026-09-10:

- systemd v255 [`src/systemd/sd-journal.h`](https://github.com/systemd/systemd/blob/v255/src/systemd/sd-journal.h)
- systemd v255 [`sd_journal_open`](https://github.com/systemd/systemd/blob/v255/man/sd_journal_open.xml),
  [`sd_journal_seek_head`](https://github.com/systemd/systemd/blob/v255/man/sd_journal_seek_head.xml),
  [`sd_journal_next`](https://github.com/systemd/systemd/blob/v255/man/sd_journal_next.xml),
  [`sd_journal_get_cursor`](https://github.com/systemd/systemd/blob/v255/man/sd_journal_get_cursor.xml),
  [`sd_journal_get_realtime_usec`](https://github.com/systemd/systemd/blob/v255/man/sd_journal_get_realtime_usec.xml),
  and [`sd_journal_get_data`](https://github.com/systemd/systemd/blob/v255/man/sd_journal_get_data.xml) manuals
- purego v0.10.0 [`dlfcn.go`](https://github.com/ebitengine/purego/blob/v0.10.0/dlfcn.go) and
  [`func.go`](https://github.com/ebitengine/purego/blob/v0.10.0/func.go)
- systemd [`Journal Export Format`](https://systemd.io/JOURNAL_EXPORT_FORMATS/) and v255
  [`systemd-journal-remote` file input/output interface](https://github.com/systemd/systemd/blob/v255/src/journal-remote/journal-remote-main.c)
- Ubuntu 24.04 [`systemd-journal-remote` package](https://packages.ubuntu.com/noble/systemd-journal-remote)
