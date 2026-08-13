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
import { ChevronDown, ChevronUp, Lightbulb } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { CHANNEL_STATUS } from '@/features/channels/constants'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { AlarmDestination } from '../management'
import type { WorkbenchAlarm } from '../types'
import { alarmDestinationLink } from './alarm-destination-link'

/** 一档客户 × 这条渠道 = 一行算式。金额已经由监控服务写成整句人话
 *  (`formula`),这里只转述,不再自己拼数字 —— 这些钱是人民币口径,拿页面上
 *  那套美元格式化去套会把单位说错。 */
export interface WorkbenchChannelDetail {
  grp: string
  who: string
  pays: number | null
  you_pay: number | null
  margin: number | null
  profit: number | null
  /** 空串 = 这一档缺进货汇率,金额压根算不出来。此时 pays / you_pay / margin /
   *  profit 一律是 null,一个数字都不能显示,更不能拿 0 顶上(裁决口径 5)。 */
  formula: string
}

/** 监控服务下发的渠道表:每条渠道一行,hub 侧的实时信号(错误率 / 熔断)和监控侧
 *  的经营结论(赚亏 / 建议 / 算式)已经在后端 join 好。
 *
 *  字段先声明在唯一的消费方这一侧:契约层 `types.ts` 不在本次改动范围内,这一轮
 *  收口时整段挪过去即可(那之后这里只留 import)。
 *
 *  除 id / name / status 外全部按可选 + 可空处理:老版本监控服务不下发经营结论那
 *  几段,hub 库半途失败时熔断次数会是「不知道」而不是 0。缺了就不显示,不补默认值。 */
export interface WorkbenchChannel {
  id: number
  name: string
  /** 1 启用 / 2 老板自己关的 / 3 系统自动禁用。取值同渠道页,见 CHANNEL_STATUS。 */
  status: number
  econ_status?: 'loss' | 'thin' | 'ok' | 'unknown' | null
  /** 人话结论,如「客户每付 ¥1,你最少赚 ¥0.35」。监控服务已经翻译好,直接显示。 */
  econ_headline?: string | null
  /** 规则生成的建议,整句中文。 */
  econ_suggestions?: string[] | null
  econ_details?: WorkbenchChannelDetail[] | null
  site_name?: string | null
  err24?: number | null
  ok24?: number | null
  /** 24 小时错误率,0~1。 */
  err_rate?: number | null
  breaker_h1?: number | null
  breaker_h24?: number | null
  breaker_opens_24h?: number | null
  breaker_opens_7d?: number | null
}

/** 报警项里「这条报警说的是哪个实体」的渠道前缀,由监控服务约定。 */
const CHANNEL_REF_PREFIX = 'channel:'

/** 报警清单已经点名的渠道。
 *
 *  这一块不自己定「错误率多高算高、熔断几次算多」:那套判据在监控服务的报警里已经
 *  有一份(熔断、错误率、亏损、自动禁用各一条)。前端再拍一套的话,迟早出现「这里
 *  说这条渠道有问题、待办清单里却没有它」——同一件事两个说法,两处阈值各调一次。
 *  所以判据只有一处:报警点到谁,这里就列谁,并补上待办清单没有的那些东西
 *  (经营结论、建议、算式)。 */
function flaggedChannelIds(alarms: WorkbenchAlarm[]): Set<number> {
  const ids = new Set<number>()
  for (const alarm of alarms) {
    if (!alarm.ref?.startsWith(CHANNEL_REF_PREFIX)) continue
    const id = Number(alarm.ref.slice(CHANNEL_REF_PREFIX.length))
    // 认不出编号的报警直接跳过:宁可少列一条,也不要凭空生出一条对不上任何渠道
    // 的空行。
    if (Number.isInteger(id) && id > 0) ids.add(id)
  }
  return ids
}

/** 亏钱的排最前,再是被系统自动关掉的(它已经在停止赚钱了),然后才是其余。
 *  自动禁用的渠道往往没有熔断和错误率可比,不给它单独一档会沉到最后一行。 */
function problemRank(channel: WorkbenchChannel): number {
  if (channel.econ_status === 'loss') return 0
  if (channel.status === CHANNEL_STATUS.AUTO_DISABLED) return 1
  return 2
}

/** 真有问题的渠道,最该动手的排最前。
 *
 *  「有问题」的三种情形:系统自动关掉了它、报警已经点名了它、它在跑而且有客户群在
 *  亏。老板自己关掉的渠道不算问题(和状态条里禁用数的口径一致)——除非报警另外
 *  点了它的名。 */
