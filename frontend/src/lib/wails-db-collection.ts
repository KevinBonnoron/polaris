import type { CollectionConfig, DeleteMutationFnParams, InsertMutationFnParams, SyncConfig, UpdateMutationFnParams } from '@tanstack/db';
import { EventsOn } from '@/wailsjs/runtime/runtime';

// Window over which rapid change events for one collection collapse into a single
// refetch. Long enough to absorb a streaming agent's PatchAgent bursts, short
// enough to stay imperceptible for one-off updates like a rename.
const refreshDebounceMs = 80;

export interface WailsCollectionConfig<TItem extends { id: string }> {
  /** Logical name used as the collection key. */
  name: string;
  /**
   * Event suffix to listen to (defaults to `name`). Use when the Go store emits
   * a shared event (e.g. `automationRuns`) but each collection instance covers a
   * filtered subset identified by `name` (e.g. `automationRuns-<id>`).
   */
  event?: string;
  /** Fetch the full collection from Go. */
  list: () => Promise<TItem[]>;
  /** Upsert a row. Required for mutable collections. */
  upsert?: (item: TItem) => Promise<TItem>;
  /** Delete a row by id. Required for mutable collections. */
  remove?: (id: string) => Promise<void>;
}

export function wailsCollectionOptions<TItem extends { id: string }>(config: WailsCollectionConfig<TItem>): CollectionConfig<TItem, string> {
  const eventName = `collection:${config.event ?? config.name}:changed`;

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

  const base: CollectionConfig<TItem, string> = { getKey: (item: TItem) => item.id, sync };

  if (config.upsert) {
    const upsert = config.upsert;
    base.onInsert = async (params: InsertMutationFnParams<TItem, string>) => {
      const ids: string[] = [];
      for (const mutation of params.transaction.mutations) {
        const persisted = await upsert(mutation.changes as TItem);
        ids.push(persisted.id);
      }
      return ids;
    };
    base.onUpdate = async (params: UpdateMutationFnParams<TItem, string>) => {
      const keys: string[] = [];
      for (const mutation of params.transaction.mutations) {
        const merged = { ...mutation.original, ...mutation.changes, id: mutation.key } as TItem;
        await upsert(merged);
        keys.push(mutation.key);
      }
      return keys;
    };
  }

  if (config.remove) {
    const remove = config.remove;
    base.onDelete = async (params: DeleteMutationFnParams<TItem, string>) => {
      const keys: string[] = [];
      for (const mutation of params.transaction.mutations) {
        await remove(mutation.key);
        keys.push(mutation.key);
      }
      return keys;
    };
  }

  return base;
}

// wailsAppendCollectionOptions builds a CollectionConfig for append-only data
// (e.g. agent log files) that avoids re-reading the entire source on every change
// event. Each change calls listFrom(offset) to fetch only the new tail; a reset
// event (e.g. log cleared or retracted) clears all items and re-reads from 0.
//
// TItem must NOT define a `_seq` field — the helper adds it as the stable sort key
// so that TanStack DB's SortedMap returns events in insertion order. The key is a
// zero-padded decimal so lexicographic ordering matches insertion order.
export function wailsAppendCollectionOptions<TItem>(name: string, listFrom: (offset: number) => Promise<{ items: TItem[]; nextOffset: number }>): CollectionConfig<TItem & { _seq: number }, string> {
  const eventName = `collection:${name}:changed`;
  type Item = TItem & { _seq: number };

  const sync: SyncConfig<Item, string> = {
    sync: ({ begin, write, commit, markReady }) => {
      let offset = 0;
      let nextSeq = 0;
      const current = new Map<string, Item>();
      let refreshing = false;
      let dirty = false;
      let pendingReset = false;

      const runRefresh = async (reset: boolean) => {
        if (reset) {
          if (current.size > 0) {
            begin();
            for (const item of current.values()) {
              write({ type: 'delete', value: item });
            }
            commit();
            current.clear();
          }
          offset = 0;
          nextSeq = 0;
        }

        let result: { items: TItem[]; nextOffset: number };
        try {
          result = await listFrom(offset);
        } catch (err) {
          console.error(`wailsAppendCollection[${name}] refresh failed`, err);
          return;
        }

        if (result.items.length > 0) {
          begin();
          for (const raw of result.items) {
            const seq = nextSeq++;
            const item = { ...raw, _seq: seq } as Item;
            write({ type: 'insert', value: item });
            current.set(String(seq), item);
          }
          commit();
        }
        offset = result.nextOffset;
      };

      const refresh = async (reset = false) => {
        if (refreshing) {
          dirty = true;
          pendingReset = pendingReset || reset;
          return;
        }
        refreshing = true;
        let nextReset = reset;
        try {
          do {
            dirty = false;
            const doReset = nextReset || pendingReset;
            nextReset = false;
            pendingReset = false;
            await runRefresh(doReset);
          } while (dirty);
        } finally {
          refreshing = false;
        }
      };

      let refreshTimer: ReturnType<typeof setTimeout> | null = null;
      const scheduleRefresh = (reset = false) => {
        if (reset) {
          if (refreshTimer != null) {
            clearTimeout(refreshTimer);
            refreshTimer = null;
          }
          void refresh(true);
          return;
        }
        if (refreshTimer != null) {
          return;
        }
        refreshTimer = setTimeout(() => {
          refreshTimer = null;
          void refresh();
        }, refreshDebounceMs);
      };

      const off = EventsOn(eventName, (payload?: { reset?: boolean }) => {
        scheduleRefresh(payload?.reset === true);
      });

      void refresh().finally(() => markReady());

      return () => {
        if (refreshTimer != null) clearTimeout(refreshTimer);
        off();
      };
    },
  };

  return {
    // Zero-pad so lexicographic sort in SortedMap matches insertion order.
    getKey: (item: Item) => String(item._seq).padStart(15, '0'),
    sync,
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
