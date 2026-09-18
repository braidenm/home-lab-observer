// Individual health probes may time out while a packaged observer starts on a
// loaded CI host. The outer poll deadline remains authoritative.
export function retryableSmokePollError(error) {
  return error?.code === "ECONNREFUSED" ||
    error?.name === "TypeError" ||
    error?.name === "TimeoutError" ||
    error?.name === "AbortError";
}
