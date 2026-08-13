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
import { ArrowRight, Clock3, Database, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'

import {
  ageMinutes,
  formatBeijingDateTime,
  getAlarmDestination,
  getWorkbenchVerdict,
} from '../management'
import type { WorkbenchSummary } from '../types'
import { alarmDestinationLink } from './alarm-destination-link'

function freshnessLabel(
  minutes: number | null,
  t: ReturnType<typeof useTranslation>['t']
) {
  if (minutes == null) return t('No data')
  if (minutes < 1) return t('Just now')
  if (minutes === 1) return t('1 minute ago')
  if (minutes < 60) return t('{{count}} minutes ago', { count: minutes })
  const hours = Math.floor(minutes / 60)
  if (hours === 1) return t('1 hour ago')
  return t('{{count}} hours ago', { count: hours })
}

export function PartnerBrief({
  summary,
  onRetry,
  retrying,
}: {
  summary: WorkbenchSummary
  onRetry: () => void
  retrying: boolean
}) {
  const { t } = useTranslation()
  const verdict = getWorkbenchVerdict(summary)
  // 数据源本身取不到时 verdict 不给首要事项(见 management.ts),所以这张卡既没有
  // 「建议下一步」也没有去处按钮,唯一的动作是重试:一句陈述配一个动作,两者指的
  // 是同一件事,全页各出现一次。判据要和 getWorkbenchVerdict 一模一样,否则会出现
  // 标题说数据源没事、旁边却摆着重试按钮。
  const dataSourceError = Boolean(summary.status_bar.hub_db_error)
  const destination = verdict.primaryAlarm
    ? getAlarmDestination(verdict.primaryAlarm)
    : null

  const collectAge = ageMinutes(summary.status_bar.last_collect_ts, summary.now)
  const isCollectionStale = collectAge != null && collectAge > 90
  let badgeVariant: 'danger' | 'warning' | 'neutral' = 'neutral'
  if (verdict.tone === 'danger') badgeVariant = 'danger'
  else if (verdict.tone === 'warning') badgeVariant = 'warning'

  // 「客户最终吃到多少次失败」紧跟在简报后面:老板问的是「我的情况怎么样」,
  // 这个数是唯一能回答「客户到底疼不疼」的,越靠上越好;它自己就是一句结论,
  // 所以不去挤简报卡里「一句陈述 + 一个动作」的位置,而是接在下面自成一块。
  // 页面重排时它会挪进「供应和客户还稳吗」那半屏,这里不为它动布局。
  return (
    <>
      <Card
        size='sm'
        className={cn(
          'border-s-4',
          verdict.tone === 'danger' && 'border-s-destructive',
          verdict.tone === 'warning' && 'border-s-warning',
          verdict.tone === 'neutral' && 'border-s-primary'
        )}
      >
        <CardContent className='grid gap-2.5 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center'>
          <div className='min-w-0 space-y-2'>
            <div className='flex flex-wrap items-center gap-x-2 gap-y-1'>
              <StatusBadge variant={badgeVariant}>
                {t('Management brief')}
              </StatusBadge>
              <h2 className='text-base font-semibold'>
                {t(verdict.titleKey, { count: verdict.count })}
              </h2>
            </div>

            {verdict.primaryAlarm && (
              <div className='flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-sm'>
                <span className='text-muted-foreground text-xs font-medium'>
                  {t('Recommended next step')}:
                </span>
                <span className='font-medium'>
                  {verdict.primaryAlarm.title}
                </span>
                {verdict.primaryAlarm.detail && (
                  <span className='text-muted-foreground text-xs'>
                    {verdict.primaryAlarm.detail}
                  </span>
                )}
              </div>
            )}

            <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
              <span className='inline-flex items-center gap-1.5'>
                <Clock3 aria-hidden='true' />
                {t('Platform snapshot')}: {formatBeijingDateTime(summary.now)}{' '}
                {t('Beijing time')}
              </span>
              <span
                className={cn(
                  'inline-flex items-center gap-1.5',
                  isCollectionStale && 'text-status-warning'
                )}
              >
                <Database aria-hidden='true' />
                {t('Upstream collection')}:{' '}
                {formatBeijingDateTime(summary.status_bar.last_collect_ts)} ·{' '}
                {freshnessLabel(collectAge, t)}
              </span>
            </div>
          </div>

          {dataSourceError && (
            <Button
              type='button'
              size='sm'
              onClick={onRetry}
              disabled={retrying}
            >
              {/* 这是故障态唯一的出路,必须给回执:轮询一轮 60 秒,没有 pending
                  态的话点下去整页零反馈,人只会反复点。和页头刷新按钮同一套表现。 */}
              <RefreshCw
                data-icon='inline-start'
                className={cn(retrying && 'animate-spin')}
                aria-hidden='true'
              />
              {t('Retry')}
            </Button>
          )}

          {destination && (
            <Button size='sm' render={alarmDestinationLink(destination)}>
              {t(destination.labelKey)}
              <ArrowRight data-icon='inline-end' aria-hidden='true' />
            </Button>
          )}
        </CardContent>
      </Card>
      {/* 「客户吃到多少失败」这一格暂不挂载:它的判据(错误文案含「无可用渠道」)
          实测数的是**上游**回给我们的那句话 —— 上游自己也是同款网关,措辞一样 ——
          而我们重试换渠道大多救回来了。生产实测该口径比真实值低报 10~28 倍
          (真值按 request_id 算是 4,211 次/天,该口径给 100~470 次/天)。
          正确判据是「某个 request_id 只有失败行、没有成功行」,与文案/语言无关。
          改完再挂,组件本身保留。详见 WORKBENCH_SPEC.md §3.5。 */}
    </>
  )
}
