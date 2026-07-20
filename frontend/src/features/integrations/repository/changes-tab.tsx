import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useCurrentProject } from '@/state/projects';
import { type GitChangesOps, GitChangesPanel } from '@/features/git/git-changes-panel';
import { CommitProject, GenerateCommitMessageForProject, GetProjectDiff, GetProjectFileStatuses, GetProjectGitState, OpenInIde, PushProject, StageProjectChanges, StageProjectFile, StageProjectFiles, SyncProject, UnstageProjectChanges, UnstageProjectFile, UnstageProjectFiles } from '@/wailsjs/go/main/App';
import type { ReloadApi } from './use-register-reload';

interface Props {
  projectId: string;
  onRegister?: (api: ReloadApi | null) => void;
}

export function ChangesTab({ projectId, onRegister }: Props) {
  const { project } = useCurrentProject();
  const refreshRef = useRef<(() => Promise<void>) | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    onRegister?.({
      reload: () => {
        setLoading(true);
        refreshRef
          .current?.()
          .catch(() => {})
          .finally(() => setLoading(false));
      },
      loading,
    });
    return () => onRegister?.(null);
  }, [onRegister, loading]);

  const ops: GitChangesOps = useMemo(
    () => ({
      getDiff: () => GetProjectDiff(projectId),
      getFileStatuses: () => GetProjectFileStatuses(projectId),
      getGitState: () => GetProjectGitState(projectId),
      stageFile: (path) => StageProjectFile(projectId, path),
      stageFiles: (paths) => StageProjectFiles(projectId, paths),
      unstageFile: (path) => UnstageProjectFile(projectId, path),
      unstageFiles: (paths) => UnstageProjectFiles(projectId, paths),
      stageAll: () => StageProjectChanges(projectId),
      unstageAll: () => UnstageProjectChanges(projectId),
      commit: (message, amend) => CommitProject(projectId, message, amend),
      push: (force) => PushProject(projectId, force),
      sync: (force) => SyncProject(projectId, force),
      generateCommitMessage: () => GenerateCommitMessageForProject(projectId),
    }),
    [projectId],
  );

  const onOpenFile = useCallback(
    (path: string, line: number) => {
      if (!project?.path) return;
      OpenInIde(project.path, path, line, 0).catch(() => {});
    },
    [project?.path],
  );

  return <GitChangesPanel ops={ops} pollInterval={2000} resetKey={projectId} refreshRef={refreshRef} onOpenFile={project?.path ? onOpenFile : undefined} />;
}