function problemChannels(
  channels: WorkbenchChannel[],
  alarms: WorkbenchAlarm[]
): WorkbenchChannel[] {
  const flagged = flaggedChannelIds(alarms)

  return channels
    .filter((channel) => {
      if (channel.status === CHANNEL_STATUS.AUTO_DISABLED) return true
      if (flagged.has(channel.id)) return true
      return (
        channel.status === CHANNEL_STATUS.ENABLED &&
        channel.econ_status === 'loss'
      )
    })
    .sort((a, b) => {
      const rank = problemRank(a) - problemRank(b)
      if (rank !== 0) return rank
      // 24 小时窗口比 1 小时完整,而且 1 小时的次数本来就含在里面:同一条渠道
      // 1 小时炸 14 次,它的 24 小时次数一定 ≥ 14,自然排在只炸过 3 次的前面。
      const breaker = (b.breaker_h24 ?? 0) - (a.breaker_h24 ?? 0)
      if (breaker !== 0) return breaker
      const errors = (b.err_rate ?? 0) - (a.err_rate ?? 0)
      if (errors !== 0) return errors
      return a.id - b.id
    })
}

/** 结论的颜色只表达赚亏,不表达稳不稳:稳不稳由旁边的次数自己说话。 */
function headlineTone(status: WorkbenchChannel['econ_status']): string {
  if (status === 'loss') return 'text-destructive'
  if (status === 'thin') return 'text-status-warning'
  return 'text-muted-foreground'
}

/** 停用状态的徽章。启用中不给徽章 —— 常驻的「正常」徽章只会让人略过这一列。
 *  自动禁用是红的(系统替他关的,他还不知道),手动禁用是中性的(他自己关的)。 */
function StatusTag({ status }: { status: number }) {
  const { t } = useTranslation()

  if (status === CHANNEL_STATUS.AUTO_DISABLED) {
    return <StatusBadge variant='danger'>{t('Auto Disabled')}</StatusBadge>
  }
  if (status === CHANNEL_STATUS.ENABLED) return null
  return <StatusBadge variant='neutral'>{t('Disabled')}</StatusBadge>
}

