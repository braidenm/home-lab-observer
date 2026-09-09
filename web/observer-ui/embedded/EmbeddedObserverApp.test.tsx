import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SyntheticObserverDataSource, syntheticCapabilities } from "../src/data/synthetic";
import type { ObserverCapabilities, ObserverDataSource } from "../src/types";
import { ObserverTransportError } from "../src/local";
import { EmbeddedObserverApp, SESSION_TOKEN_KEY } from "./EmbeddedObserverApp";

afterEach(() => {
  cleanup();
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("EmbeddedObserverApp", () => {
  it("fails safely when data-source construction rejects a token", async () => {
    const user = userEvent.setup();
    render(<EmbeddedObserverApp createDataSource={() => { throw new Error("synthetic-private-detail"); }} />);
    await user.type(screen.getByLabelText("Local access token"), "synthetic-token");
    await user.click(screen.getByRole("button", { name: "Unlock dashboard" }));
    expect((await screen.findByRole("alert")).textContent).not.toContain("synthetic-private-detail");
    expect(window.sessionStorage.getItem(SESSION_TOKEN_KEY)).toBeNull();
  });

  it("stores a submitted token only after capabilities validation and forgets it on lock", async () => {
    const user = userEvent.setup();
    let acceptCapabilities: (value: ObserverCapabilities) => void = () => undefined;
    const capabilityCheck = vi.fn(() => new Promise<ObserverCapabilities>((resolve) => { acceptCapabilities = resolve; }));
    const createDataSource = vi.fn(() => dataSourceWith(capabilityCheck));
    const startingUrl = window.location.href;
    const startingCookie = document.cookie;
    const log = vi.spyOn(console, "log").mockImplementation(() => undefined);
    const warn = vi.spyOn(console, "warn").mockImplementation(() => undefined);
    const error = vi.spyOn(console, "error").mockImplementation(() => undefined);

    render(<EmbeddedObserverApp createDataSource={createDataSource} />);
    const field = await screen.findByLabelText("Local access token");
    expect(field.getAttribute("type")).toBe("password");
    await user.type(field, "synthetic-session-secret");
    await user.click(screen.getByRole("button", { name: "Unlock dashboard" }));

    expect(screen.getByRole("status").textContent).toContain("Checking the token with the local observer");
    expect(window.sessionStorage.getItem(SESSION_TOKEN_KEY)).toBeNull();
    expect(document.body.textContent).not.toContain("synthetic-session-secret");

    acceptCapabilities(structuredClone(syntheticCapabilities));
    expect(await screen.findByRole("heading", { name: "studio-node" })).toBeTruthy();
    expect(window.sessionStorage.getItem(SESSION_TOKEN_KEY)).toBe("synthetic-session-secret");
    expect(window.localStorage.getItem(SESSION_TOKEN_KEY)).toBeNull();
    expect(window.location.href).toBe(startingUrl);
    expect(document.cookie).toBe(startingCookie);
    expect(document.body.textContent).not.toContain("synthetic-session-secret");
    expect(document.documentElement.innerHTML).not.toContain("synthetic-session-secret");
    expect(log).not.toHaveBeenCalled();
    expect(warn).not.toHaveBeenCalled();
    expect(error).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Lock dashboard" }));
    expect(window.sessionStorage.getItem(SESSION_TOKEN_KEY)).toBeNull();
    expect(await screen.findByLabelText("Local access token")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "studio-node" })).toBeNull();
  });

  it("clears an invalid saved token after a 401 without exposing it", async () => {
    window.sessionStorage.setItem(SESSION_TOKEN_KEY, "expired-session-secret");
    const capabilityCheck = vi.fn<ObserverDataSource["getCapabilities"]>().mockRejectedValue(new ObserverTransportError("Unauthorized", 401));

    render(<EmbeddedObserverApp createDataSource={() => dataSourceWith(capabilityCheck)} />);

    expect(screen.getByRole("status").textContent).toContain("Checking the token saved for this browser tab");
    expect((await screen.findByRole("alert")).textContent).toContain("could not be validated");
    expect(window.sessionStorage.getItem(SESSION_TOKEN_KEY)).toBeNull();
    expect(screen.getByLabelText("Local access token")).toBeTruthy();
    expect(document.body.textContent).not.toContain("expired-session-secret");
    expect(document.documentElement.innerHTML).not.toContain("expired-session-secret");
    expect(capabilityCheck).toHaveBeenCalledTimes(1);
  });
});

function dataSourceWith(getCapabilities: ObserverDataSource["getCapabilities"]): ObserverDataSource {
  const synthetic = new SyntheticObserverDataSource();
  return {
    getCapabilities,
    getCurrentSnapshot: () => synthetic.getCurrentSnapshot(),
    getTrends: (range) => synthetic.getTrends(range)
  };
}
