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
import { after, describe, test } from 'node:test'

import { Window } from 'happy-dom'

import type { WorkbenchAlarm } from '../types'

const domWindow = new Window({ url: 'http://localhost/workbench' })
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLAnchorElement',
  'HTMLButtonElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'KeyboardEvent',
  'MouseEvent',
  'PointerEvent',
  'CustomEvent',
  'MutationObserver',
  'ResizeObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider } = await import('react-i18next')
const {
  RouterContextProvider,
  createMemoryHistory,
  createRootRoute,
  createRouter,
} = await import('@tanstack/react-router')
const { ActionCenter } = await import('../components/action-center')

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const i18n = createInstance()
await i18n.init({
  lng: 'en',
  fallbackLng: 'en',
  resources: { en: { translation: {} } },
  interpolation: { escapeValue: false },
})

// 报警的「打开渠道」按钮渲染成 <Link>,脱离 router context 会直接抛错。空的 route
// tree 就够用:Link 的 href 由 to / params 插值得出,不需要挂上真实页面。
const router = createRouter({
  routeTree: createRootRoute(),
  history: createMemoryHistory({ initialEntries: ['/workbench'] }),
})

function alarmGroup(size: number): WorkbenchAlarm[] {
  return Array.from({ length: size }, (_, index) => ({
    level: 'warn' as const,
    kind: 'breaker',
    title: `Upstream ${index + 1} keeps failing`,
    detail: 'Circuit breaker opened',
    link: `/channels?filter=upstream-${index + 1}`,
    occurred_at: Date.UTC(2026, 7, 10, 0, index) / 1000,
  }))
}

/** 文本逐字相同、只有实体标识不同的一组报警。真实来源:渠道名在库里没有唯一约束,
 *  两个同名渠道同时踩亏损线、最差毛利率又四舍五入到同一个数时,监控服务产出的
 *  title / detail / link(亏损类的 link 是空串)一模一样,只有 ref 能区分。 */
function sameWordingAlarms(refs: string[]): WorkbenchAlarm[] {
  return refs.map((ref) => ({
    level: 'bad' as const,
    kind: 'loss',
    title: 'An upstream has a customer group losing money',
    detail: 'Worst gross margin -12%',
    link: '',
    occurred_at: Date.UTC(2026, 7, 10, 0, 0) / 1000,
    ref,
  }))
}

/** 一行报警渲染一次它的标题,所以标题出现几次就是渲染了几行。文本相同的两条报警
 *  用 `visibleAlarmCount` 数不出来 —— 那个函数问的是「这条报警的标题在不在页面
 *  上」,两条共用一个标题时少渲染一行也照样算两条。 */
function renderedRowCount(container: HTMLElement, title: string): number {
  return (container.textContent ?? '').split(title).length - 1
}

/** 收下这一段渲染期间的 console.error。React 的重复 key 告警只走这条路,而这棵
 *  组件树(ActionCenter + Link + i18next)本来一条都不该写 —— 所以断言「一条都
 *  没有」比匹配 React 某个版本的告警原文稳:换了措辞照样拦得住。 */
async function consoleErrorsDuring(run: () => Promise<void>) {
  const original = console.error
  const messages: string[] = []
  console.error = (...args: unknown[]) => {
    messages.push(args.map(String).join(' '))
  }
  try {
    await run()
  } finally {
    console.error = original
  }
  return messages
}

function actionCenter(alarms: WorkbenchAlarm[]) {
  return (
    <RouterContextProvider router={router}>
      <I18nextProvider i18n={i18n}>
        <ActionCenter alarms={alarms} />
      </I18nextProvider>
    </RouterContextProvider>
  )
}

/** 可见条数按报警标题数,不按渲染出来的时间元素数:`occurred_at` 在类型上是可选的,
 *  真实数据里有报警不带时间戳,数 <time> 会把那几条当成不存在。 */
