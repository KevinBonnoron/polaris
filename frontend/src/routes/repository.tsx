import { createFileRoute } from '@tanstack/react-router';
import { RepositoryPage } from '@/features/integrations/repository/repository-page';

export const Route = createFileRoute('/repository')({
  component: RepositoryPage,
});
