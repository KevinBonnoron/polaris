import { useNavigate } from '@tanstack/react-router';
import { useEffect, useState } from 'react';
import { useSidebar } from '@/components/ui/sidebar';
import { startDraftAgent } from '@/features/agents/start-draft-agent';
import { useShellRun } from '@/features/integrations/shell/shell-context';
import { AddProjectModal } from '@/features/projects/add-project-modal';
import { matchesShortcut } from '@/lib/shortcuts';
import { useShortcuts } from '@/providers/shortcuts';
import { useAgentClis } from '@/state/agent-clis';
import { useCurrentProject } from '@/state/projects';
import { CommandPalette } from './command-palette';
import { ProjectSwitcher } from './project-switcher';

export function ShortcutDispatcher() {
  const { shortcuts, isMac, isRecording } = useShortcuts();
  const { toggleSidebar } = useSidebar();
  const { kinds } = useAgentClis();
  const { projectId } = useCurrentProject();
  const { startSession, paneOpen, setPaneOpen } = useShellRun();
  const navigate = useNavigate();

  const [palette, setPalette] = useState(false);
  const [switcher, setSwitcher] = useState(false);
  const [addProject, setAddProject] = useState(false);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (isRecording) {
        return;
      }

      const target = e.target as HTMLElement;
      const inInput = target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.isContentEditable;

      // Ctrl+` — toggle shell pane
      const modifierHeld = isMac ? e.metaKey : e.ctrlKey;
      if (modifierHeld && e.key === '`') {
        e.preventDefault();
        e.stopImmediatePropagation();
        if (paneOpen) {
          setPaneOpen(false);
        } else {
          void startSession();
        }
        return;
      }

      for (const def of Object.values(shortcuts)) {
        if (!matchesShortcut(e, def, isMac)) {
          continue;
        }

        if (inInput && !def.meta && def.key !== 'Escape') {
          return;
        }

        if (def.id === 'closeModal') {
          const hasOpenModal = palette || switcher || addProject;
          if (!hasOpenModal) {
            return;
          }
          e.preventDefault();
          e.stopImmediatePropagation();
          setPalette(false);
          setSwitcher(false);
          setAddProject(false);
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
            if (firstInstalled && projectId) {
              void startDraftAgent(projectId, { kindId: firstInstalled.id });
              void navigate({ to: '/' });
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
  }, [shortcuts, isMac, isRecording, toggleSidebar, kinds, projectId, navigate, palette, switcher, addProject, paneOpen, setPaneOpen, startSession]);

  return (
    <>
      <CommandPalette open={palette} onOpenChange={setPalette} />
      <ProjectSwitcher open={switcher} onOpenChange={setSwitcher} />
      <AddProjectModal open={addProject} onOpenChange={setAddProject} />
    </>
  );
}
