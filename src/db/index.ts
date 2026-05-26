import { createCollection } from '@tanstack/db';
import { DeleteAgent, DeleteAutomation, DeleteCustomProvider, DeleteNotification, ListAgents, ListAutomations, ListCustomProviders, ListNotifications, ListProjects, UpsertAgent, UpsertAutomation, UpsertCustomProvider, UpsertNotification, UpsertProject } from '@/wailsjs/go/main/App';
import { wailsCollectionOptions } from '../lib/wails-db-collection';
import type { Agent, Automation, CustomProvider, Notification } from '../types';

export const agentsCollection = createCollection(
  wailsCollectionOptions<Agent>({
    name: 'agents',
    list: () => ListAgents('') as unknown as Promise<Agent[]>,
    upsert: (a) => UpsertAgent(a as never) as unknown as Promise<Agent>,
    remove: (id) => DeleteAgent(id),
  }),
);

export const notificationsCollection = createCollection(
  wailsCollectionOptions<Notification>({
    name: 'notifications',
    list: () => ListNotifications() as unknown as Promise<Notification[]>,
    upsert: (n) => UpsertNotification(n as never) as unknown as Promise<Notification>,
    remove: (id) => DeleteNotification(id),
  }),
);

export const customProvidersCollection = createCollection(
  wailsCollectionOptions<CustomProvider>({
    name: 'customProviders',
    list: () => ListCustomProviders() as unknown as Promise<CustomProvider[]>,
    upsert: (p) => UpsertCustomProvider(p as never) as unknown as Promise<CustomProvider>,
    remove: (id) => DeleteCustomProvider(id),
  }),
);

export const automationsCollection = createCollection(
  wailsCollectionOptions<Automation>({
    name: 'automations',
    list: () => ListAutomations() as unknown as Promise<Automation[]>,
    upsert: (a) => UpsertAutomation(a as never) as unknown as Promise<Automation>,
    remove: (id) => DeleteAutomation(id),
  }),
);
