import { createFileRoute } from '@tanstack/react-router';
import { DokployPage } from '@/features/integrations/dokploy/dokploy-page';

export const Route = createFileRoute('/dokploy')({
  component: DokployPage,
});
