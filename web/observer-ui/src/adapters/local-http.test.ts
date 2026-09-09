import { describe, expect, it, vi } from "vitest";
import { LocalHttpObserverDataSource } from "./local-http";

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

describe("LocalHttpObserverDataSource", () => {
  it("allows loopback endpoints and sends a token only in the authorization header", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({ schemaVersion: "observer-logs/v1" }));
    const source = new LocalHttpObserverDataSource({
      baseUrl: "http://127.0.0.1:9780",
      bearerToken: "local-test-token",
      fetcher
    });

    await source.getLogs({ source: "observer", severities: ["error", "warning"], limit: 20 });

    const [url, init] = fetcher.mock.calls[0];
    expect(url).toBe("http://127.0.0.1:9780/api/v1/logs?source=observer&severities=error%2Cwarning&limit=20");
    expect(String(url)).not.toContain("local-test-token");
    expect(new Headers(init?.headers).get("Authorization")).toBe("Bearer local-test-token");
    expect(init).toMatchObject({ credentials: "omit", cache: "no-store", redirect: "error" });
  });

  it("rejects non-loopback and credential-bearing absolute URLs", () => {
    expect(() => new LocalHttpObserverDataSource({ baseUrl: "https://observer.example.test" })).toThrow(/loopback/);
    expect(() => new LocalHttpObserverDataSource({ baseUrl: "http://user:secret@127.0.0.1:9780" })).toThrow(/credentials/);
  });

  it("rejects unsuccessful and non-JSON responses", async () => {
    const failed = new LocalHttpObserverDataSource({ fetcher: vi.fn<typeof fetch>().mockResolvedValue(jsonResponse({}, 503)) });
    await expect(failed.getOverview()).rejects.toMatchObject({ status: 503 });

    const html = new LocalHttpObserverDataSource({
      fetcher: vi.fn<typeof fetch>().mockResolvedValue(new Response("no", { headers: { "content-type": "text/plain" } }))
    });
    await expect(html.getOverview()).rejects.toThrow(/non-JSON/);
  });
});
