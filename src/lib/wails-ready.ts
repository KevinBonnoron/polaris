// Wails injects `window.go.main.App` at startup, but the timing isn't
// synchronous with the JS bundle execution. During a cold start or a full
// page reload (which HMR can trigger), our React tree may render before the
// bridge is wired up — every Wails call then throws
// `undefined is not an object (evaluating 'window["go"]["main"]')`.
//
// wailsReady() returns a promise that resolves as soon as the bridge is
// available, with a 10s timeout so a broken environment fails loud instead
// of hanging.

let cached: Promise<void> | null = null;

declare global {
  interface Window {
    go?: { main?: { App?: unknown } };
  }
}

export function wailsReady(): Promise<void> {
  if (cached) {
    return cached;
  }
  cached = new Promise<void>((resolve, reject) => {
    if (typeof window === 'undefined') {
      resolve();
      return;
    }
    if (window.go?.main?.App) {
      resolve();
      return;
    }
    const started = Date.now();
    const interval = window.setInterval(() => {
      if (window.go?.main?.App) {
        window.clearInterval(interval);
        resolve();
        return;
      }
      if (Date.now() - started > 10_000) {
        window.clearInterval(interval);
        reject(new Error('Wails bridge not available after 10s — is the app running outside of Wails?'));
      }
    }, 30);
  });
  return cached;
}
