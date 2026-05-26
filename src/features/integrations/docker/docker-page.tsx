import { Container, Layers, Settings, ShieldAlert } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@/components/atoms/page-header';
import { RefreshAction } from '@/components/atoms/refresh-action';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { imageRef } from '@/db/docker';
import { ConfigureIntegrationModal } from '@/features/integrations/configure-integration-modal';
import { toastError } from '@/lib/toast-error';
import { useCurrentProject } from '@/state/projects';
import { DockerImageHistory, LintDockerfile, ScanDockerImage } from '@/wailsjs/go/main/App';
import type { docker } from '@/wailsjs/go/models';
import { useDockerEntry } from './use-docker-entry';

interface DockerConfig {
  dockerfilePath?: string;
}

function severityVariant(severity: string): 'destructive' | 'secondary' | 'outline' {
  const s = severity.toLowerCase();
  if (s === 'critical' || s === 'high' || s === 'error') {
    return 'destructive';
  }
  if (s === 'warning' || s === 'medium') {
    return 'secondary';
  }
  return 'outline';
}

function FindingList({ findings }: { findings: docker.Finding[] }) {
  return (
    <ul className="flex flex-col gap-2">
      {findings.map((f) => (
        <li key={`${f.rule}-${f.line}-${f.message}`} className="flex items-start gap-2 rounded-md border px-3 py-2">
          <Badge variant={severityVariant(f.severity)} className="shrink-0">
            {f.rule}
          </Badge>
          <div className="min-w-0 flex-1">
            <p className="text-sm">{f.message}</p>
            {f.line > 0 && <p className="text-muted-foreground text-xs">L{f.line}</p>}
          </div>
        </li>
      ))}
    </ul>
  );
}

export function DockerPage() {
  const { t } = useTranslation();
  const { project } = useCurrentProject();
  const config = project?.integrations?.docker as DockerConfig | undefined;
  const dockerfilePath = config?.dockerfilePath ?? '';

  if (!project || !dockerfilePath) {
    return (
      <div className="flex h-full items-center justify-center p-6">
        <div className="max-w-sm text-center">
          <h2 className="font-semibold text-lg">{t('integrations.docker.detectTitle')}</h2>
          <p className="mt-2 text-muted-foreground text-sm">{t('integrations.docker.detectDesc')}</p>
        </div>
      </div>
    );
  }

  return <DockerView projectId={project.id} dockerfilePath={dockerfilePath} />;
}

