import { useEffect, useState } from 'react';
import { useSidebar } from '@/components/ui/sidebar';
import { AgentDetailModal } from '@/features/agents/agent-detail-modal';
import { AddProjectModal } from '@/features/projects/add-project-modal';
import { matchesShortcut } from '@/lib/shortcuts';
import { useShortcuts } from '@/providers/shortcuts';
import { useAgentClis } from '@/state/agent-clis';
import type { AgentKind } from '@/types';
import { CommandPalette } from './command-palette';
import { ProjectSwitcher } from './project-switcher';

export function ShortcutDispatcher() {
  const { shortcuts, isMac, isRecording } = useShortcuts();
  const { toggleSidebar } = useSidebar();
  const { kinds } = useAgentClis();

  const [palette, setPalette] = useState(false);
  const [switcher, setSwitcher] = useState(false);
  const [addProject, setAddProject] = useState(false);
  const [pendingAgent, setPendingAgent] = useState<AgentKind | null>(null);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (isRecording) {
        return;
      }

      const target = e.target as HTMLElement;
      const inInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable;

      for (const def of Object.values(shortcuts)) {
        if (!matchesShortcut(e, def, isMac)) {
          continue;
        }

        if (inInput && def.key !== 'Escape') {
          return;
        }

        if (def.id === 'closeModal') {
          const hasOpenModal = palette || switcher || addProject || pendingAgent !== null;
          if (!hasOpenModal) {
            return;
          }
          e.preventDefault();
          e.stopImmediatePropagation();
          setPalette(false);
          setSwitcher(false);
          setAddProject(false);
          setPendingAgent(null);
          break;
        }

        e.preventDefault();
        e.stopImmediatePropagation();

        switch (def.id) {
          case 'openPalette':
            setPalette((v) => !v);
            break;
          case 'switchProject':
            setSwitcher((v) => !v);
            break;
          case 'addProject':
            setAddProject(true);
            break;
          case 'newAgent': {
            const firstInstalled = kinds.find((k) => k.installed);
            if (firstInstalled) {
              setPendingAgent(firstInstalled.id);
            }
            break;
          }
          case 'toggleSidebar':
            toggleSidebar();
            break;
        }
        break;
      }
    };

    window.addEventListener('keydown', handler, { capture: true });
    return () => window.removeEventListener('keydown', handler, { capture: true });
  }, [shortcuts, isMac, isRecording, toggleSidebar, kinds, palette, switcher, addProject, pendingAgent]);

  return (
    <>
      <CommandPalette open={palette} onOpenChange={setPalette} />
      <ProjectSwitcher open={switcher} onOpenChange={setSwitcher} />
      <AddProjectModal open={addProject} onOpenChange={setAddProject} />
      {pendingAgent && <AgentDetailModal pending={{ kindId: pendingAgent }} open={true} onOpenChange={(o) => !o && setPendingAgent(null)} />}
    </>
  );
}
