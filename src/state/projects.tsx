import { useLiveQuery } from '@tanstack/react-db';
import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { projectsCollection } from '../collections/projects.collection';
import type { Project } from '../types';

const CURRENT_PROJECT_KEY = 'polaris:current-project-id';

type ProjectsState = {
  projects: Project[];
  ready: boolean;
  currentProject: Project | null;
  currentProjectId: string | null;
  setCurrentProjectId: (id: string) => void;
};

const ProjectsContext = createContext<ProjectsState | null>(null);

function readStoredProjectId(): string | null {
  if (typeof window === 'undefined') {
    return null;
  }

  try {
    return window.localStorage.getItem(CURRENT_PROJECT_KEY);
  } catch {
    return null;
  }
}

export function ProjectsProvider({ children }: { children: ReactNode }) {
  const { data: list = [], isReady } = useLiveQuery((q) => q.from({ p: projectsCollection }));

  const [storedId, setStoredId] = useState<string | null>(() => readStoredProjectId());

  const setCurrentProjectId = useCallback((id: string) => {
    setStoredId(id);
    try {
      window.localStorage.setItem(CURRENT_PROJECT_KEY, id);
    } catch {
      // ignore quota errors
    }
  }, []);

  const resolvedId = useMemo(() => {
    if (storedId && list.some((p) => p.id === storedId)) {
      return storedId;
    }
    return list[0]?.id ?? null;
  }, [storedId, list]);

  const currentProject = list.find((p) => p.id === resolvedId) ?? null;

  useEffect(() => {
    if (resolvedId && resolvedId !== storedId) {
      try {
        window.localStorage.setItem(CURRENT_PROJECT_KEY, resolvedId);
      } catch {}
      setStoredId(resolvedId);
    }
  }, [resolvedId, storedId]);

  const value = useMemo<ProjectsState>(
    () => ({
      projects: list,
      ready: isReady,
      currentProject,
      currentProjectId: resolvedId,
      setCurrentProjectId,
    }),
    [list, isReady, currentProject, resolvedId, setCurrentProjectId],
  );

  return <ProjectsContext.Provider value={value}>{children}</ProjectsContext.Provider>;
}

function useProjectsContext(): ProjectsState {
  const ctx = useContext(ProjectsContext);
  if (!ctx) {
    throw new Error('Projects hooks must be used inside <ProjectsProvider>');
  }

  return ctx;
}

export function useProjects(): { projects: Project[]; ready: boolean } {
  const { projects, ready } = useProjectsContext();
  return { projects, ready };
}

export function useCurrentProject(): { project: Project | null; projectId: string | null; setProjectId: (id: string) => void } {
  const { currentProject, currentProjectId, setCurrentProjectId } = useProjectsContext();
  return { project: currentProject, projectId: currentProjectId, setProjectId: setCurrentProjectId };
}
