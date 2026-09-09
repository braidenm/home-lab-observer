import { useCallback, useEffect, useRef, useState } from "react";
import { ObserverDashboard } from "../src";
import { LocalHttpObserverDataSource } from "../src/local";
import type { ObserverDataSource } from "../src/types";

export const SESSION_TOKEN_KEY = "home-lab-observer.access-token.v1";

type UnlockState =
  | { phase: "locked"; error: string | null }
  | { phase: "validating"; saved: boolean }
  | { phase: "unlocked"; dataSource: ObserverDataSource };

export interface EmbeddedObserverAppProps {
  createDataSource?: (token: string) => ObserverDataSource;
  tokenStorage?: Pick<Storage, "getItem" | "setItem" | "removeItem">;
}

const createLocalDataSource = (token: string): ObserverDataSource =>
  new LocalHttpObserverDataSource({ baseUrl: "", bearerToken: token });

export function EmbeddedObserverApp({
  createDataSource = createLocalDataSource,
  tokenStorage = window.sessionStorage
}: EmbeddedObserverAppProps) {
  const initialToken = useRef<string | null | undefined>(undefined);
  if (initialToken.current === undefined) initialToken.current = readToken(tokenStorage);
  const [state, setState] = useState<UnlockState>(() => initialToken.current
    ? { phase: "validating", saved: true }
    : { phase: "locked", error: null });
  const inputRef = useRef<HTMLInputElement>(null);
  const attempt = useRef(0);

  const validate = useCallback(async (token: string, saved: boolean) => {
    const currentAttempt = ++attempt.current;
    setState({ phase: "validating", saved });
    const dataSource = createDataSource(token);
    try {
      await dataSource.getCapabilities();
      if (attempt.current !== currentAttempt) return;
      writeToken(tokenStorage, token);
      initialToken.current = null;
      setState({ phase: "unlocked", dataSource });
    } catch {
      if (attempt.current !== currentAttempt) return;
      forgetToken(tokenStorage);
      initialToken.current = null;
      setState({
        phase: "locked",
        error: "The token could not be validated. Confirm the observer is running and paste the current local token."
      });
    }
  }, [createDataSource, tokenStorage]);

  useEffect(() => {
    if (initialToken.current) void validate(initialToken.current, true);
  }, [validate]);

  const unlock = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const token = inputRef.current?.value ?? "";
    if (!token) return;
    if (inputRef.current) inputRef.current.value = "";
    void validate(token, false);
  };

  const lock = () => {
    attempt.current += 1;
    forgetToken(tokenStorage);
    initialToken.current = null;
    setState({ phase: "locked", error: null });
  };

  if (state.phase === "unlocked") {
    return (
      <div className="observer-shell observer-embedded-dashboard">
        <div className="observer-embedded-session-bar">
          <span>Local session unlocked</span>
          <button type="button" aria-label="Lock dashboard" onClick={lock}>Lock &amp; forget</button>
        </div>
        <ObserverDashboard dataSource={state.dataSource} mode="local" />
      </div>
    );
  }

  if (state.phase === "validating") {
    return (
      <main className="observer-shell observer-unlock-shell">
        <section className="observer-unlock-card" role="status" aria-live="polite">
          <div className="observer-brand__mark" aria-hidden="true"><span /><span /><span /></div>
          <p className="observer-kicker">Private local access</p>
          <h1>Validating local access</h1>
          <p>{state.saved ? "Checking the token saved for this browser tab." : "Checking the token with the local observer."}</p>
          <div className="observer-unlock-progress" aria-hidden="true" />
        </section>
      </main>
    );
  }

  return (
    <main className="observer-shell observer-unlock-shell">
      <section className="observer-unlock-card" aria-labelledby="observer-unlock-title">
        <div className="observer-brand__mark" aria-hidden="true"><span /><span /><span /></div>
        <p className="observer-kicker">Private local access</p>
        <h1 id="observer-unlock-title">Unlock Home Lab Observer</h1>
        <p>Paste the token from the observer's local state file. It is checked on this machine and remembered only for this browser tab.</p>
        {state.error && <p className="observer-unlock-error" role="alert">{state.error}</p>}
        <form onSubmit={unlock}>
          <label htmlFor="observer-local-token">Local access token</label>
          <input
            ref={inputRef}
            id="observer-local-token"
            name="observer-local-token"
            type="password"
            required
            autoFocus
            autoComplete="off"
            autoCapitalize="none"
            spellCheck={false}
            maxLength={4096}
            aria-describedby="observer-token-help"
          />
          <span id="observer-token-help">The token is never added to the page URL, cookies, or persistent browser storage.</span>
          <button type="submit">Unlock dashboard</button>
        </form>
      </section>
    </main>
  );
}

function readToken(storage: Pick<Storage, "getItem" | "removeItem">): string | null {
  try {
    const token = storage.getItem(SESSION_TOKEN_KEY);
    if (token) return token;
    if (token === "") storage.removeItem(SESSION_TOKEN_KEY);
    return null;
  } catch {
    return null;
  }
}

function writeToken(storage: Pick<Storage, "setItem">, token: string): void {
  try {
    storage.setItem(SESSION_TOKEN_KEY, token);
  } catch {
    // The validated data source remains usable in memory when session storage is unavailable.
  }
}

function forgetToken(storage: Pick<Storage, "removeItem">): void {
  try {
    storage.removeItem(SESSION_TOKEN_KEY);
  } catch {
    // There is no broader storage fallback: the token remains session-only.
  }
}
