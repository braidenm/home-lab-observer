import { expect, it } from "vitest";
import { formatDuration, formatValue } from "./format";

it("keeps low byte rates compact and short sample intervals accurate", () => {
  expect(formatValue(564.1901718072071, "bytes_per_second")).toBe("564.2 B/s");
  expect(formatValue(0, "bytes_per_second")).toBe("0 B/s");
  expect(formatValue(null, "bytes_per_second")).toBe("Unavailable");
  expect(formatDuration(15)).toBe("15s");
  expect(formatDuration(60)).toBe("1m");
});
