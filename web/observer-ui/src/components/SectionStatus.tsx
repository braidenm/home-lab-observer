import type { ListSection, SectionQuality } from "../types";
import { StateBadge } from "./StateBadge";
import { formatRelative, titleCase } from "./format";

export function SectionStatus({ section, compact = false }: { section: SectionQuality | ListSection<unknown>; compact?: boolean }) {
  const list = "totalCount" in section ? section : null;
  return (
    <dl className={`observer-section-status${compact ? " observer-section-status--compact" : ""}`}>
      <div><dt>Support</dt><dd><StateBadge state={section.supportState} /></dd></div>
      <div><dt>Collection</dt><dd><StateBadge state={section.collectionState} /></dd></div>
      <div><dt>Freshness</dt><dd><StateBadge state={section.freshness} /></dd></div>
      <div><dt>Observed</dt><dd>{section.observedAt ? formatRelative(section.observedAt) : "Not observed"}</dd></div>
      {list && <div><dt>Records</dt><dd>{list.returnedCount} of {list.totalCount}{list.truncated ? " · truncated" : ""}</dd></div>}
      <div><dt>Reason</dt><dd>{section.reasonCode ? titleCase(section.reasonCode) : "None"}</dd></div>
    </dl>
  );
}
