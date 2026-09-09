export function managerSmokeFailure(primaryFailure, cleanupConfirmed) {
  if (primaryFailure !== undefined && primaryFailure !== null) {
    const cleanup = cleanupConfirmed ? "" : "; cleanup=BACKGROUND_CLEANUP_UNCONFIRMED";
    const message = primaryFailure instanceof Error ? primaryFailure.message : "Windows manager smoke failed";
    return new Error(message + cleanup);
  }
  if (!cleanupConfirmed) {
    return new Error("Windows manager cleanup could not be verified; cleanup=BACKGROUND_CLEANUP_UNCONFIRMED");
  }
  return null;
}