function visibleAlarmCount(
  container: HTMLElement,
  alarms: WorkbenchAlarm[]
): number {
  const text = container.textContent ?? ''
  return alarms.filter((alarm) => text.includes(alarm.title)).length
}

function expandToggle(container: HTMLElement): HTMLElement | null {
  return container.querySelector<HTMLElement>('button[aria-expanded]')
}

/** 这一组报警必须成立的不变量:要么全部可见,要么有展开按钮能看到剩下的。
 *  「有条目被折叠、却没有展开按钮」意味着那几条报警在刷新页面前再也看不到 ——
 *  折叠状态只在挂载时算一次就会这样,换实现也要挡住。 */
function assertEveryAlarmIsReachable(
  container: HTMLElement,
  alarms: WorkbenchAlarm[]
) {
  const hidden = alarms.filter(
    (alarm) => !container.textContent?.includes(alarm.title)
  )
  if (hidden.length === 0) return

  const toggle = expandToggle(container)
  const missing = hidden.map((alarm) => alarm.title).join(', ')
  assert.ok(toggle, `alarms hidden with no way to expand them: ${missing}`)
  assert.equal(toggle.getAttribute('aria-expanded'), 'false')
}

function mount() {
  const container = document.createElement('div')
  document.body.append(container)
  return { container, root: createRoot(container) }
}

