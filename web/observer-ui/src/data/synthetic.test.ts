import { describe, expect, it } from "vitest";
import { SyntheticObserverDataSource } from "./synthetic";

describe("SyntheticObserverDataSource", () => {
  it("filters deterministic workload fixtures without mutating the source", async () => {
    const source = new SyntheticObserverDataSource();
    const filtered = await source.getWorkloads({ kind: "container", search: "media" });
    const all = await source.getWorkloads();

    expect(filtered.items).toHaveLength(1);
    expect(filtered.items[0]).toMatchObject({ kind: "container", name: "media-worker" });
    expect(all.items.length).toBeGreaterThan(filtered.items.length);
  });

  it("keeps log message bodies disabled and absent", async () => {
    const logs = await new SyntheticObserverDataSource().getLogs();
    expect(logs.bodyStorageDefault).toBe("off");
    expect(logs.sources.every((source) => source.bodyStorage === "off")).toBe(true);
    expect(logs.events.every((event) => event.body === undefined)).toBe(true);
  });
});
