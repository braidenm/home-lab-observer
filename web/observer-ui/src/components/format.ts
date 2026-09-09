import type { Measurement } from "../types";

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

export function formatMeasurement(measurement: Measurement): string {
  if (measurement.value === null) return titleCase(measurement.availability);
  if (measurement.unit === "bytes") return formatBytes(measurement.value);
  if (measurement.unit === "celsius") return `${measurement.value.toFixed(0)} °C`;
  if (measurement.unit === "bytes-per-second") return `${formatBytes(measurement.value)}/s`;
  if (measurement.unit === "%") return `${measurement.value.toFixed(1)}%`;
  return measurement.value.toLocaleString();
}

export function formatDuration(seconds: number): string {
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  return days > 0 ? `${days}d ${hours}h` : `${hours}h`;
}

export function formatRelative(iso: string): string {
  const date = new Date(iso);
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit"
  }).format(date);
}

export function titleCase(value: string): string {
  return value.replaceAll("-", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}
