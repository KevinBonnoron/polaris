// Compatibility shim: maps the Wails v2 runtime API still used across the app to
// the Wails v3 @wailsio/runtime. The backend emits a single payload per event,
// which v3 delivers as `e.data`; v2 callers expect it as the handler's first
// argument, so we unwrap it here. EventsOff is reconstructed from the
// per-listener unsubscribe handles v3 returns.
import { Browser, Clipboard, Events } from '@wailsio/runtime';

// biome-ignore lint/suspicious/noExplicitAny: matches the v2 EventsOn callback ergonomics
type Handler = (...data: any[]) => void;

const offByName = new Map<string, Set<() => void>>();

export function EventsOn(eventName: string, callback: Handler): () => void {
  const off = Events.On(eventName, (e: { data?: unknown }) => callback(e?.data));
  let set = offByName.get(eventName);
  if (!set) {
    set = new Set();
    offByName.set(eventName, set);
  }
  const handle = () => off();
  set.add(handle);
  return () => {
    set?.delete(handle);
    if (set && set.size === 0) {
      offByName.delete(eventName);
    }
    off();
  };
}

export function EventsOff(eventName: string, ...additional: string[]): void {
  for (const name of [eventName, ...additional]) {
    const set = offByName.get(name);
    if (!set) {
      continue;
    }
    for (const off of set) {
      off();
    }
    offByName.delete(name);
  }
}

export function BrowserOpenURL(url: string): void {
  void Browser.OpenURL(url);
}

export function ClipboardGetText(): Promise<string> {
  return Clipboard.Text();
}

export function ClipboardSetText(text: string): Promise<boolean> {
  return Clipboard.SetText(text).then(() => true);
}
