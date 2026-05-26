import { type ReactNode, useEffect } from 'react';
import { Toaster } from '@/components/ui/sonner';
import { TooltipProvider } from '@/components/ui/tooltip';
import { NodejsRunProvider } from '@/features/integrations/nodejs/nodejs-run-context';
import { ShortcutsProvider } from '@/providers/shortcuts';
import { AgentClisProvider } from '@/state/agent-clis';
import { ProjectsProvider } from '@/state/projects';
import { SetAppFocused } from '@/wailsjs/go/main/App';
import { BackendHealthCheck } from './backend-health-check';
import { ShellBody } from './shell-body';

interface Props {
  children: ReactNode;
}

function useReportWindowFocus() {
  useEffect(() => {
    const push = (focused: boolean) => {
      SetAppFocused(focused).catch(() => {});
    };
    const onFocus = () => push(true);
    const onBlur = () => push(false);
    const onVisibility = () => push(document.visibilityState === 'visible' && document.hasFocus());

    push(document.hasFocus());
    window.addEventListener('focus', onFocus);
    window.addEventListener('blur', onBlur);
    document.addEventListener('visibilitychange', onVisibility);
    return () => {
      window.removeEventListener('focus', onFocus);
      window.removeEventListener('blur', onBlur);
      document.removeEventListener('visibilitychange', onVisibility);
    };
  }, []);
}

export function AppShell({ children }: Props) {
  useReportWindowFocus();
  return (
    <TooltipProvider delayDuration={300}>
      <ProjectsProvider>
        <AgentClisProvider>
          <NodejsRunProvider>
            <ShortcutsProvider>
              <ShellBody>{children}</ShellBody>
              <BackendHealthCheck />
              <Toaster position="bottom-right" richColors />
            </ShortcutsProvider>
          </NodejsRunProvider>
        </AgentClisProvider>
      </ProjectsProvider>
    </TooltipProvider>
  );
}
