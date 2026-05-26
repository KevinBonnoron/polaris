import { createFileRoute } from '@tanstack/react-router';
import { SettingsPage } from '@/features/settings/settings-page';
import type { SettingsSection } from '@/features/settings/settings-nav';

const VALID_SECTIONS: SettingsSection[] = ['general', 'appearance', 'agents', 'notifications', 'shortcuts', 'account', 'about'];

export const Route = createFileRoute('/settings')({
  validateSearch: (search: Record<string, unknown>) => ({
    section: (VALID_SECTIONS.includes(search.section as SettingsSection) ? search.section : 'general') as SettingsSection,
  }),
  component: SettingsPage,
});
