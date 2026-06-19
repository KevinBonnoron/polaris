import { createCollection } from '@tanstack/db';
import { DeleteAutomation, ListAutomations, UpsertAutomation } from '@/wailsjs/go/main/App';
import { wailsCollectionOptions } from '../lib/wails-db-collection';
import type { Automation } from '../types';

export const automationsCollection = createCollection(
  wailsCollectionOptions<Automation>({
    name: 'automations',
    list: () => ListAutomations() as unknown as Promise<Automation[]>,
    upsert: (a) => UpsertAutomation(a as never) as unknown as Promise<Automation>,
    remove: (id) => DeleteAutomation(id),
  }),
);
