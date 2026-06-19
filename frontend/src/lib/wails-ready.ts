// Under Wails v3 the bindings are plain ES module imports resolved at load time,
// so there is no `window.go.main.App` bridge to wait for. Calls go through the
// @wailsio/runtime transport, which handles its own readiness. This resolves
// immediately, keeping the original API so callers stay unchanged.
let cached: Promise<void> | null = null;

export function wailsReady(): Promise<void> {
  if (!cached) {
    cached = Promise.resolve();
  }
  return cached;
}
