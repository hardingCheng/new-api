/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery, type UseQueryResult } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { ExternalLink, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/design-system/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/design-system/table'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import { getWorkbenchSummary } from './api'
import type {
  WorkbenchAlarm,
  WorkbenchDailyPoint,
  WorkbenchSite,
  WorkbenchStatusBar,
  WorkbenchSummary,
  WorkbenchSummaryResponse,
  WorkbenchWatermark,
} from './types'

const POLL_INTERVAL_MS = 60_000

function formatMoney(value: number | null): string {
  if (value == null) return '—'
  const sign = value >= 0 ? '+' : '-'
  return `${sign}$${Math.abs(value).toFixed(2)}`
}

function formatCount(value: number | null): string {
  if (value == null) return '—'
  return value.toLocaleString()
}

function formatClock(ts: number | null): string {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleString(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function profitTone(value: number | null): string {
  if (value == null) return ''
  if (value >= 0) return 'text-status-success'
  return 'text-destructive'
}

function alarmTone(bad: number, warn: number): string {
  if (bad) return 'text-destructive'
  if (warn) return 'text-status-warning'
  return 'text-status-success'
}

function StatusCards({ statusBar }: { statusBar: WorkbenchStatusBar }) {
  const { t } = useTranslation()
  const cards = [
    {
      label: t('Today gross profit (quota basis)'),
      value: formatMoney(statusBar.pnl24),
      tone: profitTone(statusBar.pnl24),
    },
    {
      label: t('Alerts (urgent / watch)'),
      value: `${statusBar.alarm_bad} / ${statusBar.alarm_warn}`,
      tone: alarmTone(statusBar.alarm_bad, statusBar.alarm_warn),
    },
    {
      label: t('Disabled channels'),
      value: String(statusBar.disabled_channels),
      tone: statusBar.disabled_channels ? 'text-status-warning' : '',
    },
    {
      label: t('Channels tripping (last hour)'),
      value: String(statusBar.breaking_channels),
      tone: statusBar.breaking_channels ? 'text-status-warning' : '',
    },
    {
      label: t('Low-balance upstreams'),
      value: String(statusBar.low_balance_sites),
      tone: statusBar.low_balance_sites ? 'text-destructive' : '',
    },
    {
      label: t('Monitor data updated at'),
      value: formatClock(statusBar.last_collect_ts),
      tone: 'text-base',
    },
  ]

  return (
    <div className='grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6'>
      {cards.map((card) => (
        <Card key={card.label} className='py-4'>
          <CardContent className='px-4'>
            <div className='text-muted-foreground truncate text-xs'>
              {card.label}
            </div>
            <div
              className={cn(
                'mt-1 text-xl font-semibold tabular-nums',
                card.tone
              )}
            >
              {card.value}
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

function AlarmRow({ alarm }: { alarm: WorkbenchAlarm }) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap items-baseline gap-2 border-b py-2.5 last:border-b-0'>
      <StatusBadge
        variant={alarm.level === 'bad' ? 'destructive' : 'warning'}
        appearance='soft'
        className='shrink-0'
      >
        {alarm.level === 'bad' ? t('Urgent') : t('Watch')}
      </StatusBadge>
      <span className='font-medium'>{alarm.title}</span>
      {alarm.detail && (
        <span className='text-muted-foreground min-w-0 text-xs'>
          {alarm.detail}
        </span>
      )}
      {alarm.link && (
        <a
          href={alarm.link}
          className='text-primary ms-auto shrink-0 text-sm hover:underline'
        >
          {t('Handle')} →
        </a>
      )}
    </div>
  )
}

function AlarmsCard({ alarms }: { alarms: WorkbenchAlarm[] }) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          {t('Alert list')}
          {alarms.length > 0 && (
            <span className='text-muted-foreground text-xs font-normal'>
              {alarms.length}
            </span>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {alarms.length === 0 ? (
          <p className='text-muted-foreground py-4 text-sm'>
            {t('All clear, nothing needs attention')}
          </p>
        ) : (
          alarms.map((alarm) => (
            <AlarmRow
              key={`${alarm.kind}:${alarm.title}:${alarm.detail}`}
              alarm={alarm}
            />
          ))
        )}
      </CardContent>
    </Card>
  )
}

function TrendCard({ daily }: { daily: WorkbenchDailyPoint[] }) {
  const { t } = useTranslation()
  const max = Math.max(...daily.map((d) => d.requests), 1)
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Requests (last 7 days)')}</CardTitle>
      </CardHeader>
      <CardContent>
        {daily.length === 0 ? (
          <p className='text-muted-foreground py-4 text-sm'>{t('No data')}</p>
        ) : (
          <div className='flex h-28 items-end gap-1.5'>
            {daily.map((d) => (
              <div
                key={d.day_ts}
                className='flex min-w-0 flex-1 flex-col items-center justify-end gap-1'
                title={`${formatCount(d.requests)}`}
              >
                <span className='text-muted-foreground text-[10px] tabular-nums'>
                  {d.requests >= 10000
                    ? `${(d.requests / 10000).toFixed(1)}w`
                    : d.requests}
                </span>
                <div
                  className='bg-primary/70 w-full max-w-8 rounded-t'
                  style={{
                    height: `${Math.max(4, Math.round((d.requests / max) * 72))}px`,
                  }}
                />
                <span className='text-muted-foreground text-[10px]'>
                  {new Date(d.day_ts * 1000).toLocaleDateString(undefined, {
                    month: 'numeric',
                    day: 'numeric',
                  })}
                </span>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function WatermarkMeter({
  label,
  value,
  line,
}: {
  label: string
  value: number | null
  line: number
}) {
  const pct = value == null ? 0 : Math.min(100, (value / line) * 100)
  return (
    <div>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div className='mt-1 text-lg font-semibold tabular-nums'>
        {formatCount(value)}{' '}
        <span className='text-muted-foreground text-xs font-normal'>
          / {formatCount(line)}
        </span>
      </div>
      <Progress
        value={pct}
        className={cn(
          'mt-2 h-1.5',
          pct >= 100 && '[&>*]:bg-destructive',
          pct >= 60 && pct < 100 && '[&>*]:bg-status-warning'
        )}
      />
    </div>
  )
}

function WatermarkCard({
  watermark,
}: {
  watermark: WorkbenchWatermark | null
}) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Capacity watermark')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-4'>
        {watermark == null ? (
          <p className='text-muted-foreground py-4 text-sm'>
            {t('Hub database is temporarily unreadable')}
          </p>
        ) : (
          <>
            <WatermarkMeter
              label={t('Peak requests per minute (24h)')}
              value={watermark.peak_rpm}
              line={watermark.peak_rpm_line}
            />
            <WatermarkMeter
              label={t('Total request log rows')}
              value={watermark.logs_rows}
              line={watermark.logs_rows_line}
            />
          </>
        )}
      </CardContent>
    </Card>
  )
}

function SitesCard({ sites }: { sites: WorkbenchSite[] }) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('Upstream balances')}</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Upstream')}</TableHead>
              <TableHead>{t('Balance')}</TableHead>
              <TableHead>{t('Days remaining')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sites.map((site) => (
              <TableRow key={site.host}>
                <TableCell>
                  <span className='font-medium'>{site.name}</span>{' '}
                  <span className='text-muted-foreground text-xs'>
                    {site.host}
                  </span>
                </TableCell>
                <TableCell className='tabular-nums'>
                  {site.balance == null ? '—' : `$${site.balance.toFixed(2)}`}
                </TableCell>
                <TableCell
                  className={cn(
                    'tabular-nums',
                    site.est_days != null &&
                      site.est_days <= 3 &&
                      'text-destructive font-semibold',
                    site.est_days != null &&
                      site.est_days > 3 &&
                      site.needs_topup &&
                      'text-status-warning'
                  )}
                >
                  {site.est_days == null
                    ? '—'
                    : t('{{count}} days', { count: site.est_days })}
                </TableCell>
                <TableCell>
                  {site.needs_topup && (
                    <StatusBadge variant='warning' appearance='soft'>
                      {t('Top-up needed this month')}
                    </StatusBadge>
                  )}
                  {site.error && (
                    <span className='text-destructive text-xs'>
                      {site.error.slice(0, 80)}
                    </span>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

function WorkbenchBody({ summary }: { summary: WorkbenchSummary }) {
  return (
    <div className='space-y-4'>
      <StatusCards statusBar={summary.status_bar} />
      {summary.status_bar.hub_db_error && (
        <p className='text-destructive text-xs'>
          {summary.status_bar.hub_db_error}
        </p>
      )}
      <AlarmsCard alarms={summary.alarms} />
      <div className='grid gap-4 lg:grid-cols-2'>
        <TrendCard daily={summary.daily} />
        <WatermarkCard watermark={summary.watermark} />
      </div>
      <SitesCard sites={summary.sites} />
    </div>
  )
}

function renderWorkbenchContent(
  query: UseQueryResult<WorkbenchSummaryResponse>,
  t: TFunction
) {
  if (query.isLoading) {
    return (
      <div className='space-y-4'>
        <Skeleton className='h-20 w-full' />
        <Skeleton className='h-40 w-full' />
        <Skeleton className='h-40 w-full' />
      </div>
    )
  }
  const resp = query.data
  if (!resp?.success || !resp.data) {
    return (
      <ErrorState
        title={t('Workbench data service unavailable')}
        description={resp?.message}
        onRetry={() => query.refetch()}
      />
    )
  }
  return <WorkbenchBody summary={resp.data} />
}

export function Workbench() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['workbench-summary'],
    queryFn: getWorkbenchSummary,
    refetchInterval: POLL_INTERVAL_MS,
  })

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-2'>
          <span className='truncate'>{t('Ops Workbench')}</span>
          <StatusBadge appearance='outline' className='shrink-0'>
            Root
          </StatusBadge>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          onClick={() => window.open('/_watch/', '_blank', 'noreferrer')}
        >
          <ExternalLink />
          {t('Upstream monitor')}
        </Button>
        <Button
          variant='outline'
          size='sm'
          onClick={() => query.refetch()}
          disabled={query.isFetching}
        >
          <RefreshCw className={cn(query.isFetching && 'animate-spin')} />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        {renderWorkbenchContent(query, t)}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