function DockerView({ projectId, dockerfilePath }: { projectId: string; dockerfilePath: string }) {
  const { t } = useTranslation();
  const { images, parsed, caps, loading, reload } = useDockerEntry(dockerfilePath);

  const [layers, setLayers] = useState<Record<string, docker.Layer[]>>({});
  const [vulns, setVulns] = useState<Record<string, docker.Vulnerability[]>>({});
  const [lint, setLint] = useState<docker.Finding[] | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const hasScanner = !!caps?.trivy || !!caps?.grype;

  const toggleLayers = async (img: docker.Image) => {
    const ref = imageRef(img);
    if (layers[ref]) {
      setLayers((prev) => {
        const next = { ...prev };
        delete next[ref];
        return next;
      });
      return;
    }
    setBusy(`layers:${ref}`);
    try {
      const data = await DockerImageHistory(ref);
      setLayers((prev) => ({ ...prev, [ref]: data ?? [] }));
    } catch (err) {
      toastError({ title: t('integrations.docker.couldNotLoadLayers'), err });
    } finally {
      setBusy(null);
    }
  };

  const scan = async (img: docker.Image) => {
    const ref = imageRef(img);
    setBusy(`scan:${ref}`);
    try {
      const data = await ScanDockerImage(ref);
      setVulns((prev) => ({ ...prev, [ref]: data ?? [] }));
    } catch (err) {
      toastError({ title: t('integrations.docker.couldNotScan'), err });
    } finally {
      setBusy(null);
    }
  };

  const runLint = async () => {
    setBusy('lint');
    try {
      const data = await LintDockerfile(dockerfilePath);
      setLint(data ?? []);
    } catch (err) {
      toastError({ title: t('integrations.docker.couldNotLint'), err });
    } finally {
      setBusy(null);
    }
  };

  const stages = parsed?.stages ?? [];
  const staticFindings = parsed?.findings ?? [];

  return (
    <div className="flex h-full flex-col gap-6 overflow-auto p-6">
      <PageHeader
        icon={<Container className="size-5 text-muted-foreground" />}
        title={t('integrations.docker.title')}
        subtitle={<span title={dockerfilePath}>{dockerfilePath}</span>}
        actions={
          <div className="flex items-center gap-2">
            <RefreshAction onRefresh={() => reload()} loading={loading} />
            <ConfigureIntegrationModal projectId={projectId} integrationId="docker">
              <Button variant="outline" size="sm">
                <Settings className="size-3.5" />
                {t('common.configure')}
              </Button>
            </ConfigureIntegrationModal>
          </div>
        }
      />

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('integrations.docker.structure')}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <ul className="flex flex-col gap-2">
            {stages.map((stage, i) => (
              <li key={`${stage.baseImage}-${i}`} className="flex flex-wrap items-center gap-2 rounded-md border px-3 py-2">
                <span className="font-medium text-sm">{stage.baseImage || '—'}</span>
                {stage.name && <span className="text-muted-foreground text-xs">AS {stage.name}</span>}
                {stage.digest ? <Badge variant="outline">{t('integrations.docker.pinned')}</Badge> : !stage.tag || stage.tag === 'latest' ? <Badge variant="secondary">{t('integrations.docker.notPinned')}</Badge> : null}
                {stage.final && <Badge>{t('integrations.docker.finalStage')}</Badge>}
              </li>
            ))}
          </ul>
          <div className="flex flex-wrap gap-2 text-sm">
            <Badge variant={parsed?.user ? 'outline' : 'destructive'}>
              {t('integrations.docker.user')}: {parsed?.user || t('integrations.docker.root')}
            </Badge>
            <Badge variant={parsed?.hasHealthcheck ? 'outline' : 'secondary'}>
              {t('integrations.docker.healthcheck')}: {parsed?.hasHealthcheck ? t('integrations.docker.present') : t('integrations.docker.absent')}
            </Badge>
            {(parsed?.exposedPorts ?? []).map((p) => (
              <Badge key={p} variant="outline">
                {t('integrations.docker.port')} {p}
              </Badge>
            ))}
          </div>
          {staticFindings.length > 0 && <FindingList findings={staticFindings} />}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle className="text-base">{t('integrations.docker.lint')}</CardTitle>
          <Button size="sm" variant="outline" disabled={!caps?.hadolint || busy === 'lint'} onClick={runLint}>
            {busy === 'lint' ? t('integrations.docker.scanning') : t('integrations.docker.runLint')}
          </Button>
        </CardHeader>
        <CardContent>
          {!caps?.hadolint ? (
            <p className="text-muted-foreground text-sm">{t('integrations.docker.hadolintMissing')}</p>
          ) : lint === null ? (
            <p className="text-muted-foreground text-sm">{t('integrations.docker.lintIdle')}</p>
          ) : lint.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('integrations.docker.lintClean')}</p>
          ) : (
            <FindingList findings={lint} />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('integrations.docker.images')}</CardTitle>
        </CardHeader>
        <CardContent>
          {!caps?.dockerDaemon ? (
            <p className="text-muted-foreground text-sm">{t('integrations.docker.daemonOff')}</p>
          ) : images.length === 0 ? (
            <p className="text-muted-foreground text-sm">{t('integrations.docker.noBaseImages')}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('integrations.docker.baseImage')}</TableHead>
                  <TableHead className="text-right">{t('integrations.docker.size')}</TableHead>
                  <TableHead className="w-px" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {images.map((img) => {
                  const ref = imageRef(img);
                  const imgLayers = layers[ref];
                  const imgVulns = vulns[ref];
                  return (
                    <>
                      <TableRow key={ref}>
                        <TableCell className="font-medium">{ref}</TableCell>
                        <TableCell className="text-right tabular-nums">{img.size}</TableCell>
                        <TableCell className="flex justify-end gap-2">
                          <Button size="sm" variant="ghost" disabled={busy === `layers:${ref}`} onClick={() => toggleLayers(img)}>
                            <Layers className="size-3.5" />
                            {t('integrations.docker.layers')}
                          </Button>
                          <Button size="sm" variant="ghost" disabled={!hasScanner || busy === `scan:${ref}`} onClick={() => scan(img)}>
                            <ShieldAlert className="size-3.5" />
                            {busy === `scan:${ref}` ? t('integrations.docker.scanning') : t('integrations.docker.scanCve')}
                          </Button>
                        </TableCell>
                      </TableRow>
                      {imgLayers && (
                        <TableRow key={`${ref}-layers`}>
                          <TableCell colSpan={3}>
                            <ul className="flex flex-col gap-1">
                              {imgLayers.map((layer, i) => (
                                <li key={`${ref}-l${i}`} className="flex items-center justify-between gap-3 text-xs">
                                  <span className="truncate font-mono text-muted-foreground">{layer.createdBy || '—'}</span>
                                  <span className="shrink-0 tabular-nums">{layer.size}</span>
                                </li>
                              ))}
                            </ul>
                          </TableCell>
                        </TableRow>
                      )}
                      {imgVulns && (
                        <TableRow key={`${ref}-vulns`}>
                          <TableCell colSpan={3}>
                            {imgVulns.length === 0 ? (
                              <p className="text-muted-foreground text-sm">{t('integrations.docker.scanClean')}</p>
                            ) : (
                              <ul className="flex flex-col gap-1">
                                {imgVulns.map((v) => (
                                  <li key={`${v.id}-${v.package}`} className="flex flex-wrap items-center gap-2 text-xs">
                                    <Badge variant={severityVariant(v.severity)} className="shrink-0">
                                      {v.severity}
                                    </Badge>
                                    <span className="font-mono">{v.id}</span>
                                    <span className="text-muted-foreground">
                                      {v.package} {v.installed} → {v.fixedVersion}
                                    </span>
                                  </li>
                                ))}
                              </ul>
                            )}
                          </TableCell>
                        </TableRow>
                      )}
                    </>
                  );
                })}
              </TableBody>
            </Table>
          )}
          {caps?.dockerDaemon && !hasScanner && <p className="mt-3 text-muted-foreground text-xs">{t('integrations.docker.scannerMissing')}</p>}
        </CardContent>
      </Card>
    </div>
  );
}
