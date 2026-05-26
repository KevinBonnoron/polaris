import { createCollection } from '@tanstack/db';
import { DeleteProject, ListProjects, UpsertProject } from '@/wailsjs/go/main/App';
import { wailsCollectionOptions } from '../lib/wails-db-collection';
import type { Project } from '../types';

export const projectsCollection = createCollection(
  wailsCollectionOptions<Project>({
    name: 'projects',
    list: () => ListProjects() as unknown as Promise<Project[]>,
    upsert: (p) => UpsertProject(p as never) as unknown as Promise<Project>,
    remove: (id) => DeleteProject(id),
  }),
);
