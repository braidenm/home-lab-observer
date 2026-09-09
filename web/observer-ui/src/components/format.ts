export function formatBytes(value: number | null): string {
  if (value === null) return "Unavailable";
  if (value < 1_024) return `${value} B`;
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
  if (unit === "percent" || unit === "%") return `${value.toFixed(1)}%`;
  if (unit === "milliseconds") return `${value.toLocaleString()} ms`;
  return `${value.toLocaleString()}${unit === "count" ? "" : ` ${unit}`}`;
}

export function formatDuration(seconds: number): string {
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  return days > 0 ? `${days}d ${hours}h` : `${hours}h`;
}

export function formatRelative(iso: string): string {
  return new Intl.DateTimeFormat(undefined, { hour: "numeric", minute: "2-digit" }).format(new Date(iso));
}

export function titleCase(value: string): string {
  return value.toLowerCase().replaceAll("_", " ").replaceAll("-", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

/** A code-owned presentation label; it never reads a log body. */
export function formatEventCode(value: string): string { return titleCase(value); }
