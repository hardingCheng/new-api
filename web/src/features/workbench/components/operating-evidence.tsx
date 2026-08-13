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
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toIntlLocale } from '@/i18n/languages'
import { formatCompactNumber, formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { UPSTREAM_BALANCES_ANCHOR } from '../management'
import type {
  WorkbenchDailyPoint,
  WorkbenchSite,
  WorkbenchStatusBar,
  WorkbenchSummary,
  WorkbenchWatermark,
} from '../types'
import { ProblemChannels, type WorkbenchChannel } from './problem-channels'

function formatMoney(value: number | null, locale: string | undefined): string {
  if (value == null) return '—'
  const amount = new Intl.NumberFormat(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(Math.abs(value))
  return value < 0 ? `-$${amount}` : `$${amount}`
}

/** 毛利是这一屏唯一一个「方向和大小同样重要」的金额:先得看出是赚还是亏,再看
 *  多少。所以它永远带显式正负号,不靠颜色一个人扛。 */
function formatSignedMoney(
  value: number | null,
  locale: string | undefined
): string {
  if (value == null) return '—'
  const money = formatMoney(value, locale)
  return value < 0 ? money : `+${money}`
}

/** 覆盖率百分比一律向下取整。四舍五入会把 0.999 说成「100%」,而同一屏下方
 *  还并排列着未计入的上游 —— 一屏同时声称全覆盖和有营收没算进去。
 *  向下取整让「100%」只有真的一分不差时才可能出现。 */
function coveragePercent(coverage: number): number {
  return Math.floor(coverage * 100)
}

function profitTone(value: number | null): string {
  if (value == null) return ''
  return value >= 0 ? 'text-status-success' : 'text-destructive'
}

/** 能说出口的覆盖率,说不出口时返回 null,由调用方显示「算不出来」。
 *
 *  两种说不出口:一是取不到;二是覆盖率为 0 却又列不出一个未计入的上游 —— 那是
 *  自相矛盾的数据,一分营收都没算出成本,就总得说得出是哪些上游没算进去,更可能
 *  的情形是这 24 小时压根没有营收、0/0 被报成了 0。两种情形下这个 0 都不代表
 *  「覆盖率确实是零」,摆上屏就是拿假装算出来的数充当事实。
 *
 *  覆盖率在这一屏出现两次(顶部指标格的注脚、毛利质量卡),判断只能有一处,
 *  否则同一页会对同一件事说两种话。 */
function usableCoverage(statusBar: WorkbenchStatusBar): number | null {
  const coverage = statusBar.pnl24_coverage
  if (coverage == null) return null
  if (coverage === 0 && (statusBar.pnl24_uncovered_sites ?? []).length === 0) {
    return null
  }
  return coverage
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
    <Card size='sm' className='min-h-20'>
      <CardContent className='flex h-full flex-col'>
        <div className='text-muted-foreground text-xs leading-snug'>
          {label}
        </div>
        <div className={cn('mt-0.5 text-xl font-semibold tabular-nums', tone)}>
          {value}
        </div>
        {hint && (
          <div className='text-muted-foreground mt-auto pt-0.5 text-xs leading-snug'>
            {hint}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function OperatingMetrics({
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
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const soonest = sites
    .filter((site) => site.est_days != null)
    .sort((a, b) => (a.est_days ?? 0) - (b.est_days ?? 0))[0]
  const latestUtcDay = daily.at(-1) ?? null
  const coverage = usableCoverage(statusBar)
  const busyHourAverageRatio =
    watermark?.peak_rpm != null && watermark.peak_rpm_line > 0
      ? watermark.peak_rpm / watermark.peak_rpm_line
      : null

  return (
    <div className='grid grid-cols-2 gap-2 xl:grid-cols-4'>
      <MetricTile
        label={t('Estimated gross profit (last 24h)')}
        value={formatSignedMoney(statusBar.pnl24, locale)}
        tone={profitTone(statusBar.pnl24)}
        hint={
          coverage != null
            ? t('{{pct}}% of revenue covered', {
                pct: coveragePercent(coverage),
              })
            : t('Coverage unavailable')
        }
      />
      <MetricTile
        label={t('Successful billed requests today')}
        value={
          latestUtcDay == null
            ? '—'
            : formatNumber(latestUtcDay.requests, locale)
        }
        hint={t('UTC day boundary; current total')}
      />
      <MetricTile
        label={t('Busy-hour average RPM')}
        value={
          watermark?.peak_rpm == null
            ? '—'
            : formatNumber(watermark.peak_rpm, locale)
        }
        tone={
          busyHourAverageRatio != null && busyHourAverageRatio >= 0.8
            ? 'text-status-warning'
            : undefined
        }
        hint={
          watermark
            ? t('scale review line {{line}}; not a one-minute peak', {
                // 插值默认走 String(),不格式化的话同一格里会并排出现
                // 「12,000」和「15000」两种写法。
                line: formatNumber(watermark.peak_rpm_line, locale),
              })
            : undefined
        }
      />
      <MetricTile
        label={t('Soonest upstream to run dry')}
        value={
          soonest?.est_days != null
            ? t('{{d}} d', { d: soonest.est_days })
            : '—'
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

const UNCOVERED_ROWS_SHOWN = 5

/** 监控服务在「这个上游根本没配账号凭据」时填的原因,它已经是业务语言,照常显示。
 *  除它之外的原因都是采集过程抛出来的异常原文,里面会带上游的响应内容(实测出现过
 *  凭据片段),一个字都不能上屏。所以这里精确匹配这一句放行、其余全部归成固定的
 *  失败文案 —— 白名单比黑名单稳:以后多一种采集异常也不会漏到老板面前。监控服务
 *  改了这句话的话,这里会退成失败文案,退错的方向也不泄漏原文。 */
const NO_CREDENTIALS_REASON = '未配置上游账号凭据'

function ProfitQualityCard({ statusBar }: { statusBar: WorkbenchStatusBar }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const uncovered = statusBar.pnl24_uncovered_sites ?? []
  const coverage = usableCoverage(statusBar)
  // 这张列表监控服务已经先截过一刀,拿它的长度算「还有几个」会少报(真有 14 个
  // 未计入时会说「还有 5 个」)。所以用总数算;老监控服务不返回总数时就不报数字,
  // 因为这张卡的意义全在「哪些营收没算进毛利」,错的数字比不报更误导。
  const uncoveredTotal = statusBar.pnl24_uncovered_total
  const hiddenUncovered =
    uncoveredTotal == null ? 0 : uncoveredTotal - UNCOVERED_ROWS_SHOWN
  const hasUnlistedUpstreams =
    uncoveredTotal == null && uncovered.length > UNCOVERED_ROWS_SHOWN
  // 「没有未计入的上游」不等于全覆盖:数据源取不到营收时,覆盖率和未计入列表会
  // 同时是空的,那是算不出来,不是算出来全覆盖。绿徽章三个条件都要:覆盖率说得出
  // 口、不是 0、没有未计入的上游 —— 覆盖率 0 和「已全部覆盖」直接互相打脸。
  const fullyCovered =
    coverage != null && coverage > 0 && uncovered.length === 0
  const coverageUnknown = coverage == null && uncovered.length === 0

  return (
    <Card size='sm' className='h-full'>
      <CardHeader>
        <CardTitle>{t('Gross profit data quality')}</CardTitle>
        <CardDescription className='text-xs'>
          {t(
            'Know how much of the business this estimate can actually explain.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-2.5'>
        <div className='flex flex-wrap items-end gap-x-4 gap-y-2'>
          <div>
            <div className='text-muted-foreground text-xs'>{t('Coverage')}</div>
            <div
              className={cn(
                'mt-0.5 text-2xl font-semibold tabular-nums',
                coverage != null && coverage < 0.9 && 'text-status-warning'
              )}
            >
              {coverage == null ? '—' : `${coveragePercent(coverage)}%`}
            </div>
          </div>
          <div className='pb-0.5'>
            <div className='text-muted-foreground text-xs'>
              {t('Estimated gross profit (last 24h)')}
            </div>
            <div
              className={cn(
                'mt-0.5 text-base font-semibold tabular-nums',
                profitTone(statusBar.pnl24)
              )}
            >
              {formatSignedMoney(statusBar.pnl24, locale)}
            </div>
          </div>
        </div>

        {uncovered.length > 0 && (
          <div className='space-y-1.5 border-t pt-2'>
            <p className='text-muted-foreground text-xs'>
              {t(
                'Not counted: {{amount}} of revenue from upstreams with no cost data',
                {
                  amount: formatMoney(statusBar.pnl24_uncovered_sell, locale),
                }
              )}
            </p>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='h-8'>{t('Upstream')}</TableHead>
                  <TableHead className='h-8 text-right'>
                    {t('Revenue not counted')}
                  </TableHead>
                  <TableHead className='h-8'>{t('Reason')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody className='[&>tr]:h-9'>
                {uncovered.slice(0, UNCOVERED_ROWS_SHOWN).map((site) => (
                  <TableRow key={site.host}>
                    <TableCell className='py-1.5 text-xs'>
                      {site.name}
                    </TableCell>
                    <TableCell className='py-1.5 text-right text-xs tabular-nums'>
                      {formatMoney(site.sell24, locale)}
                    </TableCell>
                    <TableCell className='text-muted-foreground max-w-52 py-1.5 text-xs'>
                      {site.reason === NO_CREDENTIALS_REASON
                        ? t('No account credentials')
                        : t('Collection failed')}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {hiddenUncovered > 0 && (
              <p className='text-muted-foreground text-xs'>
                {t('{{count}} more upstreams not listed', {
                  count: hiddenUncovered,
                })}
              </p>
            )}
            {hasUnlistedUpstreams && (
              <p className='text-muted-foreground text-xs'>
                {t('More upstreams are not listed')}
              </p>
            )}
          </div>
        )}
        {fullyCovered && (
          <StatusBadge variant='success'>{t('Fully covered')}</StatusBadge>
        )}
        {coverageUnknown && (
          <p className='text-muted-foreground text-xs'>
            {t("Can't work out coverage right now")}
          </p>
        )}
      </CardContent>
    </Card>
  )
}

function TrendCard({
  daily,
  now,
}: {
  daily: WorkbenchDailyPoint[]
  now: number
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const max = Math.max(...daily.map((point) => point.requests), 1)
  const currentUtcDay = now - (now % 86400)
  // 分桶是按 UTC 日切的,所以轴标签也必须按 UTC 取月日,否则标签会和柱子错开
  // 一天;只有月日的先后顺序跟着读者的语言走。
  const dayLabel = new Intl.DateTimeFormat(locale, {
    timeZone: 'UTC',
    month: 'numeric',
    day: 'numeric',
  })

  return (
    <Card size='sm' className='h-full'>
      <CardHeader>
        <CardTitle>{t('Request trend (successful billed, UTC)')}</CardTitle>
        <CardDescription className='text-xs'>
          {t('The current UTC day is still in progress.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {daily.length === 0 ? (
          <p className='text-muted-foreground py-4 text-sm'>{t('No data')}</p>
        ) : (
          <div
            className='flex h-24 items-end gap-1.5'
            role='img'
            aria-label={t('Request trend (successful billed, UTC)')}
          >
            {daily.map((point) => {
              const inProgress = point.day_ts === currentUtcDay
              const valueLabel = formatNumber(point.requests, locale)
              return (
                <div
                  key={point.day_ts}
                  className='flex min-w-0 flex-1 flex-col items-center justify-end gap-1'
                  title={`${valueLabel}${inProgress ? ` · ${t('In progress')}` : ''}`}
                  aria-label={`${new Date(point.day_ts * 1000).toISOString().slice(0, 10)}: ${valueLabel}`}
                >
                  <span className='text-muted-foreground text-[10px] tabular-nums'>
                    {formatCompactNumber(point.requests, locale)}
                  </span>
                  <div
                    className={cn(
                      'w-full max-w-8 rounded-t-[var(--radius-sm)]',
                      inProgress ? 'bg-primary/45' : 'bg-primary/75'
                    )}
                    style={{
                      height: `${Math.max(
                        4,
                        Math.round((point.requests / max) * 54)
                      )}px`,
                    }}
                  />
                  <span className='text-muted-foreground text-[10px]'>
                    {dayLabel.format(point.day_ts * 1000)}
                  </span>
                </div>
              )
            })}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function SitesCard({ sites }: { sites: WorkbenchSite[] }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  // 锚点落点和报警按钮的 href 共用同一个常量:落点写死字面量的话,常量改名后
  // 按钮就静默滚不动了,而 typecheck 和用例都看不出来。
  return (
    <Card
      id={UPSTREAM_BALANCES_ANCHOR}
      size='sm'
      className='scroll-mt-16 gap-2'
    >
      <CardHeader>
        <CardTitle>{t('Upstream balances')}</CardTitle>
        <CardDescription className='text-xs'>
          {t('The riskiest and soonest-to-run-dry upstreams appear first.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {sites.length === 0 ? (
          <p className='text-muted-foreground py-4 text-sm'>
            {t('No upstream balances yet')}
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className='h-8'>{t('Upstream')}</TableHead>
                <TableHead className='h-8'>{t('Balance')}</TableHead>
                <TableHead className='h-8'>{t('Days remaining')}</TableHead>
                <TableHead className='h-8'>{t('Status')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody className='[&>tr]:h-10'>
              {sites.map((site) => (
                <TableRow key={site.host}>
                  <TableCell className='py-1.5'>
                    <span className='font-medium'>{site.name}</span>{' '}
                    <span className='text-muted-foreground text-xs'>
                      {site.host}
                    </span>
                  </TableCell>
                  <TableCell className='py-1.5 tabular-nums'>
                    {formatMoney(site.balance, locale)}
                  </TableCell>
                  <TableCell
                    className={cn(
                      'py-1.5 tabular-nums',
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
                  <TableCell className='py-1.5'>
                    <div className='flex flex-wrap items-center gap-2'>
                      {site.needs_topup && (
                        <StatusBadge variant='warning'>
                          {t('Top-up needed this month')}
                        </StatusBadge>
                      )}
                      {site.error && (
                        <span className='text-destructive text-xs'>
                          {t('Collection failed')}
                        </span>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}

/** 监控服务的渠道表。契约层 `types.ts` 不在本次改动范围内,所以这一段先声明在
 *  这里(字段形状在 `problem-channels.tsx`),等这一轮收口时整体挪进 `types.ts`。
 *  声明成可选:老版本监控服务不下发这一段,缺了「出问题的渠道」不渲染。 */
interface WorkbenchChannelsPayload {
  channels?: WorkbenchChannel[] | null
}

export function OperatingEvidence({
  summary,
}: {
  summary: WorkbenchSummary & WorkbenchChannelsPayload
}) {
  const { t } = useTranslation()

  // 采集异常的原文里带着上游的响应内容,余额表和未计入表都只显示结论(行本身就是
  // 上游名,老板知道是哪个站失败了)。原文留给排障的人看控制台 —— 和监控数据源
  // 报错的处理方式一致。未计入表的原因也来自这里的 error,所以只在这里记一次。
  const collectionErrors = summary.sites
    .filter((site) => site.error)
    .map((site) => `${site.host}: ${site.error}`)
    .join(' | ')
  useEffect(() => {
    if (collectionErrors) {
      console.warn('[workbench] upstream collection failed:', collectionErrors)
    }
  }, [collectionErrors])

  return (
    <section className='space-y-3' aria-labelledby='operating-snapshot-title'>
      <div className='flex flex-wrap items-baseline gap-x-2 gap-y-0.5'>
        <h2 id='operating-snapshot-title' className='text-base font-semibold'>
          {t('Operating snapshot')}
        </h2>
        <p className='text-muted-foreground text-xs'>
          {t('Evidence for decisions, not a to-do list.')}
        </p>
      </div>
      <OperatingMetrics
        statusBar={summary.status_bar}
        sites={summary.sites}
        daily={summary.daily}
        watermark={summary.watermark}
      />
      <div className='grid gap-3 lg:grid-cols-5'>
        <div className='lg:col-span-3'>
          <ProfitQualityCard statusBar={summary.status_bar} />
        </div>
        <div className='lg:col-span-2'>
          <TrendCard daily={summary.daily} now={summary.now} />
        </div>
      </div>
      <ProblemChannels
        channels={summary.channels ?? []}
        alarms={summary.alarms}
        dataSourceFailed={Boolean(summary.status_bar.hub_db_error)}
      />
      <SitesCard sites={summary.sites} />
    </section>
  )
}
