import { Plug } from 'lucide-react';
import type { ReactNode } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { ConfigureIntegrationModal } from './configure-integration-modal';

export function SelectProjectPrompt({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-full items-center justify-center p-4">
      <p className="text-sm text-muted-foreground">{children}</p>
    </div>
  );
}

interface NotConnectedCardProps {
  projectId: string;
  integrationId: string;
  title: string;
  subtitle: string;
  connectTitle: string;
  connectDesc: string;
  cta: string;
}

// Strings are passed in already-translated so each page keeps its own type-safe t() calls.
export function NotConnectedCard({ projectId, integrationId, title, subtitle, connectTitle, connectDesc, cta }: NotConnectedCardProps) {
  return (
    <div className="flex h-full flex-col gap-6 p-4">
      <header className="flex flex-col gap-1">
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        <p className="text-sm text-muted-foreground">{subtitle}</p>
      </header>
      <Card className="border-dashed">
        <CardHeader className="items-center text-center">
          <div className="mb-3 flex size-12 items-center justify-center rounded-full bg-muted">
            <Plug className="size-5 text-muted-foreground" />
          </div>
          <CardTitle className="text-base">{connectTitle}</CardTitle>
          <CardDescription>{connectDesc}</CardDescription>
        </CardHeader>
        <CardContent className="flex justify-center">
          <ConfigureIntegrationModal projectId={projectId} integrationId={integrationId}>
            <Button>{cta}</Button>
          </ConfigureIntegrationModal>
        </CardContent>
      </Card>
    </div>
  );
}
