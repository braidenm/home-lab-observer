import { lstat, open } from "node:fs/promises";
import path from "node:path";

const limit = 64 * 1024;
const unavailable = () => ({ code: "RUNTIME_DIAGNOSTIC_UNAVAILABLE" });
const events = new Set(["RUNTIME_STARTED", "RUNTIME_READY", "STOP_REQUESTED", "RUNTIME_STOPPED", "COLLECTION_CYCLE", "HISTORY_MAINTENANCE", "HTTP_SERVER"]);
const codes = new Set(["OK", "PARTIAL", "FAILED", "TIMEOUT", "UNAVAILABLE", "PERMISSION_DENIED", "DISABLED", "CANCELLED"]);
const fields = ["code", "count", "duration_ms", "event", "observed_at", "version"];

export function summarizeRuntimeDiagnostics(bytes) {
  try {
    if (!Buffer.isBuffer(bytes) || bytes.length === 0 || bytes.length > limit) return unavailable();
    const text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    if (!text.endsWith("\n")) return unavailable();
    const rows = text.slice(0, -1).split("\n");
    if (rows.length > 256) return unavailable();
    const result = rows.map((line) => {
      if (Buffer.byteLength(line) > 8192) throw new Error();
      const row = JSON.parse(line);
      if (!row || typeof row !== "object" || Array.isArray(row) || JSON.stringify(Object.keys(row).sort()) !== JSON.stringify(fields) ||
          !events.has(row.event) || !codes.has(row.code) || typeof row.observed_at !== "string" || typeof row.version !== "string" ||
          row.observed_at.length > 40 || row.version.length > 64 || !Number.isSafeInteger(row.count) || row.count < 0 ||
          !Number.isSafeInteger(row.duration_ms) || row.duration_ms < 0 || row.duration_ms > 3_600_000) throw new Error();
      return { event: row.event, code: row.code };
    });
    return { code: "RUNTIME_DIAGNOSTIC_AVAILABLE", events: result.slice(-8) };
  } catch {
    return unavailable();
  }
}

// Called only with the smoke's freshly created private temporary state root.
export async function readRuntimeSmokeDiagnostics(state) {
  let handle;
  try {
    const directory = path.join(state, "diagnostics");
    const parent = await lstat(directory);
    if (!parent.isDirectory() || parent.isSymbolicLink()) return unavailable();
    const target = path.join(directory, "observer.jsonl");
    const before = await lstat(target);
    if (!before.isFile() || before.isSymbolicLink() || before.nlink !== 1 || before.size > limit) return unavailable();
    handle = await open(target, "r");
    const actual = await handle.stat();
    if (!actual.isFile() || actual.nlink !== 1 || actual.dev !== before.dev || actual.ino !== before.ino || actual.size > limit) return unavailable();
    const bytes = Buffer.alloc(limit + 1);
    let length = 0;
    while (length <= limit) {
      const { bytesRead } = await handle.read(bytes, length, bytes.length - length, length);
      if (bytesRead === 0) break;
      length += bytesRead;
    }
    if (length > limit) return unavailable();
    return summarizeRuntimeDiagnostics(bytes.subarray(0, length));
  } catch {
    return unavailable();
  } finally {
    if (handle) await handle.close().catch(() => {});
  }
}
