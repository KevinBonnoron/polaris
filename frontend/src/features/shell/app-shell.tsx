import { type ReactNode, useEffect } from 'react';
import { Toaster } from '@/components/ui/sonner';
import { TooltipProvider } from '@/components/ui/tooltip';
import { CSharpRunProvider } from '@/features/integrations/csharp/csharp-run-context';
import { NodejsRunProvider } from '@/features/integrations/nodejs/nodejs-run-context';
import { PythonRunProvider } from '@/features/integrations/python/python-run-context';
import { ShellRunProvider } from '@/features/integrations/shell/shell-context';
import { TaskfileRunProvider } from '@/features/integrations/taskfile/taskfile-run-context';
import { ShortcutsProvider } from '@/providers/shortcuts';
import { AgentClisProvider } from '@/state/agent-clis';
import { AgentDefaultsProvider } from '@/state/agent-defaults';
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
          <AgentDefaultsProvider>
            <NodejsRunProvider>
              <PythonRunProvider>
                <CSharpRunProvider>
                  <TaskfileRunProvider>
                    <ShellRunProvider>
                      <ShortcutsProvider>
                        <ShellBody>{children}</ShellBody>
                        <BackendHealthCheck />
                        <Toaster position="bottom-right" richColors />
                      </ShortcutsProvider>
                    </ShellRunProvider>
                  </TaskfileRunProvider>
                </CSharpRunProvider>
              </PythonRunProvider>
            </NodejsRunProvider>
          </AgentDefaultsProvider>
        </AgentClisProvider>
      </ProjectsProvider>
    </TooltipProvider>
  );
}
