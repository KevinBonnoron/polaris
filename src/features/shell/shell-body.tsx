import type { ReactNode } from 'react';
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar';
import { NodejsTerminal } from '@/features/integrations/nodejs/nodejs-terminal';
import { PythonTerminal } from '@/features/integrations/python/python-terminal';
import { WelcomeWizard } from '@/features/projects/welcome-wizard';
import { useProjects } from '@/state/projects';
import { AppSidebar } from './app-sidebar';
import { PageLoader } from './page-loader';
import { ShortcutDispatcher } from './shortcut-dispatcher';

interface Props {
  children: ReactNode;
}

export function ShellBody({ children }: Props) {
  const { projects, ready } = useProjects();

  if (!ready) {
    return (
      <main className="h-screen">
        <PageLoader />
      </main>
    );
  }

  if (projects.length === 0) {
    return (
      <main className="h-screen overflow-auto">
        <WelcomeWizard />
      </main>
    );
  }

  return (
    <SidebarProvider defaultOpen={false}>
      <ShortcutDispatcher />
      <AppSidebar />
      <SidebarInset className="flex h-screen flex-col overflow-hidden">
        <main className="flex-1 overflow-auto">{children}</main>
        <NodejsTerminal />
        <PythonTerminal />
      </SidebarInset>
    </SidebarProvider>
  );
}
