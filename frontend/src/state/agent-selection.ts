import { useSyncExternalStore } from 'react';

let selectedId: string | null = null;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) {
    l();
  }
}

export function selectAgent(id: string) {
  selectedId = id;
  emit();
}

export function clearSelection() {
  selectedId = null;
  emit();
}

export function useSelectedAgentId(): string | null {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => selectedId,
  );
}
