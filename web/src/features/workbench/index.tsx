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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  Alert,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import { getChatDumpViewerUrl, getWorkbenchSummary } from './api'
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

function profitTone(value: number | null): string {
  if (value == null) return ''
  if (value >= 0) return 'text-status-success'
  return 'text-destructive'
}

// 只在真出事时出现。平时这里什么都没有——常驻的「一切正常」是噪音，
// 会让人习惯性略过，真出事那天也照样略过。
function CriticalBanner({ alarms }: { alarms: WorkbenchAlarm[] }) {
  const { t } = useTranslation()
  const bad = alarms.filter((a) => a.level === 'bad')
  if (bad.length === 0) return null
  const top = bad[0]
  return (
    <Alert variant='destructive'>
      <AlertTitle>
        {t('{{count}} items need handling', { count: bad.length })}
        {' · '}
        {top.title}
      </AlertTitle>
      {top.detail && <AlertDescription>{top.detail}</AlertDescription>}
    </Alert>
  )
}

function MetricTile({
  label,
  value,
  hint,
  tone,
}: {
  label: string
  value: string
  hint?: string
  tone?: string
}) {
  return (
    <Card className='py-4'>
      <CardContent className='px-4'>
        <div className='text-muted-foreground truncate text-xs'>{label}</div>
        <div className={cn('mt-1 text-2xl font-semibold tabular-nums', tone)}>
          {value}
        </div>
        {hint && (
          <div className='text-muted-foreground mt-0.5 truncate text-xs'>
            {hint}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

// 固定四格，位置不变。数量固定才形成得了肌肉记忆，每天扫同一个地方。
// 只放「此刻的状态」和老板每天都要看的利润，累计统计一律下沉到下面的卡片。
function NowMetrics({
  statusBar,
  sites,
}: {
  statusBar: WorkbenchStatusBar
  sites: WorkbenchSite[]
}) {
  const { t } = useTranslation()
  const soonest = sites
    .filter((s) => s.est_days != null)
    .sort((a, b) => (a.est_days ?? 0) - (b.est_days ?? 0))[0]
  const coverage = statusBar.pnl24_coverage
  return (
    <div className='grid grid-cols-2 gap-3 xl:grid-cols-4'>
      <MetricTile
        label={t('Today gross profit (quota basis)')}
        value={formatMoney(statusBar.pnl24)}
        tone={profitTone(statusBar.pnl24)}
        hint={
          coverage != null && coverage < 0.999
            ? t('covers {{pct}}% of revenue', {
                pct: Math.round(coverage * 100),
              })
            : undefined
        }
      />
      <MetricTile
        label={t('Channels tripping (last hour)')}
        value={String(statusBar.breaking_channels)}
        tone={statusBar.breaking_channels ? 'text-destructive' : undefined}
      />
      <MetricTile
        label={t('Soonest upstream to run dry')}
        value={
          soonest?.est_days != null ? t('{{d}} d', { d: soonest.est_days }) : '—'
        }
        tone={
          soonest?.est_days != null && soonest.est_days <= 3
            ? 'text-destructive'
            : undefined
        }
        hint={soonest?.name}
      />
      <MetricTile
        label={t('Disabled channels')}
        value={String(statusBar.disabled_channels)}
        tone={statusBar.disabled_channels ? 'text-status-warning' : undefined}
      />
    </div>
  )
}

function AlarmRow({ alarm }: { alarm: WorkbenchAlarm }) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap items-baseline gap-2 border-b py-2.5 last:border-b-0'>
      <StatusBadge
        variant={alarm.level === 'bad' ? 'danger' : 'warning'}
        className='shrink-0'
      >
        {alarmKindLabel(alarm.kind, t)}
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

// 排序即结论：要花钱的和要断服务的排在最前，纯信息类沉底。
// 用户不该自己从一堆同色条目里找重点。
const ALARM_KIND_ORDER: Record<string, number> = {
  balance: 0,
  topup: 1,
  price_up: 2,
  loss: 3,
  disabled: 4,
  breaker: 5,
  error_rate: 6,
  price_down: 7,
  ratio: 8,
  collect: 9,
}

function alarmKindLabel(kind: string, t: TFunction): string {
  const map: Record<string, string> = {
    balance: t('Balance'),
    topup: t('Top-up'),
    price_up: t('Price up'),
    price_down: t('Price down'),
    loss: t('Losing money'),
    disabled: t('Disabled'),
    breaker: t('Circuit breaker'),
    error_rate: t('Error rate'),
    collect: t('Collection'),
  }
  return map[kind] ?? kind
}

function sortAlarms(alarms: WorkbenchAlarm[]): WorkbenchAlarm[] {
  return [...alarms].sort((a, b) => {
    if (a.level !== b.level) return a.level === 'bad' ? -1 : 1
    const ka = ALARM_KIND_ORDER[a.kind] ?? 99
    const kb = ALARM_KIND_ORDER[b.kind] ?? 99
    return ka - kb
  })
}

function AlarmsCard({ alarms }: { alarms: WorkbenchAlarm[] }) {
  const { t } = useTranslation()
  const sorted = sortAlarms(alarms)
  return (
    <Card className='h-full'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          {t('Needs your attention')}
          {sorted.length > 0 && (
            <span className='text-muted-foreground text-xs font-normal'>
              {sorted.length}
            </span>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {sorted.length === 0 ? (
          <p className='text-muted-foreground py-4 text-sm'>
            {t('Nothing needs handling right now')}
          </p>
        ) : (
          sorted.map((alarm) => (
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
                    <StatusBadge variant='warning'>
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

// 利润数字必须自带口径说明：缺上游成本的站算不出利润，不说清楚的话
// 这个数看着像全站合计、实际只是子集，反而误导决策。
function ProfitCard({ statusBar }: { statusBar: WorkbenchStatusBar }) {
  const { t } = useTranslation()
  const uncovered = statusBar.pnl24_uncovered_sites ?? []
  return (
    <Card className='h-full'>
      <CardHeader>
        <CardTitle>{t('Today gross profit (quota basis)')}</CardTitle>
      </CardHeader>
      <CardContent className='space-y-3'>
        <div
          className={cn(
            'text-3xl font-semibold tabular-nums',
            profitTone(statusBar.pnl24)
          )}
        >
          {formatMoney(statusBar.pnl24)}
        </div>
        {uncovered.length > 0 && (
          <div className='space-y-1.5'>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Not counted: {{amount}} of revenue from upstreams with no cost data',
                { amount: formatMoney(statusBar.pnl24_uncovered_sell) }
              )}
            </p>
            <Table>
              <TableBody>
                {uncovered.slice(0, 5).map((u) => (
                  <TableRow key={u.host}>
                    <TableCell className='py-1.5 text-xs'>{u.name}</TableCell>
                    <TableCell className='py-1.5 text-right text-xs tabular-nums'>
                      {formatMoney(u.sell24)}
                    </TableCell>
                    <TableCell className='text-muted-foreground max-w-[200px] truncate py-1.5 text-xs'>
                      {u.reason}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function WorkbenchBody({ summary }: { summary: WorkbenchSummary }) {
  const { t } = useTranslation()
  return (
    <div className='space-y-4'>
      <CriticalBanner alarms={summary.alarms} />
      {summary.status_bar.hub_db_error && (
        <Alert variant='destructive'>
          <AlertTitle>{t('Monitor data is incomplete')}</AlertTitle>
          <AlertDescription>{summary.status_bar.hub_db_error}</AlertDescription>
        </Alert>
      )}
      <NowMetrics statusBar={summary.status_bar} sites={summary.sites} />
      {/* 左宽右窄：左边是「要你处理的」，右边是「处理时要参考的前提」 */}
      <div className='grid gap-4 lg:grid-cols-5'>
        <div className='lg:col-span-3'>
          <AlarmsCard alarms={summary.alarms} />
        </div>
        <div className='lg:col-span-2'>
          <SitesCard sites={summary.sites} />
        </div>
      </div>
      <div className='grid gap-4 lg:grid-cols-5'>
        <div className='lg:col-span-3'>
          <ProfitCard statusBar={summary.status_bar} />
        </div>
        <div className='lg:col-span-2'>
          <WatermarkCard watermark={summary.watermark} />
        </div>
      </div>
      <TrendCard daily={summary.daily} />
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
  // 换票是一次性的，按钮加锁避免双击时两个窗口抢同一张票
  const [openingCaptures, setOpeningCaptures] = useState(false)
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
          <StatusBadge variant='neutral' className='shrink-0'>
            Root
          </StatusBadge>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          disabled={openingCaptures}
          onClick={async () => {
            if (openingCaptures) return
            setOpeningCaptures(true)
            // 先同步开窗再去换票：window.open 放在 await 之后会被弹窗拦截
            const opened = window.open('', '_blank')
            try {
              const url = await getChatDumpViewerUrl()
              if (!url) throw new Error('empty viewer url')
              if (opened) {
                opened.location.href = url
              } else {
                // 弹窗被拦就在当前标签打开，票只有一分钟有效，不能浪费
                window.location.href = url
              }
            } catch {
              opened?.close()
              toast.error(t('Failed to open, please try again'))
            } finally {
              setOpeningCaptures(false)
            }
          }}
        >
          <ExternalLink />
          {t('Conversation captures')}
        </Button>
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
