import { createCollection } from '@tanstack/db';
import { DeleteCustomProvider, ListCustomProviders, UpsertCustomProvider } from '@/wailsjs/go/main/App';
import { wailsCollectionOptions } from '../lib/wails-db-collection';
import type { CustomProvider } from '../types';

export const customProvidersCollection = createCollection(
  wailsCollectionOptions<CustomProvider>({
    name: 'customProviders',
    list: () => ListCustomProviders() as unknown as Promise<CustomProvider[]>,
    upsert: (p) => UpsertCustomProvider(p as never) as unknown as Promise<CustomProvider>,
    remove: (id) => DeleteCustomProvider(id),
  }),
);