describe('workbench alarm groups', () => {
  after(() => {
    domWindow.close()
  })

  test('keeps every alarm visible when polling shrinks a group below the collapse threshold', async () => {
    const { container, root } = mount()
    const polled = alarmGroup(5)
    const repolled = alarmGroup(2)

    await act(async () => root.render(actionCenter(polled)))
    assert.equal(visibleAlarmCount(container, polled), 3)
    assertEveryAlarmIsReachable(container, polled)

    await act(async () => root.render(actionCenter(repolled)))

    assert.equal(visibleAlarmCount(container, repolled), 2)
    assert.equal(expandToggle(container), null)
    assertEveryAlarmIsReachable(container, repolled)

    await act(async () => root.unmount())
    container.remove()
  })

  test('offers the expand toggle when polling grows a group past the collapse threshold', async () => {
    const { container, root } = mount()
    const polled = alarmGroup(2)
    const repolled = alarmGroup(5)

    await act(async () => root.render(actionCenter(polled)))
    assert.equal(expandToggle(container), null)

    await act(async () => root.render(actionCenter(repolled)))

    assert.equal(visibleAlarmCount(container, repolled), 3)
    assertEveryAlarmIsReachable(container, repolled)

    await act(async () => root.unmount())
    container.remove()
  })

  test('shows every alarm without a toggle when a group sits at the collapse threshold', async () => {
    const { container, root } = mount()
    const alarms = alarmGroup(3)

    await act(async () => root.render(actionCenter(alarms)))

    assert.equal(visibleAlarmCount(container, alarms), 3)
    assert.equal(expandToggle(container), null)
    assertEveryAlarmIsReachable(container, alarms)

    await act(async () => root.unmount())
    container.remove()
  })

  test('collapses the fourth alarm behind a toggle once a group exceeds the threshold', async () => {
    const { container, root } = mount()
    const alarms = alarmGroup(4)

    await act(async () => root.render(actionCenter(alarms)))

    assert.equal(visibleAlarmCount(container, alarms), 3)
    assert.equal(container.textContent?.includes(alarms[3].title), false)
    assertEveryAlarmIsReachable(container, alarms)

    await act(async () => root.unmount())
    container.remove()
  })

  // `occurred_at` 是可选的,余额 / 采集这类报警实际下发时就没有时间戳。折叠必须按
  // 报警条数算,而不是按页面上出现了几个时间。
  test('collapses a group by alarm count when only some alarms carry a timestamp', async () => {
    const { container, root } = mount()
    const alarms = alarmGroup(5).map((alarm, index) =>
      index % 2 === 0 ? { ...alarm, occurred_at: null } : alarm
    )

    await act(async () => root.render(actionCenter(alarms)))

    assert.equal(visibleAlarmCount(container, alarms), 3)
    assertEveryAlarmIsReachable(container, alarms)

    await act(async () => root.unmount())
    container.remove()
  })

  // 轮询会整体换掉 items。前面的报警一恢复,后面每条的位置都往前挪一格 —— 位置一旦
  // 进了 key,React 就把它们当成全新元素重建,老板正在点的那个入口焦点掉回 <body>。
  test('keeps the focused alarm entry point focused when polling drops an earlier alarm', async () => {
    const { container, root } = mount()
    const polled = alarmGroup(3)

    await act(async () => root.render(actionCenter(polled)))
    const focused =
      container.querySelectorAll<HTMLAnchorElement>('a[href]')[1] ?? null
    assert.ok(focused, 'the alarm rows render no entry point to focus')
    focused.focus()
    assert.equal(document.activeElement, focused)

    await act(async () => root.render(actionCenter(polled.slice(1))))

    assert.equal(
      document.activeElement,
      focused,
      'the focused entry point was rebuilt, so the click target moved away'
    )

    await act(async () => root.unmount())
    container.remove()
  })

  // 报警文本不足以区分两条,列表 key 必须用「这条报警说的是哪个实体」。撞了 key
  // 今天不伤用户(两行字一模一样),但 React 按 key 复用组件状态:等这一行有了
  // 行内展开或输入框,后一条就会吃前一条的状态,而且不会有任何东西报错。
  test('keeps two identically worded alarms apart by the entity each one points at', async () => {
    const { container, root } = mount()
    const alarms = sameWordingAlarms(['channel:31', 'channel:32'])
    // 前提:两条的文本真的逐字相同,否则文本兜底就够用,这个用例白写。
    assert.equal(alarms[0].title, alarms[1].title)
    assert.equal(alarms[0].detail, alarms[1].detail)
    assert.equal(alarms[0].link, alarms[1].link)

    const errors = await consoleErrorsDuring(async () => {
      await act(async () => root.render(actionCenter(alarms)))
    })

    assert.deepEqual(errors, [], 'React complained while rendering the alarms')
    assert.equal(renderedRowCount(container, alarms[0].title), 2)

    await act(async () => root.unmount())
    container.remove()
  })

  // 监控服务表达「没有值」惯用空串(报警的 link 就是),所以 ref 也可能是空串。
  // 空串当成「有值」的话,整组报警会共用同一个空 key —— 比退回文本兜底更糟。
  test('falls back to the alarm wording when the reported entity ref is empty', async () => {
    const { container, root } = mount()
    const alarms = sameWordingAlarms(['', '']).map((alarm, index) => ({
      ...alarm,
      title: `Upstream ${index + 1} has a customer group losing money`,
    }))

    const errors = await consoleErrorsDuring(async () => {
      await act(async () => root.render(actionCenter(alarms)))
    })

    assert.deepEqual(errors, [], 'React complained while rendering the alarms')
    assert.equal(visibleAlarmCount(container, alarms), 2)

    await act(async () => root.unmount())
    container.remove()
  })

  test('reveals and re-hides the collapsed alarms when the toggle is clicked', async () => {
    const { container, root } = mount()
    const alarms = alarmGroup(5)

    await act(async () => root.render(actionCenter(alarms)))
    const toggle = expandToggle(container)
    assert.ok(toggle)

    await act(async () => toggle.click())
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
    assert.equal(visibleAlarmCount(container, alarms), 5)
    assertEveryAlarmIsReachable(container, alarms)

    await act(async () => toggle.click())
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    assert.equal(visibleAlarmCount(container, alarms), 3)
    assertEveryAlarmIsReachable(container, alarms)

    await act(async () => root.unmount())
    container.remove()
  })
})
