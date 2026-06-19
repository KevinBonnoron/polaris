import { createFileRoute } from '@tanstack/react-router';
import { SentryPage } from '@/features/integrations/sentry-page';

export const Route = createFileRoute('/sentry')({
  component: SentryPage,
});
