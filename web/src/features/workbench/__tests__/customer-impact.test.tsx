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

import type { WorkbenchHardFail } from '../types'

const domWindow = new Window({ url: 'http://localhost/workbench' })
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
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
const { CustomerImpact, FINAL_FAILURE_COUNT_SLOT } =
  await import('../components/customer-impact')

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

/** 生产形状:占比是万分之几的量级,分布里既有具名渠道,也有一条「还没落到渠道
 *  就被拒」(监控服务用编号 0 表达它,名字是空串)。 */
const impact: WorkbenchHardFail = {
  window_sec: 86_400,
  hard_fails: 320,
  requests: 742_000,
  errors: 12_800,
  rate: 0.00043,
  by_channel: [
    { channel_id: 14, name: 'video-upstream', count: 210 },
    { channel_id: 0, name: '', count: 98 },
    { channel_id: 21, name: 'code-upstream', count: 12 },
  ],
  by_group: [{ group: 'z', label: 'Z site customers', count: 300 }],
}

/** 窗口里一条客户吃到的失败都没有 —— 这是真的 0,不是算不出来。 */
const nothingFailed: WorkbenchHardFail = {
  ...impact,
  hard_fails: 0,
  rate: 0,
  by_channel: [],
  by_group: [],
}

/** 判据失效或分布被截断时监控服务报「算不出来」,并且明确不用 0 代替。 */
const cannotWorkOut: WorkbenchHardFail = {
  ...impact,
  hard_fails: null,
  rate: null,
  by_channel: [],
  by_group: [],
}

async function renderImpact(hardFail?: WorkbenchHardFail | null) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () =>
    root.render(
      <I18nextProvider i18n={i18n}>
        <CustomerImpact hardFail={hardFail} />
      </I18nextProvider>
    )
  )

  return {
    container,
    get text() {
      return container.textContent ?? ''
    },
    /** 主数字自己的落点。整段文字里数不出它 —— 标题里就带着时间窗的数字。 */
    get headline() {
      return container
        .querySelector(`[data-slot="${FINAL_FAILURE_COUNT_SLOT}"]`)
        ?.textContent?.trim()
    },
    breakdownToggle() {
      return container.querySelector<HTMLButtonElement>('button[aria-expanded]')
    },
    breakdownLabels() {
      return [...container.querySelectorAll('li')].map((row) =>
        row.firstElementChild?.textContent?.trim()
      )
    },
    async unmount() {
      await act(async () => root.unmount())
      container.remove()
    },
  }
}

describe('customer impact metric', () => {
  after(() => {
    domWindow.close()
  })

  // 次数当主数字、占比做注脚:0.043% 这个数自己说不出「一天几百次」,而老板问的
  // 是影响多深。占比同时钉住格式 —— 固定两位小数会把它显示成 0.00%,一个正数
  // 被说成零。
  test('leads with how many requests failed and keeps the share as a hint', async () => {
    const page = await renderImpact(impact)

    assert.equal(page.headline, '320')
    assert.match(page.text, /0\.043% of all requests/)

    await page.unmount()
  })

  // 反向护栏:真的一次都没有时照实报 0,别把上一条修过头、把 0 也吞成空态。
  test('reports a real zero as zero instead of as unavailable', async () => {
    const page = await renderImpact(nothingFailed)

    assert.equal(page.headline, '0')
    assert.doesNotMatch(page.text, /Can't work this out right now/)

    await page.unmount()
  })

  // 裁决口径 5:没有数据就说没有。算不出来时显示 0 等于把「不知道」说成「没事」,
  // 而这一格存在的理由正是回答客户到底疼不疼。
  test('says the number cannot be worked out instead of reporting zero', async () => {
    const page = await renderImpact(cannotWorkOut)

    assert.equal(page.headline, '—')
    assert.match(page.text, /Can't work this out right now/)
    assert.doesNotMatch(
      page.text,
      /of all requests/,
      'the card still states a share it cannot back up'
    )

    await page.unmount()
  })

  // 监控服务这一轮压根没查到这个数:页面另有全局故障态说明数据源的事,这里不
  // 摆一张空卡,更不摆一个占位的数。
  test('renders nothing when this round carries no customer impact data', async () => {
    const page = await renderImpact(null)

    assert.equal(page.text, '')
    assert.equal(page.container.querySelector('[data-slot="card"]'), null)

    await page.unmount()
  })

  // 一边说「算不出来」一边列出「失败落在哪」是同一屏两种说法。监控服务算不出总数
  // 时会把分布一并收空,但那是它的不变量,不是这里的。
  test('offers no breakdown while the number itself cannot be worked out', async () => {
    const page = await renderImpact({
      ...cannotWorkOut,
      by_channel: impact.by_channel,
      by_group: impact.by_group,
    })

    assert.equal(page.breakdownToggle(), null)
    assert.doesNotMatch(page.text, /video-upstream/)

    await page.unmount()
  })

  test('keeps the breakdown collapsed until it is asked for', async () => {
    const page = await renderImpact(impact)

    const toggle = page.breakdownToggle()
    assert.ok(toggle, 'the breakdown cannot be opened at all')
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    assert.doesNotMatch(page.text, /video-upstream/)

    await act(async () => toggle.click())

    assert.equal(toggle.getAttribute('aria-expanded'), 'true')
    assert.match(page.text, /video-upstream/)
    assert.match(page.text, /Z site customers/)

    await page.unmount()
  })

  // 裁决口径 6:内部编号不进 UI。监控服务用编号 0 表达「请求还没落到任何渠道就
  // 被拒」,名字是空串 —— 直接渲染就会摆出一条叫「渠道 #0」的假渠道。
  test('names requests rejected before any channel instead of numbering them', async () => {
    const page = await renderImpact(impact)

    const toggle = page.breakdownToggle()
    assert.ok(toggle)
    await act(async () => toggle.click())

    assert.deepEqual(page.breakdownLabels(), [
      'video-upstream',
      'Rejected before reaching any channel',
      'code-upstream',
      'Z site customers',
    ])

    await page.unmount()
  })

  // 分布长过列表时要说清楚还有多少没列出来,否则老板会把前几行当成全部。
  test('states how many rows are left out when the breakdown is longer than the list', async () => {
    const page = await renderImpact({
      ...impact,
      by_channel: Array.from({ length: 7 }, (_, index) => ({
        channel_id: index + 1,
        name: `upstream-${index + 1}`,
        count: 7 - index,
      })),
    })

    const toggle = page.breakdownToggle()
    assert.ok(toggle)
    await act(async () => toggle.click())

    // 报出「还有 2 个」的前提是真的只列了 5 个:两边不一致的话这行字反而更误导。
    assert.deepEqual(page.breakdownLabels(), [
      'upstream-1',
      'upstream-2',
      'upstream-3',
      'upstream-4',
      'upstream-5',
      'Z site customers',
    ])
    assert.match(page.text, /2 more not listed/)

    await page.unmount()
  })
})
