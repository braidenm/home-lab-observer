import { titleCase } from "./format";

export function StateBadge({ state, label }: { state: string; label?: string }) {
  const token = state.toLowerCase().replaceAll("_", "-");
  return (
    <span className={`observer-state observer-state--${token}`}>
      <span className="observer-state__dot" aria-hidden="true" />
      {label ?? titleCase(state)}
    </span>
  );
}
