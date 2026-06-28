import type { CollectionConfig, DeleteMutationFnParams, InsertMutationFnParams, SyncConfig, UpdateMutationFnParams } from '@tanstack/db';
import { EventsOn } from '@/wailsjs/runtime/runtime';

// Window over which rapid change events for one collection collapse into a single
// refetch. Long enough to absorb a streaming agent's PatchAgent bursts, short
// enough to stay imperceptible for one-off updates like a rename.
const refreshDebounceMs = 80;

export interface WailsCollectionConfig<TItem extends { id: string }> {
  /** Logical name — matches the `collection:<name>:changed` event the Go store emits. */
  name: string;
  /** Fetch the full collection from Go. */
  list: () => Promise<TItem[]>;
  /** Upsert a row. Must return the persisted record (with server-assigned id if it was empty). */
  upsert: (item: TItem) => Promise<TItem>;
  /** Delete a row by id. */
  remove: (id: string) => Promise<void>;
}

export function wailsCollectionOptions<TItem extends { id: string }>(config: WailsCollectionConfig<TItem>): CollectionConfig<TItem, string> {
  const eventName = `collection:${config.name}:changed`;

  const sync: SyncConfig<TItem, string> = {
    sync: ({ begin, write, commit, markReady }) => {
      const current = new Map<string, TItem>();

      // Serialise refreshes. Change events can arrive in bursts (e.g. live token
      // updates while an agent streams); running list() calls concurrently lets a
      // refresh started before a write land its now-stale result after a later
      // one, resurrecting fields the backend already cleared (e.g. a pending
      // question). The in-flight guard coalesces overlapping events and always
      // runs one final pass so the collection settles on the latest state.
      let refreshing = false;
      let dirty = false;

      const runRefresh = async () => {
        let next: TItem[];
        try {
          next = await config.list();
        } catch (err) {
          console.error(`wailsCollection[${config.name}] refresh failed`, err);
          return;
        }
        begin();
        const seen = new Set<string>();
        for (const item of next) {
          seen.add(item.id);
          const prev = current.get(item.id);
          if (!prev) {
            write({ type: 'insert', value: item });
            current.set(item.id, item);
          } else if (!shallowEqual(prev, item)) {
            write({ type: 'update', value: item });
            current.set(item.id, item);
          }
        }
        for (const [id, item] of current) {
          if (!seen.has(id)) {
            write({ type: 'delete', value: item });
            current.delete(id);
          }
        }
        commit();
      };

      const refresh = async () => {
        if (refreshing) {
          dirty = true;
          return;
        }
        refreshing = true;
        try {
          do {
            dirty = false;
            await runRefresh();
          } while (dirty);
        } finally {
          refreshing = false;
        }
      };

      // Coalesce bursts of change events into one refetch. A working agent emits
      // many PatchAgent-driven changes per turn; without this each one re-runs the
      // full list() + per-item diff + whole-list re-render, which is what makes the
      // UI lag while agents stream. The in-flight guard above still settles on the
      // latest state after the window.
      let refreshTimer: ReturnType<typeof setTimeout> | null = null;
      const scheduleRefresh = () => {
        if (refreshTimer != null) {
          return;
        }
        refreshTimer = setTimeout(() => {
          refreshTimer = null;
          void refresh();
        }, refreshDebounceMs);
      };

      const off = EventsOn(eventName, scheduleRefresh);

      void refresh().finally(() => markReady());

      return () => {
        if (refreshTimer != null) {
          clearTimeout(refreshTimer);
        }
        off();
      };
    },
  };

  return {
    getKey: (item: TItem) => item.id,
    sync,
    onInsert: async (params: InsertMutationFnParams<TItem, string>) => {
      const ids: string[] = [];
      for (const mutation of params.transaction.mutations) {
        const persisted = await config.upsert(mutation.changes as TItem);
        ids.push(persisted.id);
      }
      return ids;
    },
    onUpdate: async (params: UpdateMutationFnParams<TItem, string>) => {
      const keys: string[] = [];
      for (const mutation of params.transaction.mutations) {
        const merged = { ...mutation.original, ...mutation.changes, id: mutation.key } as TItem;
        await config.upsert(merged);
        keys.push(mutation.key);
      }
      return keys;
    },
    onDelete: async (params: DeleteMutationFnParams<TItem, string>) => {
      const keys: string[] = [];
      for (const mutation of params.transaction.mutations) {
        await config.remove(mutation.key);
        keys.push(mutation.key);
      }
      return keys;
    },
  };
}

function shallowEqual(a: unknown, b: unknown): boolean {
  if (a === b) {
    return true;
  }

  if (typeof a !== 'object' || typeof b !== 'object' || a === null || b === null) {
    return false;
  }

  // Cheap structural comparison for our flat record shapes (no nested arrays we care about beyond integrations).
  return JSON.stringify(a) === JSON.stringify(b);
}