function ProblemChannelRow({ channel }: { channel: WorkbenchChannel }) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [showSuggestions, setShowSuggestions] = useState(false)
  const [showDetails, setShowDetails] = useState(false)
  const suggestions = channel.econ_suggestions ?? []
  const details = channel.econ_details ?? []
  const failed = channel.err24 ?? 0
  const attempts = failed + (channel.ok24 ?? 0)
  const trips1h = channel.breaker_h1 ?? 0
  const trips24h = channel.breaker_h24 ?? 0
  // 错误率算不出来(或这 24 小时一次都没失败)就一个字都不说,不拿 0% 顶上。
  const tellsErrorRate = channel.err_rate != null && failed > 0
  // 1 小时的次数本来就含在 24 小时里,两个都说等于把同一件事说两遍;急的那个
  // (1 小时)优先。
  const tellsTrips1h = trips1h > 0
  const tellsTrips24h = trips1h === 0 && trips24h > 0
  // 深链的筛选值用渠道编号而不是渠道名:渠道名在库里没有唯一约束,为轮换 key 复制
  // 一份同名渠道是常规做法,按名字筛过去会同时列出两条,而这一行刻意不显示编号
  // (技术细节不进 UI),老板分不出被点名的是哪一条。渠道页的筛选认编号,按它筛
  // 过去落到的正好是这一条。
  const destination: AlarmDestination = {
    kind: 'channels',
    filter: String(channel.id),
    labelKey: 'Open channels',
  }

  return (
    <div className='border-t py-2 first:border-t-0 first:pt-0 last:pb-0'>
      <div className='grid gap-1.5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start'>
        <div className='min-w-0'>
          <div className='flex flex-wrap items-center gap-x-2 gap-y-1'>
            <span className='font-medium'>
              {channel.name || t('Unnamed channel')}
            </span>
            <StatusTag status={channel.status} />
            {channel.site_name && (
              <span className='text-muted-foreground text-xs'>
                {t('via {{site}}', { site: channel.site_name })}
              </span>
            )}
          </div>
          {channel.econ_headline && (
            <p
              className={cn(
                'mt-0.5 text-xs leading-snug',
                headlineTone(channel.econ_status)
              )}
            >
              {channel.econ_headline}
            </p>
          )}
          {(tellsErrorRate || tellsTrips1h || tellsTrips24h) && (
            <div className='text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs tabular-nums'>
              {tellsErrorRate && (
                <span>
                  {t('24h error rate {{pct}}% ({{failed}}/{{total}})', {
                    pct: Math.round((channel.err_rate ?? 0) * 100),
                    // 插值默认走 String(),不格式化的话同一行里会并排出现
                    // 「12,000」和「12000」两种写法。
                    failed: formatNumber(failed, locale),
                    total: formatNumber(attempts, locale),
                  })}
                </span>
              )}
              {tellsTrips1h && (
                <span>
                  {t('Tripped {{times}} times in the last hour', {
                    times: formatNumber(trips1h, locale),
                  })}
                </span>
              )}
              {tellsTrips24h && (
                <span>
                  {t('Tripped {{times}} times in 24h', {
                    times: formatNumber(trips24h, locale),
                  })}
                </span>
              )}
            </div>
          )}
        </div>
        <Button
          variant='outline'
          size='xs'
          className='justify-self-start sm:justify-self-end'
          render={alarmDestinationLink(destination)}
        >
          {t(destination.labelKey)}
        </Button>
      </div>

      {(suggestions.length > 0 || details.length > 0) && (
        <div className='mt-1 flex flex-wrap gap-x-1'>
          {suggestions.length > 0 && (
            <Button
              variant='ghost'
              size='xs'
              aria-expanded={showSuggestions}
              onClick={() => setShowSuggestions((value) => !value)}
            >
              {t('{{count}} suggestions', { count: suggestions.length })}
              {showSuggestions ? (
                <ChevronUp data-icon='inline-end' aria-hidden='true' />
              ) : (
                <ChevronDown data-icon='inline-end' aria-hidden='true' />
              )}
            </Button>
          )}
          {details.length > 0 && (
            <Button
              variant='ghost'
              size='xs'
              aria-expanded={showDetails}
              onClick={() => setShowDetails((value) => !value)}
            >
              {t('Show the formula')}
              {showDetails ? (
                <ChevronUp data-icon='inline-end' aria-hidden='true' />
              ) : (
                <ChevronDown data-icon='inline-end' aria-hidden='true' />
              )}
            </Button>
          )}
        </div>
      )}

      {showSuggestions && (
        <ul className='mt-1 space-y-1'>
          {suggestions.map((suggestion) => (
            <li key={suggestion} className='flex gap-1.5 text-xs leading-snug'>
              <Lightbulb className='mt-0.5 shrink-0' aria-hidden='true' />
              <span>{suggestion}</span>
            </li>
          ))}
        </ul>
      )}

      {showDetails && (
        <ul className='mt-1 space-y-1.5'>
          {details.map((detail) => (
            <li key={`${detail.grp}:${detail.who}`} className='text-xs'>
              <span className='font-medium'>{detail.grp}</span>{' '}
              <span className='text-muted-foreground'>{detail.who}</span>
              <p className='text-muted-foreground mt-0.5 leading-snug'>
                {detail.formula || t('No purchase rate for this tier yet')}
              </p>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** ③「供应和客户还稳吗」里的「出问题的渠道」。没有问题渠道时整块消失:常驻的
 *  「一切正常」会训练人略过这个位置,真出事那天照样略过。 */
export function ProblemChannels({
  channels,
  alarms,
  dataSourceFailed,
}: {
  channels: WorkbenchChannel[]
  alarms: WorkbenchAlarm[]
  dataSourceFailed: boolean
}) {
  const { t } = useTranslation()

  // 这一块的输入除经营结论以外全部来自平台库(渠道状态、错误率、熔断次数),库取不
  // 到的时候它们会以结构性的 0 到达前端 —— 「一次都没熔断」和「不知道熔断了几次」
  // 长得一模一样。半真的清单比没有清单更糟(老板会照着它下判断),而页面此时已经
  // 在最上面说了「先修数据源」,所以整块不渲染。
  if (dataSourceFailed) return null

  const problems = problemChannels(channels, alarms)
  if (problems.length === 0) return null

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle>{t('Channels with problems')}</CardTitle>
        <CardDescription className='text-xs'>
          {t('Only channels with a problem right now are listed here.')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {problems.map((channel) => (
          // key 用渠道编号:渠道名可以重、可以为空、还能被改,编号才是这一行说的
          // 那一条渠道。位置更不能进 key —— 前面一条恢复了会让后面每行整体挪位,
          // React 当成全新元素重建,正在点开的算式直接收起、焦点掉回 <body>。
          <ProblemChannelRow key={channel.id} channel={channel} />
        ))}
      </CardContent>
    </Card>
  )
}
