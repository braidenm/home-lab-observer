import type { Availability, HealthState, Severity } from "../types";
import { titleCase } from "./format";

type BadgeState = Availability | HealthState | Severity | "running" | "active" | "exited" | "inactive" | "denied";

export function StateBadge({ state, label }: { state: BadgeState; label?: string }) {
  return (
    <span className={`observer-state observer-state--${state}`}>
      <span className="observer-state__dot" aria-hidden="true" />
      {label ?? titleCase(state)}
    </span>
  );
}
