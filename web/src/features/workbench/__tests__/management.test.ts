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
import assert from 'node:assert/strict'
import { test } from 'node:test'

import {
  ageMinutes,
  formatBeijingDateTime,
  getAlarmDestination,
  getWorkbenchVerdict,
  groupAlarms,
} from '../management'
import type { WorkbenchAlarm, WorkbenchSummary } from '../types'

// 时间格式化必须自己钉住北京时间,不能跟着开发机的时区走。进程时区设成 UTC 之后,
// 少写那个时区参数的实现在这里就会露出来 —— 在中国的开发机上本来看不出差别。
process.env.TZ = 'UTC'

function alarm(kind: string, level: 'bad' | 'warn', link = ''): WorkbenchAlarm {
  return {
    kind,
    level,
    link,
    title: `${kind} title`,
    detail: `${kind} detail`,
  }
}

function summary(alarms: WorkbenchAlarm[]): WorkbenchSummary {
  return {
    now: 1_786_334_400,
    alarms,
    daily: [],
    sites: [],
    watermark: null,
    status_bar: {
      pnl24: null,
      pnl24_coverage: null,
      pnl24_uncovered_sell: null,
      pnl24_uncovered_sites: null,
      alarm_bad: alarms.filter((item) => item.level === 'bad').length,
      alarm_warn: alarms.filter((item) => item.level === 'warn').length,
      disabled_channels: 0,
      breaking_channels: 0,
      low_balance_sites: 0,
      last_collect_ts: null,
      hub_db_error: null,
    },
  }
}

test('groups repeated alarms into management issues and keeps urgent groups first', () => {
  const groups = groupAlarms([
    alarm('breaker', 'warn'),
    alarm('balance', 'bad'),
    alarm('breaker', 'bad'),
    alarm('collect', 'warn'),
  ])

  assert.deepEqual(
    groups.map((group) => [group.kind, group.level, group.items.length]),
    [
      ['balance', 'bad', 1],
      ['breaker', 'bad', 2],
      ['collect', 'warn', 1],
    ]
  )
})

test('routes alarms without links to the area that can resolve them', () => {
  assert.deepEqual(getAlarmDestination(alarm('balance', 'bad')), {
    kind: 'upstream-balances',
    labelKey: 'Upstream balances',
  })
  assert.deepEqual(getAlarmDestination(alarm('collect', 'warn')), {
    kind: 'upstream-balances',
    labelKey: 'Upstream balances',
  })
  assert.deepEqual(getAlarmDestination(alarm('loss', 'bad')), {
    kind: 'channels',
    labelKey: 'Open channels',
  })
})

test('sends pricing alarms to pricing settings even when they report a channel link', () => {
  assert.deepEqual(getAlarmDestination(alarm('price_up', 'bad', '/channels')), {
    kind: 'group-pricing',
    labelKey: 'Open pricing settings',
  })
})

test('carries the reported channel name into the channels filter', () => {
  assert.deepEqual(
    getAlarmDestination(
      alarm('breaker', 'warn', '/channels?filter=hz%20%2B%20omni')
    ),
    {
      kind: 'channels',
      filter: 'hz + omni',
      labelKey: 'Open channels',
    }
  )
})

// 渠道名本身带 `?` 且没被转义时,link 里就有第二个 `?`。按第一个切,剩下的仍然
// 属于渠道名 —— 从第二个 `?` 处丢掉后半段会筛出一个不存在的渠道名。
test('keeps the part of a channel name that follows a second question mark', () => {
  assert.deepEqual(
    getAlarmDestination(
      alarm('breaker', 'warn', '/channels?filter=who?%20omni')
    ),
    {
      kind: 'channels',
      filter: 'who? omni',
      labelKey: 'Open channels',
    }
  )
})

test('opens the channels list unfiltered when the reported link carries no query', () => {
  assert.deepEqual(getAlarmDestination(alarm('breaker', 'warn', '/channels')), {
    kind: 'channels',
    filter: undefined,
    labelKey: 'Open channels',
  })
})

// 报警的 link 是监控服务下发的文本。认不出来的路径必须丢掉,而不是原样交给路由:
// 否则一次数据变更就能把管理员点到任意地址上去。
test('drops a reported link that does not point at a known area', () => {
  assert.equal(getAlarmDestination(alarm('breaker', 'warn', '/whatever')), null)
  assert.equal(
    getAlarmDestination(
      alarm('breaker', 'warn', 'https://example.com/channels')
    ),
    null
  )
})

test('gives no destination when an alarm kind without a fixed home reports no link', () => {
  assert.equal(getAlarmDestination(alarm('breaker', 'warn', '')), null)
})

test('management verdict counts issue types instead of duplicate alarm rows', () => {
  const verdict = getWorkbenchVerdict(
    summary([
      alarm('breaker', 'bad'),
      alarm('breaker', 'bad'),
      alarm('balance', 'bad'),
      alarm('collect', 'warn'),
    ])
  )

  assert.equal(verdict.tone, 'danger')
  assert.equal(verdict.count, 2)
  assert.equal(verdict.primaryAlarm?.kind, 'balance')
})

test('data errors override a reassuring operational verdict', () => {
  const data = summary([])
  data.status_bar.hub_db_error = 'query failed'

  const verdict = getWorkbenchVerdict(data)
  assert.equal(verdict.tone, 'danger')
  assert.equal(
    verdict.titleKey,
    'Monitoring data is incomplete. Fix the data source first.'
  )
})

// 数据源本身取不到时,报警清单是残缺的,排在最前面的那条不一定是眼下最该做的事 ——
// 唯一能推荐的动作是让数据先回来,所以这个状态没有首要事项。上面那条用的是空报警
// 列表,那时本来就没有首要事项,断言不到这个契约;这里必须有一条 bad 报警。
test('recommends no next step while the monitoring data source is down', () => {
  const data = summary([alarm('balance', 'bad'), alarm('breaker', 'warn')])
  data.status_bar.hub_db_error = 'query failed'

  assert.equal(getWorkbenchVerdict(data).primaryAlarm, null)
})

test('timestamps are shown in Beijing time and freshness never goes negative', () => {
  const ts = Date.UTC(2026, 7, 10, 0, 30) / 1000
  assert.match(formatBeijingDateTime(ts), /08\/10 08:30/)
  assert.equal(ageMinutes(ts, ts + 3_599), 59)
  assert.equal(ageMinutes(ts + 60, ts), 0)
})
