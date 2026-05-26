import { useLiveQuery } from '@tanstack/react-db';
import { Plus, Zap } from 'lucide-react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { automationsCollection } from '@/collections/automations.collection';
import { PageHeader } from '@/components/atoms/page-header';
import { Button } from '@/components/ui/button';
import { PageLoader } from '@/features/shell/page-loader';
import { useCurrentProject } from '@/state/projects';
import { AutomationCard } from './automation-card';
import { AutomationEditModal } from './automation-edit-modal';
import { AutomationsEmpty } from './automations-empty';
import { projectHasAutomatable } from './eligibility';

export function AutomationsPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();

  const { data: automations = [], isReady } = useLiveQuery((q) => q.from({ a: automationsCollection }));

  const eligible = project ? projectHasAutomatable(project) : false;
  const projectAutomations = useMemo(() => (project ? automations.filter((a) => a.projectId === project.id) : []), [automations, project]);

  return (
    <div className="flex h-full flex-col gap-6 p-4">
      <PageHeader
        icon={<Zap className="size-5 text-muted-foreground" />}
        title={t('automations.title')}
        subtitle={t('automations.description')}
        actions={
          eligible && project ? (
            <AutomationEditModal projectId={project.id}>
              <Button size="sm">
                <Plus className="size-3.5" />
                {t('automations.new')}
              </Button>
            </AutomationEditModal>
          ) : null
        }
      />

      {!isReady ? (
        <PageLoader />
      ) : !project ? (
        <p className="text-sm text-muted-foreground">{t('automations.noProject')}</p>
      ) : !eligible ? (
        <p className="text-sm text-muted-foreground">{t('automations.noEligible')}</p>
      ) : projectAutomations.length === 0 ? (
        <AutomationsEmpty projectId={project.id} />
      ) : (
        <div className="flex flex-col gap-2">
          {projectAutomations.map((a) => (
            <AutomationCard key={a.id} automation={a} />
          ))}
        </div>
      )}
    </div>
  );
}
