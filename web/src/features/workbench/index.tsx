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
// 四格只放「下面那张清单里没有的东西」。原来放熔断数和禁用数，可下面的
// 清单已经逐条列了同样的事，等于同一件事说三遍——那才是「看着没变化」的
// 真正原因。这里换成经营面：赚了多少、跑了多少、还能跑多久。
function NowMetrics({
  statusBar,
  sites,
  daily,
  watermark,
}: {
  statusBar: WorkbenchStatusBar
  sites: WorkbenchSite[]
  daily: WorkbenchDailyPoint[]
  watermark: WorkbenchWatermark | null
}) {
  const { t } = useTranslation()
  const soonest = sites
    .filter((s) => s.est_days != null)
    .sort((a, b) => (a.est_days ?? 0) - (b.est_days ?? 0))[0]
  const coverage = statusBar.pnl24_coverage
  const today = daily.at(-1) ?? null
  const prev = daily.length > 1 ? (daily.at(-2) ?? null) : null
  const dod =
    today && prev && prev.requests > 0
      ? (today.requests - prev.requests) / prev.requests
      : null
  const rpmPct =
    watermark?.peak_rpm != null && watermark.peak_rpm_line > 0
      ? watermark.peak_rpm / watermark.peak_rpm_line
      : null
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
        label={t('Requests today')}
        value={formatCount(today?.requests ?? null)}
        hint={
          dod != null
            ? t('{{pct}}% vs yesterday', {
                pct: (dod >= 0 ? '+' : '') + Math.round(dod * 100),
              })
            : undefined
        }
      />
      <MetricTile
        label={t('Peak RPM headroom')}
        value={
          rpmPct != null ? `${Math.round(rpmPct * 100)}%` : formatCount(null)
        }
        tone={rpmPct != null && rpmPct >= 0.8 ? 'text-status-warning' : undefined}
        hint={
          watermark?.peak_rpm != null
            ? t('peak {{p}} / limit {{l}}', {
                p: watermark.peak_rpm,
                l: watermark.peak_rpm_line,
              })
            : undefined
        }
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

// 同类报警必须归并。原来 26 条里有 10 条都是「某渠道熔断了 N 次」、3 条
// 「采集失败」、3 条「渠道被禁用」——逐条平铺读起来是文字墙，真正有独立
// 含义的只有 8 件事左右。归并成一行结论 + 需要时展开明细。
type AlarmGroup = {
  kind: string
  level: 'bad' | 'warn'
  items: WorkbenchAlarm[]
}

function groupAlarms(alarms: WorkbenchAlarm[]): AlarmGroup[] {
  const byKind = new Map<string, WorkbenchAlarm[]>()
  for (const a of sortAlarms(alarms)) {
    const list = byKind.get(a.kind)
    if (list) list.push(a)
    else byKind.set(a.kind, [a])
  }
  const groups: AlarmGroup[] = []
  for (const [kind, items] of byKind) {
    groups.push({
      kind,
      level: items.some((i) => i.level === 'bad') ? 'bad' : 'warn',
      items,
    })
  }
  return groups.sort((a, b) => {
    if (a.level !== b.level) return a.level === 'bad' ? -1 : 1
    return (ALARM_KIND_ORDER[a.kind] ?? 99) - (ALARM_KIND_ORDER[b.kind] ?? 99)
  })
}

function AlarmGroupBlock({ group }: { group: AlarmGroup }) {
  const { t } = useTranslation()
  // 三条以内直接铺开——为看两条明细多点一次不值得；超过三条才折叠。
  const [expanded, setExpanded] = useState(group.items.length <= 3)
  const head = group.items[0]
  return (
    <div className='border-b py-2.5 last:border-b-0'>
      <div className='flex flex-wrap items-baseline gap-2'>
        <StatusBadge
          variant={group.level === 'bad' ? 'danger' : 'warning'}
          className='shrink-0'
        >
          {alarmKindLabel(group.kind, t)}
        </StatusBadge>
        {expanded ? (
          <span className='text-muted-foreground text-xs'>
            {t('{{n}} items', { n: group.items.length })}
          </span>
        ) : (
          <span className='min-w-0 font-medium'>{head.title}</span>
        )}
        {group.items.length > 3 && (
          <button
            type='button'
            onClick={() => setExpanded((v) => !v)}
            className='text-primary ms-auto shrink-0 text-xs hover:underline'
          >
            {expanded
              ? t('Collapse')
              : t('and {{n}} more', { n: group.items.length - 1 })}
          </button>
        )}
      </div>
      {expanded && (
        <div className='mt-1 space-y-1 ps-1'>
          {group.items.map((a) => (
            <div
              key={`${a.title}:${a.detail}`}
              className='flex flex-wrap items-baseline gap-2 text-sm'
            >
              <span className='min-w-0'>{a.title}</span>
              {a.detail && (
                <span className='text-muted-foreground text-xs'>
                  {a.detail}
                </span>
              )}
              {a.link && (
                <a
                  href={a.link}
                  className='text-primary ms-auto shrink-0 text-xs hover:underline'
                >
                  {t('Handle')} →
                </a>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

function AlarmsCard({ alarms }: { alarms: WorkbenchAlarm[] }) {
  const { t } = useTranslation()
  const groups = groupAlarms(alarms)
  return (
    <Card className='h-full'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          {t('Needs your attention')}
          {groups.length > 0 && (
            <span className='text-muted-foreground text-xs font-normal'>
              {t('{{n}} kinds', { n: groups.length })}
            </span>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {groups.length === 0 ? (
          <p className='text-muted-foreground py-4 text-sm'>
            {t('Nothing needs handling right now')}
          </p>
        ) : (
          groups.map((g) => <AlarmGroupBlock key={g.kind} group={g} />)
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
      <NowMetrics
        statusBar={summary.status_bar}
        sites={summary.sites}
        daily={summary.daily}
        watermark={summary.watermark}
      />
      {/* 左宽右窄：左边是「要你处理的」，右边是「处理时要参考的前提」 */}
      <div className='grid gap-4 lg:grid-cols-5'>
        <div className='lg:col-span-3'>
          <AlarmsCard alarms={summary.alarms} />
        </div>
        <div className='lg:col-span-2'>
          <SitesCard sites={summary.sites} />
        </div>
      </div>
      {/* 容量水位已经收进上面的「峰值余量」一格，这里不再重复一张卡 */}
      <div className='grid gap-4 lg:grid-cols-5'>
        <div className='lg:col-span-3'>
          <ProfitCard statusBar={summary.status_bar} />
        </div>
        <div className='lg:col-span-2'>
          <TrendCard daily={summary.daily} />
        </div>
      </div>
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
