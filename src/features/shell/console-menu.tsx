import { Check, TerminalSquare } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Popover, PopoverClose, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { SidebarMenuButton } from '@/components/ui/sidebar';
import { useCurrentProject } from '@/state/projects';
import { DetectTerminals, OpenTerminal } from '@/wailsjs/go/main/App';
import type { terminal } from '@/wailsjs/go/models';

export function ConsoleMenu() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const [terminals, setTerminals] = useState<terminal.Terminal[]>([]);

  const refresh = useCallback(async () => {
    try {
      const res = await DetectTerminals();
      setTerminals(res ?? []);
    } catch {
      setTerminals([]);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const installed = terminals
    .filter((tt) => tt.installed)
    .slice()
    .sort((a, b) => Number(Boolean(b.default)) - Number(Boolean(a.default)));
  const projectPath = project?.path ?? '';
  const disabled = !projectPath;

  const launch = (id: string) => {
    if (!projectPath) {
      return;
    }
    OpenTerminal(id, projectPath).catch((err) => {
      console.error(t('sidebar.consoleFailed'), err);
    });
  };

  return (
    <Popover onOpenChange={(next) => next && void refresh()}>
      <PopoverTrigger asChild>
        <SidebarMenuButton tooltip={t('sidebar.console')}>
          <TerminalSquare />
          <span>{t('sidebar.console')}</span>
        </SidebarMenuButton>
      </PopoverTrigger>
      <PopoverContent side="right" align="start" sideOffset={8} className="w-64 p-1" onCloseAutoFocus={(e) => e.preventDefault()}>
        <div className="px-2 py-1.5 text-sm font-medium">{t('sidebar.console')}</div>
        <div className="-mx-1 my-1 h-px bg-border" />
        {disabled && <div className="px-2 py-1.5 text-sm text-muted-foreground">{t('sidebar.consoleNoProject')}</div>}
        {!disabled && installed.length === 0 && <div className="px-2 py-1.5 text-sm text-muted-foreground">{t('sidebar.consoleNoTerminal')}</div>}
        {!disabled &&
          installed.map((tt) => (
            <PopoverClose asChild key={tt.id}>
              <button type="button" onClick={() => launch(tt.id)} className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground">
                <TerminalSquare className="size-3.5 text-muted-foreground" />
                <span className="flex-1">{tt.name}</span>
                {tt.default && (
                  <span className="inline-flex items-center gap-1 rounded bg-accent px-1.5 py-0.5 text-[10px] uppercase text-muted-foreground">
                    <Check className="size-3" />
                    {t('sidebar.consoleDefault')}
                  </span>
                )}
              </button>
            </PopoverClose>
          ))}
      </PopoverContent>
    </Popover>
  );
}
