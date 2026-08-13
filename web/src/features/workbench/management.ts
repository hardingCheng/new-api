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
import type { WorkbenchAlarm, WorkbenchSummary } from './types'

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

export interface AlarmGroup {
  kind: string
  level: 'bad' | 'warn'
  items: WorkbenchAlarm[]
}

/** 报警的去处。用可辨识联合而不是 href 字符串:组件据此渲染类型安全的
 *  路由链接,不再用裸 <a> 触发整页刷新,也不用在组件里解析路径。 */
export type AlarmDestination =
  | { kind: 'channels'; filter?: string; labelKey: string }
  | { kind: 'group-pricing'; labelKey: string }
  | { kind: 'upstream-balances'; labelKey: string }

export const UPSTREAM_BALANCES_ANCHOR = 'upstream-balances'

/** 监控服务下发的 link 形如 `/channels?filter=<渠道名>`。只认白名单路径,
 *  未知路径一律丢弃 —— 不把上游文本直接喂给路由。
 *  只在第一个 `?` 处切,渠道名里再出现 `?` 也不会丢掉后半段。 */
function channelsDestinationFrom(link: string): AlarmDestination | null {
  if (!link) return null
  const mark = link.indexOf('?')
  const path = mark === -1 ? link : link.slice(0, mark)
  if (path !== '/channels') return null

  const query = mark === -1 ? '' : link.slice(mark + 1)
  const filter = query ? new URLSearchParams(query).get('filter') : null
  return {
    kind: 'channels',
    // 空 filter 等于不筛,别把 `?filter=` 原样透给路由
    filter: filter || undefined,
    labelKey: 'Open channels',
  }
}

export interface WorkbenchVerdict {
  tone: 'danger' | 'warning' | 'neutral'
  titleKey: string
  count: number
  primaryAlarm: WorkbenchAlarm | null
}

function sortAlarms(alarms: WorkbenchAlarm[]): WorkbenchAlarm[] {
  return [...alarms].sort((a, b) => {
    if (a.level !== b.level) return a.level === 'bad' ? -1 : 1
    return (ALARM_KIND_ORDER[a.kind] ?? 99) - (ALARM_KIND_ORDER[b.kind] ?? 99)
  })
}

export function groupAlarms(alarms: WorkbenchAlarm[]): AlarmGroup[] {
  const byKind = new Map<string, WorkbenchAlarm[]>()
  for (const alarm of sortAlarms(alarms)) {
    const items = byKind.get(alarm.kind)
    if (items) items.push(alarm)
    else byKind.set(alarm.kind, [alarm])
  }

  return [...byKind].map(([kind, items]) => ({
    kind,
    level: items.some((item) => item.level === 'bad') ? 'bad' : 'warn',
    items,
  }))
}

export function alarmKindLabelKey(kind: string): string {
  const labels: Record<string, string> = {
    balance: 'Balance',
    topup: 'Top-up',
    price_up: 'Price up',
    price_down: 'Price down',
    loss: 'Losing money',
    disabled: 'Disabled',
    breaker: 'Circuit breaker',
    error_rate: 'Error rate',
    collect: 'Collection',
    ratio: 'Ratio',
  }
  return labels[kind] ?? kind
}

const UPSTREAM_BALANCES_DESTINATION: AlarmDestination = {
  kind: 'upstream-balances',
  labelKey: 'Upstream balances',
}

export function getAlarmDestination(
  alarm: WorkbenchAlarm
): AlarmDestination | null {
  if (
    alarm.kind === 'price_up' ||
    alarm.kind === 'price_down' ||
    alarm.kind === 'ratio'
  ) {
    return { kind: 'group-pricing', labelKey: 'Open pricing settings' }
  }

  if (
    alarm.kind === 'balance' ||
    alarm.kind === 'topup' ||
    alarm.kind === 'collect'
  ) {
    return UPSTREAM_BALANCES_DESTINATION
  }

  const channels = channelsDestinationFrom(alarm.link)
  if (channels) return channels

  if (alarm.kind === 'loss') {
    return { kind: 'channels', labelKey: 'Open channels' }
  }

  return null
}

export function getWorkbenchVerdict(
  summary: WorkbenchSummary
): WorkbenchVerdict {
  const groups = groupAlarms(summary.alarms)
  const badGroups = groups.filter((group) => group.level === 'bad')

  // 数据源本身取不到时,报警清单是残缺的,排在最前面的那条不一定是眼下最该做的
  // 事 —— 唯一能推荐的动作是让数据先回来。所以这个状态没有首要事项,由简报卡把
  // 「重试」作为唯一入口给出。
  if (summary.status_bar.hub_db_error) {
    return {
      tone: 'danger',
      titleKey: 'Monitoring data is incomplete. Fix the data source first.',
      count: badGroups.length,
      primaryAlarm: null,
    }
  }

  if (badGroups.length > 0) {
    return {
      tone: 'danger',
      titleKey: '{{count}} types of issues need your decision',
      count: badGroups.length,
      primaryAlarm: sortAlarms(summary.alarms)[0] ?? null,
    }
  }

  if (groups.length > 0) {
    return {
      tone: 'warning',
      titleKey:
        'The current monitors found {{count}} types of risks to review.',
      count: groups.length,
      primaryAlarm: sortAlarms(summary.alarms)[0] ?? null,
    }
  }

  return {
    tone: 'neutral',
    titleKey: 'No urgent issues were found by the current monitors.',
    count: 0,
    primaryAlarm: null,
  }
}

export function formatBeijingDateTime(ts: number | null): string {
  if (!ts) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(ts * 1000))
}

export function ageMinutes(ts: number | null, now: number): number | null {
  if (!ts) return null
  return Math.max(0, Math.floor((now - ts) / 60))
}
