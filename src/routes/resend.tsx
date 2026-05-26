import { createFileRoute } from '@tanstack/react-router';
import { ResendPage } from '@/features/integrations/resend/resend-page';

export const Route = createFileRoute('/resend')({
  component: ResendPage,
});
