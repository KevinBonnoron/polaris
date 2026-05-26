import { useMemo } from 'react';
import { type GitChangesOps, GitChangesPanel } from '@/features/git/git-changes-panel';
import type { Agent } from '@/types';
import { CommitAgent, GetAgentDiff, GetAgentFileStatuses, GetAgentGitState, PushAgent, StageAgentChanges, StageAgentFile, StageAgentFiles, SyncAgent, UnstageAgentChanges, UnstageAgentFile, UnstageAgentFiles } from '@/wailsjs/go/main/App';

export function AgentDetailFilesTab({ agent }: { agent: Agent | null }) {
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
    };
  }, [agent?.id]);

  if (!ops) {
    return null;
  }

  return <GitChangesPanel ops={ops} pollInterval={agent?.status === 'working' ? 2000 : 0} resetKey={agent?.id} />;
}
