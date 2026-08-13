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
import { ChevronDown, ChevronUp } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import type { WorkbenchHardFail } from '../types'

/** 分布每边最多列这么多行。这一格要回答的是「疼不疼、疼在哪」,不是渠道全表;
 *  剩下的只报个数,老板要逐条看的时候去渠道页。 */
const BREAKDOWN_ROWS_SHOWN = 5

/** 用例按这个标记取主数字。数字本身是这一格的全部意义:算不出来的时候它必须是
 *  破折号而不是 0,而「整段文字里没有 0」这种断言挡不住任何东西(标题里就有
 *  数字),所以给它一个稳定的落点。 */
export const FINAL_FAILURE_COUNT_SLOT = 'final-failure-count'

/** 时间窗跟着监控服务给的窗口长度走,不写死「24 小时」:这个数只在它自己的窗口
 *  里成立,窗口换了而标签没换,这行字就是假的。缺这个字段才退回 24 —— 老监控
 *  服务连整块都不下发,退回值只影响本来就不该渲染的情形。 */
function windowHours(windowSec: number | null | undefined): number {
  if (windowSec == null || windowSec <= 0) return 24
  return Math.round(windowSec / 3600)
}

/** 占比是 0.02% 这种量级,固定两位小数会把它显示成「0.00%」—— 一个正数被说成
 *  零。按有效数字格式化,再小也还留得住两位有效数字。 */
function formatShare(rate: number, locale: string | undefined): string {
  return new Intl.NumberFormat(locale, {
    style: 'percent',
    maximumSignificantDigits: 2,
  }).format(rate)
}

interface BreakdownRow {
  key: string
  label: string
  count: number
}

function BreakdownSection({
  title,
  rows,
}: {
  title: string
  rows: BreakdownRow[]
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const hidden = rows.length - BREAKDOWN_ROWS_SHOWN

  if (rows.length === 0) return null

  return (
    <div className='min-w-0'>
      <p className='text-muted-foreground text-xs font-medium'>{title}</p>
      <ul className='mt-1 space-y-0.5'>
        {rows.slice(0, BREAKDOWN_ROWS_SHOWN).map((row) => (
          <li
            key={row.key}
            className='flex items-baseline justify-between gap-3 text-xs'
          >
            <span className='min-w-0 truncate'>{row.label}</span>
            <span className='tabular-nums'>
              {formatNumber(row.count, locale)}
            </span>
          </li>
        ))}
      </ul>
      {hidden > 0 && (
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('{{count}} more not listed', { count: hidden })}
        </p>
      )}
    </div>
  )
}

/** 客户最终吃到的失败。
 *
 *  和渠道那侧的失败率不是一回事:那边的失败绝大多数被重试换渠道救回来了,说的是
 *  供给质量;这一格说的是客户实际拿到的答案。两者不能互相代替,所以标题和说明
 *  自己就得把「最终」说清楚,而不是靠脚注补救。
 *
 *  这里只端出客户吃到的次数,不端「上游一共失败了多少次」:同一屏并排摆着两个
 *  差两个数量级的失败数,老板迟早会拿错的那个当客户体验。
 *
 *  暂时挂在简报卡下面(见 partner-brief),页面重排时移进「供应和客户还稳吗」。 */
export function CustomerImpact({
  hardFail,
}: {
  hardFail?: WorkbenchHardFail | null
}) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [expanded, setExpanded] = useState(false)

  // 这一轮压根没查到这个数:页面已有全局故障态说明数据源的事,这里不占位、
  // 不摆一张空卡。
  if (!hardFail) return null

  const count = hardFail.hard_fails
  // null 是「算不出来」,0 是「一次都没有」。分开渲染 —— 把算不出来显示成 0
  // 等于拿一个没算出来的数假装天下太平,而这一格存在的理由正是回答疼不疼。
  const unavailable = count == null
  const share = hardFail.rate

  const channelRows: BreakdownRow[] = (hardFail.by_channel ?? []).map(
    (channel) => ({
      key: String(channel.channel_id),
      // 编号 0 是「请求还没落到任何渠道就被拒」,不是某个叫 0 的渠道,更不能
      // 把编号本身摆上屏。名字为空的其他情形只有一种解释:这条渠道已经不在
      // 列表里了(记日志之后被删的),那就照实说不知道是谁,不编一个名字。
      label:
        channel.channel_id === 0
          ? t('Rejected before reaching any channel')
          : channel.name || t('A channel that is no longer listed'),
      count: channel.count,
    })
  )
  const groupRows: BreakdownRow[] = (hardFail.by_group ?? []).map((group) => ({
    key: group.group,
    label: group.label,
    count: group.count,
  }))
  // 总数算不出来的时候分布也不成立(监控服务这时会把它一并收空)。万一还是来了
  // 几行,也不能一边说「算不出来」一边列出「失败落在哪」—— 同一屏两种说法。
  const hasBreakdown =
    !unavailable && (channelRows.length > 0 || groupRows.length > 0)

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle>
          {t('Requests that finally failed (last {{hours}}h)', {
            hours: windowHours(hardFail.window_sec),
          })}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t('What the customer got back after retries and channel switches.')}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-2'>
        <div className='flex flex-wrap items-baseline gap-x-3 gap-y-1'>
          <span
            data-slot={FINAL_FAILURE_COUNT_SLOT}
            className='text-2xl font-semibold tabular-nums'
          >
            {unavailable ? '—' : formatNumber(count, locale)}
          </span>
          {unavailable ? (
            <span className='text-muted-foreground text-xs'>
              {t("Can't work this out right now")}
            </span>
          ) : (
            share != null && (
              <span className='text-muted-foreground text-xs'>
                {t('{{pct}} of all requests', {
                  pct: formatShare(share, locale),
                })}
              </span>
            )
          )}
        </div>

        {hasBreakdown && (
          <div className='border-t pt-2'>
            <Button
              variant='ghost'
              size='xs'
              className='-ms-2'
              aria-expanded={expanded}
              onClick={() => setExpanded((value) => !value)}
            >
              {t('Where these failed')}
              {expanded ? (
                <ChevronUp data-icon='inline-end' aria-hidden='true' />
              ) : (
                <ChevronDown data-icon='inline-end' aria-hidden='true' />
              )}
            </Button>
            {expanded && (
              <div className='mt-1.5 grid gap-x-6 gap-y-2 sm:grid-cols-2'>
                <BreakdownSection title={t('By channel')} rows={channelRows} />
                <BreakdownSection
                  title={t('By customer group')}
                  rows={groupRows}
                />
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
