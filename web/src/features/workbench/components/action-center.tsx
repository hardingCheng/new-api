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
import { ChevronDown, ChevronUp, Clock3 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import {
  alarmKindLabelKey,
  formatBeijingDateTime,
  getAlarmDestination,
  groupAlarms,
  type AlarmGroup,
} from '../management'
import type { WorkbenchAlarm } from '../types'
import { alarmDestinationLink } from './alarm-destination-link'

/** 超过这个条数才折叠;折叠时也正好留这么多条可见。 */
const COLLAPSED_VISIBLE_ITEMS = 3

function AlarmItem({ alarm }: { alarm: WorkbenchAlarm }) {
  const { t } = useTranslation()
  const destination = getAlarmDestination(alarm)

  return (
    <div className='grid gap-1.5 border-t py-2 first:border-t-0 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start'>
      <div className='min-w-0'>
        <p className='font-medium'>{alarm.title}</p>
        {alarm.detail && (
          <p className='text-muted-foreground mt-0.5 text-xs leading-snug'>
            {alarm.detail}
          </p>
        )}
        {alarm.occurred_at && (
          <time
            className='text-muted-foreground mt-1 inline-flex items-center gap-1 text-xs'
            dateTime={new Date(alarm.occurred_at * 1000).toISOString()}
          >
            <Clock3 aria-hidden='true' />
            {formatBeijingDateTime(alarm.occurred_at)} {t('Beijing time')}
          </time>
        )}
      </div>
      {destination && (
        <Button
          variant='outline'
          size='xs'
          className='justify-self-start sm:justify-self-end'
          render={alarmDestinationLink(destination)}
        >
          {t(destination.labelKey)}
        </Button>
      )}
    </div>
  )
}

function AlarmGroupSection({ group }: { group: AlarmGroup }) {
  const { t } = useTranslation()
  // 只存「用户手动展开过」,可见条数每次渲染按当前条数重新推导:轮询会换掉
  // group.items 而 key 仍是 group.kind(组件不重挂载),把展开态存成初始值
  // 会永久停在首次挂载时的判断上 —— 条数掉到阈值以下时展开按钮消失,
  // 剩下的报警在刷新页面前再也看不到。
  const [manuallyExpanded, setManuallyExpanded] = useState(false)
  const collapsible = group.items.length > COLLAPSED_VISIBLE_ITEMS
  const expanded = manuallyExpanded || !collapsible
  const visibleItems = expanded
    ? group.items
    : group.items.slice(0, COLLAPSED_VISIBLE_ITEMS)

  return (
    <section className='border-t py-2 first:border-t-0 first:pt-0 last:pb-0'>
      <div className='flex flex-wrap items-center gap-2'>
        <StatusBadge variant={group.level === 'bad' ? 'danger' : 'warning'}>
          {t(alarmKindLabelKey(group.kind))}
        </StatusBadge>
        <span className='text-muted-foreground text-xs'>
          {t('{{count}} items', { count: group.items.length })}
        </span>
        {collapsible && (
          <Button
            variant='ghost'
            size='xs'
            className='ms-auto'
            aria-expanded={expanded}
            onClick={() => setManuallyExpanded((value) => !value)}
          >
            {expanded ? t('Collapse') : t('View all')}
            {expanded ? (
              <ChevronUp data-icon='inline-end' aria-hidden='true' />
            ) : (
              <ChevronDown data-icon='inline-end' aria-hidden='true' />
            )}
          </Button>
        )}
      </div>
      <div className='mt-1'>
        {visibleItems.map((alarm) => (
          // key 优先用 ref(这条报警指向哪个实体),缺了才退回报警文本。空串也算
          // 缺 —— 监控服务表达「没有」惯用空串(link 就是),`??` 留住空串会让
          // 整组共用同一个 key,所以这里用 `||`。
          //
          // 文本兜底真的会撞:渠道名在库里没有唯一约束(见 types.ts 的 ref 字段),
          // 两个同名渠道同时踩亏损线、最差毛利率又落到同一个数时,title / detail /
          // link(亏损类是空串)逐字相同。采集类同理。撞了今天不伤用户(两行字
          // 一模一样),但 React 按 key 复用组件状态 —— 等 AlarmItem 有了行内展开
          // 或输入框,后一条就会吃前一条的状态,且不会有任何东西报错。
          //
          // 即便如此也严格好过把位置写进 key:轮询整体换掉 items,前面的报警一
          // 恢复,后面每条位置都往前挪一格,React 当成全新元素重建,正在点的那个
          // 按钮焦点直接掉回 <body>。撞 key 只波及同名的那几条,位置进 key 每次
          // 恢复都波及它后面的全部。occurred_at 同理不进 key:它每轮都在往前走,
          // 进了 key 等于每轮给每行换新 key,把焦点问题从偶发变成必然。
          <AlarmItem
            key={alarm.ref || `${alarm.title}:${alarm.detail}:${alarm.link}`}
            alarm={alarm}
          />
        ))}
      </div>
    </section>
  )
}

export function ActionCenter({ alarms }: { alarms: WorkbenchAlarm[] }) {
  const { t } = useTranslation()
  const groups = groupAlarms(alarms)

  return (
    <Card size='sm' className='h-full'>
      <CardHeader>
        <CardTitle className='flex flex-wrap items-center gap-2'>
          {t('Needs your attention')}
          {groups.length > 0 && (
            <span className='text-muted-foreground text-xs font-normal'>
              {t('{{count}} kinds', { count: groups.length })}
            </span>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {groups.length === 0 ? (
          <p className='text-muted-foreground py-3 text-sm'>
            {t('Nothing needs handling right now')}
          </p>
        ) : (
          groups.map((group) => (
            <AlarmGroupSection key={group.kind} group={group} />
          ))
        )}
      </CardContent>
    </Card>
  )
}
