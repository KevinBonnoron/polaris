import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Area, AreaChart, Bar, BarChart, CartesianGrid, Cell, XAxis, YAxis } from 'recharts';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from '@/components/ui/chart';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { formatTokens } from '@/lib/format';
import { GetDashboardStats } from '@/wailsjs/go/main/App';
import type { polaris } from '@/wailsjs/go/models';

const CHART_COLORS = ['#818cf8', '#34d399', '#fb923c', '#e879f9', '#38bdf8'];

function formatCost(usd: number): string {
  if (usd <= 0) {
    return '$0.00';
  }
  if (usd < 0.01) {
    return `$${usd.toFixed(4)}`;
  }
  return `$${usd.toFixed(2)}`;
}

function formatDuration(sec: number): string {
  if (sec <= 0) {
    return '—';
  }
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (h > 0) {
    return `${h}h ${m}m`;
  }
  if (m > 0) {
    return `${m}m`;
  }
  return `${Math.round(sec)}s`;
}

function formatDate(iso: string): string {
  return new Date(`${iso}T00:00:00`).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

function EmptyChart({ height }: { height: string }) {
  const { t } = useTranslation();
  return (
    <div className={`flex ${height} items-center justify-center`}>
      <p className="text-xs text-muted-foreground/50">{t('dashboard.noData')}</p>
    </div>
  );
}

function StatCard({ label, value, loading }: { label: string; value: string; loading: boolean }) {
  return (
    <Card className="gap-2 py-4">
      <CardHeader className="px-4 pb-0">
        <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{label}</CardTitle>
      </CardHeader>
      <CardContent className="px-4">{loading ? <Skeleton className="h-7 w-20" /> : <p className="text-2xl font-bold tabular-nums">{value}</p>}</CardContent>
    </Card>
  );
}

const AREA_COLOR = '#818cf8';

const ALL = '__all__';

export function DashboardPage() {
  const { t } = useTranslation();
  const seriesLabel = t('dashboard.charts.seriesLabel');
  const areaChartConfig = useMemo(() => ({ sessions: { label: seriesLabel, color: AREA_COLOR } }) satisfies ChartConfig, [seriesLabel]);
  const kindChartConfig = useMemo(() => ({ sessions: { label: seriesLabel, color: 'var(--chart-1)' } }) satisfies ChartConfig, [seriesLabel]);
  const modelChartConfig = useMemo(() => ({ sessions: { label: seriesLabel, color: 'var(--chart-1)' } }) satisfies ChartConfig, [seriesLabel]);
  const [stats, setStats] = useState<polaris.DashboardStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [kindFilter, setKindFilter] = useState(ALL);
  const [modelFilter, setModelFilter] = useState(ALL);
  // Kinds come from the unfiltered load; models are scoped to the selected
  // kind so the dropdown never offers a model with no data for that kind.
  const [allKinds, setAllKinds] = useState<string[]>([]);
  const [allModels, setAllModels] = useState<string[]>([]);
  const requestSeq = useRef(0);

  const fetchStats = useCallback(async (kind: string, model: string) => {
    const seq = ++requestSeq.current;
    setLoading(true);
    try {
      const data = await GetDashboardStats(kind === ALL ? '' : kind, model === ALL ? '' : model);
      if (seq !== requestSeq.current) {
        return;
      }
      setStats(data);
      if (kind === ALL && model === ALL) {
        setAllKinds(data.byKind?.map((k) => k.kind) ?? []);
      }
      if (model === ALL) {
        setAllModels(data.byModel?.map((m) => m.model) ?? []);
      }
    } catch {
      if (seq === requestSeq.current) {
        setStats(null);
      }
    } finally {
      if (seq === requestSeq.current) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    fetchStats(kindFilter, modelFilter);
  }, [fetchStats, kindFilter, modelFilter]);

  const statCards = [
    { key: 'totalSessions', value: stats ? stats.totalSessions.toLocaleString() : '0' },
    { key: 'avgDuration', value: stats ? formatDuration(stats.avgDurationSec) : '—' },
    { key: 'avgTokensPerSession', value: stats ? formatTokens(Math.round(stats.avgTokensPerSession)) : '0' },
    { key: 'avgCost', value: stats ? formatCost(stats.avgCostUsd) : '$0.00' },
    { key: 'totalCost', value: stats ? formatCost(stats.totalCostUsd) : '$0.00' },
  ] as const;

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="flex shrink-0 items-center justify-between border-b px-6 py-4">
        <div>
          <h1 className="text-base font-semibold">{t('dashboard.title')}</h1>
          <p className="text-xs text-muted-foreground">{t('dashboard.description')}</p>
        </div>
        <div className="flex gap-2">
          <Select
            value={kindFilter}
            onValueChange={(v) => {
              setKindFilter(v);
              setModelFilter(ALL);
            }}
          >
            <SelectTrigger className="h-8 w-36 text-xs">
              <SelectValue placeholder={t('dashboard.filters.kind')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('dashboard.filters.allKinds')}</SelectItem>
              {allKinds.map((k) => (
                <SelectItem key={k} value={k}>
                  {k}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={modelFilter} onValueChange={setModelFilter}>
            <SelectTrigger className="h-8 w-48 text-xs">
              <SelectValue placeholder={t('dashboard.filters.model')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>{t('dashboard.filters.allModels')}</SelectItem>
              {allModels.map((m) => (
                <SelectItem key={m} value={m}>
                  {m}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <ScrollArea className="flex-1">
        <div className="flex flex-col gap-4 p-6">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
            {statCards.map(({ key, value }) => (
              <StatCard key={key} label={t(`dashboard.stats.${key}`)} value={value} loading={loading} />
            ))}
          </div>

          <Card className="gap-3 py-4">
            <CardHeader className="px-5 pb-0">
              <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('dashboard.charts.sessionsByDay')}</CardTitle>
            </CardHeader>
            <CardContent className="px-2">
              {loading ? (
                <Skeleton className="mx-3 h-56" />
              ) : !stats?.totalSessions ? (
                <EmptyChart height="h-56" />
              ) : (
                <ChartContainer config={areaChartConfig} className="h-56 w-full">
                  <AreaChart data={stats?.byDay ?? []} margin={{ top: 8, right: 8, bottom: 0, left: -8 }}>
                    <defs>
                      <linearGradient id="sessions-gradient" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor={AREA_COLOR} stopOpacity={0.55} />
                        <stop offset="60%" stopColor={AREA_COLOR} stopOpacity={0.15} />
                        <stop offset="100%" stopColor={AREA_COLOR} stopOpacity={0} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid vertical={false} strokeDasharray="3 3" className="stroke-border/30" />
                    <XAxis dataKey="date" tickLine={false} axisLine={false} tick={{ fontSize: 11, fill: 'var(--muted-foreground)' }} tickFormatter={formatDate} interval="preserveStartEnd" />
                    <YAxis tickLine={false} axisLine={false} tick={{ fontSize: 11, fill: 'var(--muted-foreground)' }} allowDecimals={false} width={28} />
                    <ChartTooltip content={<ChartTooltipContent labelFormatter={(v) => formatDate(v as string)} />} />
                    <Area type="monotone" dataKey="sessions" stroke={AREA_COLOR} strokeWidth={2} fill="url(#sessions-gradient)" dot={false} activeDot={{ r: 4, fill: AREA_COLOR, strokeWidth: 0 }} />
                  </AreaChart>
                </ChartContainer>
              )}
            </CardContent>
          </Card>

          <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
            <Card className="gap-3 py-4">
              <CardHeader className="px-5 pb-0">
                <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('dashboard.charts.byKind')}</CardTitle>
              </CardHeader>
              <CardContent className="px-2">
                {loading ? (
                  <Skeleton className="mx-3 h-44" />
                ) : !stats?.byKind?.length ? (
                  <EmptyChart height="h-44" />
                ) : (
                  <ChartContainer key={`kind-${kindFilter}-${modelFilter}`} config={kindChartConfig} className="h-44 w-full">
                    <BarChart data={stats?.byKind ?? []} margin={{ top: 4, right: 8, bottom: 0, left: -8 }}>
                      <CartesianGrid vertical={false} strokeDasharray="3 3" className="stroke-border/40" />
                      <XAxis dataKey="kind" tickLine={false} axisLine={false} tick={{ fontSize: 11 }} />
                      <YAxis tickLine={false} axisLine={false} tick={{ fontSize: 11 }} allowDecimals={false} width={28} />
                      <ChartTooltip content={<ChartTooltipContent />} />
                      <Bar dataKey="sessions" radius={[4, 4, 0, 0]} maxBarSize={48}>
                        {(stats?.byKind ?? []).map((item, i) => (
                          <Cell key={item.kind} fill={CHART_COLORS[i % CHART_COLORS.length]} />
                        ))}
                      </Bar>
                    </BarChart>
                  </ChartContainer>
                )}
              </CardContent>
            </Card>

            <Card className="gap-3 py-4">
              <CardHeader className="px-5 pb-0">
                <CardTitle className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t('dashboard.charts.byModel')}</CardTitle>
              </CardHeader>
              <CardContent className="px-2">
                {loading ? (
                  <Skeleton className="mx-3 h-44" />
                ) : !stats?.byModel?.length ? (
                  <EmptyChart height="h-44" />
                ) : (
                  <ChartContainer key={`model-${kindFilter}-${modelFilter}`} config={modelChartConfig} className="h-44 w-full">
                    <BarChart data={stats?.byModel ?? []} layout="vertical" margin={{ top: 0, right: 8, bottom: 0, left: 0 }}>
                      <XAxis type="number" tickLine={false} axisLine={false} tick={{ fontSize: 11 }} allowDecimals={false} />
                      <YAxis type="category" dataKey="model" tickLine={false} axisLine={false} tick={{ fontSize: 11 }} width={148} tickFormatter={(v: string) => (v.length > 20 ? `${v.slice(0, 20)}…` : v)} />
                      <ChartTooltip content={<ChartTooltipContent />} />
                      <Bar dataKey="sessions" radius={[0, 4, 4, 0]} maxBarSize={28}>
                        {(stats?.byModel ?? []).map((item, i) => (
                          <Cell key={item.model} fill={CHART_COLORS[i % CHART_COLORS.length]} />
                        ))}
                      </Bar>
                    </BarChart>
                  </ChartContainer>
                )}
              </CardContent>
            </Card>
          </div>
        </div>
      </ScrollArea>
    </div>
  );
}
