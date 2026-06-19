import { Files } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/atoms/page-header';
import { useCurrentProject } from '@/state/projects';
import { FilesTab } from './files-tab';

export function FilesPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();

  if (!project?.path) {
    return (
      <div className="flex h-full items-center justify-center p-4">
        <p className="text-sm text-muted-foreground">{t('files.noProject')}</p>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="shrink-0 p-4">
        <PageHeader icon={<Files className="size-5 text-muted-foreground" />} title={t('files.title')} subtitle={<span title={project.path}>{project.path}</span>} />
      </div>

      <div className="flex h-0 flex-1">
        <FilesTab projectPath={project.path} />
      </div>
    </div>
  );
}
