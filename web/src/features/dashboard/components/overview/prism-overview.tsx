/*
巴西站 prism 版用量概览(Layer 3 平行变体,见 ~/ai/BR_PRISM_UI_SPEC.md)。
布局参照 prismix:一排 KPI → 双栏图表 → 每日明细表;空模块一律不渲染。
数据层全部复用上游(getUserQuotaDates / auth store / lib/format)。
*/
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowRight, KeyRound } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getUserQuotaDates } from '@/features/dashboard/api'
import type { QuotaDataItem } from '@/features/dashboard/types'
import { ROLE } from '@/lib/roles'
import { formatNumber, formatQuota } from '@/lib/format'
import { useAuthStore } from '@/stores/auth-store'

import { AnnouncementsPanel } from './announcements-panel'
import { PerformanceHealthPanel } from './performance-health-panel'

const DAY = 86400

function dayKey(ts: number) {
  const d = new Date(ts * 1000)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

interface DayRow {
  date: string
  count: number
  tokens: number
  quota: number
}

function KpiCard({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className='bg-card rounded-lg border p-4'>
      <div className='text-muted-foreground text-[11px] font-medium tracking-[0.08em] uppercase'>
        {label}
      </div>
      <div className='mt-2 font-mono text-2xl font-semibold tracking-tight'>{value}</div>
      {hint ? <div className='text-muted-foreground mt-1 text-xs'>{hint}</div> : null}
    </div>
  )
}

function PanelCard({
  title,
  subtitle,
  children,
}: {
  title: string
  subtitle?: string
  children: React.ReactNode
}) {
  return (
    <div className='bg-card rounded-lg border p-5'>
      <div className='text-sm font-semibold'>{title}</div>
      {subtitle ? <div className='text-muted-foreground mt-0.5 text-xs'>{subtitle}</div> : null}
      <div className='mt-4'>{children}</div>
    </div>
  )
}

export function OverviewDashboard() {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const isAdmin = Number(user?.role ?? 0) >= ROLE.ADMIN

  const now = Math.floor(Date.now() / 1000)
  const start7d = now - 7 * DAY

  const trendQuery = useQuery({
    queryKey: ['dashboard', 'prism-overview', start7d],
    queryFn: () =>
      getUserQuotaDates({
        start_timestamp: start7d,
        end_timestamp: now,
        default_time: 'day',
      }),
    staleTime: 60 * 1000,
  })
  const items: QuotaDataItem[] = trendQuery.data?.data ?? []

  const { days, byModel, total7d } = useMemo(() => {
    const dayMap = new Map<string, DayRow>()
    const modelMap = new Map<string, number>()
    let quotaSum = 0
    for (let ts = start7d; ts <= now; ts += DAY) {
      const k = dayKey(ts)
      dayMap.set(k, { date: k, count: 0, tokens: 0, quota: 0 })
    }
    for (const it of items) {
      const k = dayKey(it.created_at)
      const row = dayMap.get(k) ?? { date: k, count: 0, tokens: 0, quota: 0 }
      row.count += Number(it.count ?? 0)
      row.tokens += Number(it.token_used ?? 0)
      row.quota += Number(it.quota ?? 0)
      dayMap.set(k, row)
      quotaSum += Number(it.quota ?? 0)
      if (it.model_name) {
        modelMap.set(
          it.model_name,
          (modelMap.get(it.model_name) ?? 0) + Number(it.token_used ?? 0)
        )
      }
    }
    const days = [...dayMap.values()].sort((a, b) => a.date.localeCompare(b.date))
    const byModel = [...modelMap.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 8)
    return { days, byModel, total7d: quotaSum }
  }, [items, now, start7d])

  const remainQuota = Number(user?.quota ?? 0)
  const usedQuota = Number(user?.used_quota ?? 0)
  const requestCount = Number(user?.request_count ?? 0)
  const avgDaily = total7d / 7
  const runwayDays = avgDaily > 0 ? remainQuota / avgDaily : null

  const maxDayQuota = Math.max(...days.map((d) => d.quota), 1)
  const maxModelTokens = Math.max(...byModel.map(([, v]) => v), 1)
  const hasUsage = requestCount > 0 || total7d > 0

  let runwayDisplay: string
  if (runwayDays !== null) {
    runwayDisplay =
      runwayDays > 999 ? `999+ ${t('days')}` : `~${formatNumber(Math.floor(runwayDays))} ${t('days')}`
  } else if (remainQuota <= 0) {
    runwayDisplay = t('Balance depleted')
  } else {
    runwayDisplay = t('No recent usage')
  }

  return (
    <div className='flex flex-col gap-4'>
      {!hasUsage && (
        <div className='bg-card flex flex-wrap items-center justify-between gap-3 rounded-lg border px-4 py-3'>
          <div className='flex items-center gap-3'>
            <KeyRound className='text-muted-foreground size-4' aria-hidden='true' />
            <span className='text-sm'>{t('Create a key, copy the API address, and complete your first request in minutes.')}</span>
          </div>
          <Button size='sm' render={<Link to='/keys' />}>
            {t('Create API Key')}
            <ArrowRight data-icon='inline-end' />
          </Button>
        </div>
      )}

      <div className='grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-5'>
        <KpiCard label={t('Balance')} value={formatQuota(remainQuota)} hint={runwayDisplay} />
        <KpiCard label={t('Last 24h usage')} value={formatQuota(days.at(-1)?.quota ?? 0)} />
        <KpiCard label={t('Requests (last 7 days)')} value={formatNumber(days.reduce((s, d) => s + d.count, 0))} />
        <KpiCard label={t('Total Usage')} value={formatQuota(usedQuota)} />
        <KpiCard label={t('Total requests made')} value={formatNumber(requestCount)} />
      </div>

      <div className='grid gap-4 xl:grid-cols-2'>
        <PanelCard title={t('Call Trend')} subtitle={t('Requests (last 7 days)')}>
          <div className='flex h-40 items-end gap-2'>
            {days.map((d) => (
              <div key={d.date} className='group flex min-w-0 flex-1 flex-col items-center gap-1.5'>
                <div className='text-muted-foreground font-mono text-[10px] opacity-0 transition-opacity group-hover:opacity-100'>
                  {formatQuota(d.quota)}
                </div>
                <div
                  className='bg-primary/85 w-full rounded-sm transition-colors group-hover:bg-primary'
                  style={{ height: `${Math.max((d.quota / maxDayQuota) * 100, 2)}%` }}
                />
                <div className='text-muted-foreground font-mono text-[10px]'>{d.date.slice(5)}</div>
              </div>
            ))}
          </div>
        </PanelCard>

        <PanelCard title={t('Token Breakdown')} subtitle={t('Top models by traffic')}>
          {byModel.length === 0 ? (
            <div className='text-muted-foreground flex h-40 items-center justify-center text-sm'>
              {t('No recent usage')}
            </div>
          ) : (
            <div className='flex flex-col gap-2.5'>
              {byModel.map(([name, tokens]) => (
                <div key={name} className='flex items-center gap-3'>
                  <span className='w-44 truncate font-mono text-xs'>{name}</span>
                  <div className='bg-muted h-1.5 min-w-0 flex-1 overflow-hidden rounded-full'>
                    <div
                      className='bg-chart-1 h-full rounded-full'
                      style={{ width: `${(tokens / maxModelTokens) * 100}%` }}
                    />
                  </div>
                  <span className='text-muted-foreground w-16 text-right font-mono text-xs'>
                    {formatNumber(tokens)}
                  </span>
                </div>
              ))}
            </div>
          )}
        </PanelCard>
      </div>

      <PanelCard title={t('Historical Usage')} subtitle={t('Requests (last 7 days)')}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Time')}</TableHead>
              <TableHead className='text-right'>{t('Requests')}</TableHead>
              <TableHead className='text-right'>{t('Tokens')}</TableHead>
              <TableHead className='text-right'>{t('Cost')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {[...days].reverse().map((d) => (
              <TableRow key={d.date}>
                <TableCell className='font-mono text-xs'>{d.date}</TableCell>
                <TableCell className='text-right font-mono text-xs'>{formatNumber(d.count)}</TableCell>
                <TableCell className='text-right font-mono text-xs'>{formatNumber(d.tokens)}</TableCell>
                <TableCell className='text-right font-mono text-xs'>{formatQuota(d.quota)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </PanelCard>

      <AnnouncementsPanel />
      {isAdmin && <PerformanceHealthPanel />}
    </div>
  )
}
