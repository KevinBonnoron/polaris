import { useCallback, useMemo } from 'react';
import { type GitChangesOps, GitChangesPanel } from '@/features/git/git-changes-panel';
import { useCurrentProject } from '@/state/projects';
import type { Agent } from '@/types';
import {
  ArchiveAgent,
  CommitAgent,
  DeleteAgent,
  DiscardAgentFile,
  DiscardAgentFiles,
  GenerateCommitMessageForAgent,
  GetAgentDiff,
  GetAgentFileStatuses,
  GetAgentGitState,
  GetGeneralSettings,
  OpenInIde,
  PushAgent,
  StageAgentChanges,
  StageAgentFile,
  StageAgentFiles,
  SyncAgent,
  UnstageAgentChanges,
  UnstageAgentFile,
  UnstageAgentFiles,
} from '@/wailsjs/go/main/App';
import { clearSelection } from '@/state/agent-selection';

export function AgentDetailFilesTab({ agent }: { agent: Agent | null }) {
  const { project } = useCurrentProject();
  const ops: GitChangesOps | null = useMemo(() => {
    if (!agent?.id) {
      return null;
    }
    const id = agent.id;
    return {
      getDiff: () => GetAgentDiff(id),
      getFileStatuses: () => GetAgentFileStatuses(id),
      getGitState: () => GetAgentGitState(id),
      stageFile: (path) => StageAgentFile(id, path),
      stageFiles: (paths) => StageAgentFiles(id, paths),
      unstageFile: (path) => UnstageAgentFile(id, path),
      unstageFiles: (paths) => UnstageAgentFiles(id, paths),
      stageAll: () => StageAgentChanges(id),
      unstageAll: () => UnstageAgentChanges(id),
      commit: (message, amend) => CommitAgent(id, message, amend),
      push: (force) => PushAgent(id, force),
      sync: (force) => SyncAgent(id, force),
      discardFile: (path, untracked) => DiscardAgentFile(id, path, untracked),
      discardFiles: (tracked, untracked) => DiscardAgentFiles(id, tracked, untracked),
      generateCommitMessage: () => GenerateCommitMessageForAgent(id),
    };
  }, [agent?.id]);

  const onOpenFile = useCallback(
    (path: string, line: number) => {
      if (!project?.path) return;
      OpenInIde(project.path, path, line, 0).catch(() => {});
    },
    [project?.path],
  );

  const onClose = useCallback(async () => {
    if (!agent?.id) return;
    const settings = await GetGeneralSettings().catch(() => null);
    if (settings?.agentCloseAction === 'delete') {
      await DeleteAgent(agent.id);
    } else {
      await ArchiveAgent(agent.id);
    }
    clearSelection();
  }, [agent?.id]);

  if (!ops) {
    return null;
  }

  return <GitChangesPanel ops={ops} pollInterval={agent?.status === 'working' ? 2000 : 0} resetKey={agent?.id} onOpenFile={project?.path ? onOpenFile : undefined} onClose={onClose} />;
}
