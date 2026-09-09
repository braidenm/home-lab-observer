import { describe, expect, it } from "vitest";
import { SyntheticObserverDataSource } from "./synthetic";

describe("SyntheticObserverDataSource", () => {
  it("returns deterministic contract-shaped snapshots without process identity fields", async () => {
    const source = new SyntheticObserverDataSource();
    const first = await source.getCurrentSnapshot();
    const second = await source.getCurrentSnapshot();
    expect(first).toEqual(second);
    expect(first.sections.processes.items.every((item) => !("account" in item) && !("owner" in item) && !("user" in item) && !("arguments" in item))).toBe(true);
    expect(first.sections.processes).toMatchObject({ totalCount: 231, returnedCount: 4, truncated: true });
  });

  it("keeps log bodies omitted and exposes trends only as an optional demo source", async () => {
    const source = new SyntheticObserverDataSource();
    const snapshot = await source.getCurrentSnapshot();
    expect(snapshot.privacy.profile).toBe("SAFE_DEFAULT");
    expect(snapshot.sections.logs.items.every((event) => event.body.state === "OMITTED")).toBe(true);
    expect((await source.getTrends("24h")).range).toBe("24h");
  });
});
