import { useTranslation } from 'react-i18next';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import type { IntegrationConfig } from '@/types';
import type { Integration } from './integration-catalog';

interface Props {
  integration: Integration;
  instances: IntegrationConfig[];
  selectedIndex: number;
  onSelect: (index: number) => void;
  projectPath: string;
}

export function InstanceSelector({ integration, instances, selectedIndex, onSelect, projectPath }: Props) {
  const { t } = useTranslation();

  if (instances.length <= 1) {
    return null;
  }

  const label = (config: IntegrationConfig, i: number) => integration.instanceLabel?.(config, projectPath) ?? `${t('integrations.instances.fallback')} ${i + 1}`;

  return (
    <Select value={String(selectedIndex)} onValueChange={(v) => onSelect(Number(v))}>
      <SelectTrigger className="h-8 w-auto min-w-[160px]">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {instances.map((instance, i) => (
          <SelectItem key={i} value={String(i)}>
            {label(instance, i)}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
