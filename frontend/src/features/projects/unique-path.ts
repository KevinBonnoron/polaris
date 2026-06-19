import type { TFunction } from 'i18next';
import type { Project } from '@/types';

export function makeUniquePathValidator(projects: Project[], i18nKey: string, t: TFunction, excludeId?: string) {
  return ({ value }: { value: string }): string | undefined => {
    const trimmed = value.trim();
    if (!trimmed) {
      return undefined;
    }
    return projects.some((p) => p.id !== excludeId && (p.path ?? '').trim() === trimmed) ? t(i18nKey) : undefined;
  };
}
