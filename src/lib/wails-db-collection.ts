import type { CollectionConfig, DeleteMutationFnParams, InsertMutationFnParams, SyncConfig, UpdateMutationFnParams } from '@tanstack/db';
import { EventsOff, EventsOn } from '@/wailsjs/runtime/runtime';

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

      const refresh = async () => {
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

      EventsOn(eventName, () => {
        void refresh();
      });

      void refresh().finally(() => markReady());

      return () => {
        EventsOff(eventName);
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
