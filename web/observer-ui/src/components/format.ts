export function formatBytes(value: number | null): string {
  if (value === null) return "Unavailable";
  if (value < 1_024) return `${Number(value.toFixed(1))} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let amount = value;
  let unit = "B";
  for (const candidate of units) {
    amount /= 1_024;
    unit = candidate;
    if (amount < 1_024) break;
  }
  return `${amount >= 10 ? amount.toFixed(0) : amount.toFixed(1)} ${unit}`;
}

export function formatValue(value: number | null, unit: string): string {
  if (value === null) return "Unavailable";
  if (unit === "bytes") return formatBytes(value);
  if (unit === "bytes_per_second") return `${formatBytes(value)}/s`;
  if (unit === "percent" || unit === "%") return `${value.toFixed(1)}%`;
  if (unit === "milliseconds") return `${value.toLocaleString()} ms`;
  return `${value.toLocaleString()}${unit === "count" ? "" : ` ${unit}`}`;
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) return `${Math.max(0, Math.round(seconds))}s`;
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h`;
  return `${Math.max(1, Math.floor(seconds / 60))}m`;
}

export function formatRelative(iso: string): string {
  return new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" }).format(new Date(iso));
}

export function titleCase(value: string): string {
  return value.toLowerCase().replaceAll("_", " ").replaceAll("-", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

/** A code-owned presentation label; it never reads a log body. */
export function formatEventCode(value: string): string { return titleCase(value); }
