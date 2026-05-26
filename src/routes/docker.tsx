import { createFileRoute } from '@tanstack/react-router';
import { DockerPage } from '@/features/integrations/docker/docker-page';

export const Route = createFileRoute('/docker')({
  component: DockerPage,
});
