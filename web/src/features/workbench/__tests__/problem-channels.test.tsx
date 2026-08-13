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

import type { WorkbenchChannel } from '../components/problem-channels'
import type { WorkbenchSummary } from '../types'

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
const { OperatingEvidence } = await import('../components/operating-evidence')

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

// 直达入口是 <Link>,脱离 router context 会直接抛错。空的 route tree 就够用:
// href 由 to / search 插值得出,不需要挂上真实页面。
const router = createRouter({
  routeTree: createRootRoute(),
  history: createMemoryHistory({ initialEntries: ['/workbench'] }),
})

/** 在跑、有客户群在亏。经营结论、建议、算式都取监控服务真实产出的措辞。 */
const losing: WorkbenchChannel = {
  id: 41,
  name: 'gptproo-main',
  status: 1,
  econ_status: 'loss',
  econ_headline: '有客户群在亏:最差每收 ¥1 倒贴 ¥0.12',
  econ_suggestions: [
    '「claudemax」卖给z站客户是亏的(收¥1.0 < 付¥1.2):把该分组卖价提上去,或让这条渠道退出该分组',
  ],
  econ_details: [
    {
      grp: 'claudemax',
      who: 'z站客户',
      pays: 1.0,
      you_pay: 1.2,
      margin: -0.2,
      profit: -0.2,
      formula:
        '客户付 ¥1.0(卖价×1 × 该站充值汇率) − 你付 ¥1.2(上游档位×1.2 × 进货1:1) = 亏 ¥0.2',
    },
    // 缺进货汇率的那一档:金额全是 null,算式是空串。
    {
      grp: 'claudemax-v5',
      who: '杭州站客户',
      pays: null,
      you_pay: null,
      margin: null,
      profit: null,
      formula: '',
    },
  ],
  site_name: 'happycode.vip',
  err24: 2,
  ok24: 998,
  err_rate: 0.002,
  breaker_h1: 0,
  breaker_h24: 0,
  breaker_opens_24h: 0,
  breaker_opens_7d: 0,
}

/** 赚得不错,但一直在熔断 —— 只有报警点了它的名才会进这张清单。 */
const tripping: WorkbenchChannel = {
  id: 14,
  name: 'zrocode-relay',
  status: 1,
  econ_status: 'ok',
  econ_headline: '客户每付 ¥1,你最少赚 ¥0.35',
  econ_suggestions: [],
  econ_details: [],
  site_name: 'origin.zrocode',
  err24: 680,
  ok24: 1320,
  err_rate: 0.34,
  breaker_h1: 14,
  breaker_h24: 21,
  breaker_opens_24h: 21,
  breaker_opens_7d: 2279,
}

/** 又稳又赚,报警也没点它 —— 不该出现在这张清单里。 */
const healthy: WorkbenchChannel = {
  id: 7,
  name: 'ccmax-partner',
  status: 1,
  econ_status: 'ok',
  econ_headline: '客户每付 ¥1,你最少赚 ¥0.42',
  econ_suggestions: [],
  econ_details: [],
  site_name: 'CCMAX-Partner',
  err24: 10,
  ok24: 990,
  err_rate: 0.01,
  breaker_h1: 0,
  breaker_h24: 0,
  breaker_opens_24h: 0,
  breaker_opens_7d: 0,
}

/** 老板自己关掉的,而且关之前在亏 —— 关掉本身就是处理结果,不是待办。 */
const ownerTurnedOff: WorkbenchChannel = {
  ...losing,
  id: 9,
  name: 'retired-relay',
  status: 2,
}

/** 被系统自动禁用,库里连名字都没填。 */
const autoDisabled: WorkbenchChannel = {
  id: 22,
  name: '',
  status: 3,
  econ_status: 'unknown',
  econ_headline: '缺进货汇率,先按额度口径看明细',
  econ_suggestions: [],
  econ_details: [],
  site_name: 'origin.zrocode',
  err24: 0,
  ok24: 0,
  err_rate: null,
  breaker_h1: 0,
  breaker_h24: 0,
  breaker_opens_24h: 0,
  breaker_opens_7d: 0,
}

const BREAKER_ALARM = {
  level: 'warn' as const,
  kind: 'breaker',
  title: 'A channel keeps tripping its breaker',
  detail: '',
  link: '/channels?filter=zrocode-relay',
  ref: 'channel:14',
}

type SummaryWithChannels = WorkbenchSummary & { channels: WorkbenchChannel[] }

function summaryWith(
  channels: WorkbenchChannel[],
  alarms: WorkbenchSummary['alarms'] = [],
  hubDbError: string | null = null
): SummaryWithChannels {
  return {
    now: Date.UTC(2026, 7, 11, 0, 30) / 1000,
    status_bar: {
      pnl24: 28_913,
      pnl24_coverage: 0.76,
      pnl24_uncovered_sell: null,
      pnl24_uncovered_sites: [],
      alarm_bad: 0,
      alarm_warn: alarms.length,
      disabled_channels: 0,
      breaking_channels: 0,
      low_balance_sites: 0,
      last_collect_ts: Date.UTC(2026, 7, 11, 0, 0) / 1000,
      hub_db_error: hubDbError,
    },
    alarms,
    watermark: null,
    daily: [],
    sites: [],
    channels,
  }
}

/** 按标题定位这一张卡,不依赖 class 串或 DOM 层级。找不到就是整块没渲染。 */
function problemCard(container: HTMLElement): HTMLElement | undefined {
  return [
    ...container.querySelectorAll<HTMLElement>('[data-slot="card"]'),
  ].find((card) => card.textContent?.includes('Channels with problems'))
}

function buttonWith(card: HTMLElement, label: string): HTMLButtonElement {
  const button = [...card.querySelectorAll<HTMLButtonElement>('button')].find(
    (candidate) => candidate.textContent?.includes(label)
  )
  assert.ok(button, `the "${label}" control is no longer on the card`)
  return button
}

async function render(summary: SummaryWithChannels) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () =>
    root.render(
      <RouterContextProvider router={router}>
        <I18nextProvider i18n={i18n}>
          <OperatingEvidence summary={summary} />
        </I18nextProvider>
      </RouterContextProvider>
    )
  )

  return {
    container,
    async unmount() {
      await act(async () => root.unmount())
      container.remove()
    },
  }
}

describe('channels with problems', () => {
  after(() => {
    domWindow.close()
  })

  test('states the plain-language verdict and links straight to that one channel', async () => {
    const page = await render(summaryWith([losing]))

    const card = problemCard(page.container)
    assert.ok(card, 'the problem channels block did not render')
    assert.match(card.textContent ?? '', /gptproo-main/)
    assert.match(card.textContent ?? '', /有客户群在亏/)

    // 深链按渠道编号筛选:同名渠道是常规做法(轮换 key 时复制一份),按名字筛
    // 会同时列出两条,而这一行刻意不显示编号,老板分不出被点名的是哪一条。
    const link = card.querySelector<HTMLAnchorElement>('a[href]')
    assert.ok(link, 'the row offers no way to get to the channel')
    const href = link.getAttribute('href') ?? ''
    assert.match(href, /^\/channels\?/)
    // 引号来自路由自己的查询串编码,筛选值本身是渠道编号。解码后比对,不把路由的
    // 编码格式钉进用例。
    assert.match(decodeURIComponent(href), /filter="?41"?(&|$)/)

    // 后台是单页应用:裸 <a href='/...'> 会重载整个应用。
    const click = new MouseEvent('click', { bubbles: true, cancelable: true })
    await act(async () => {
      link.dispatchEvent(click)
    })
    assert.equal(
      click.defaultPrevented,
      true,
      'the channel link falls through to a full page load'
    )

    await page.unmount()
  })

  // spec §2.2:「出问题的渠道」无内容时该子块消失。常驻的「一切正常」会训练人
  // 略过这个位置,真出事那天照样略过。
  test('renders nothing at all when every channel is fine', async () => {
    const page = await render(summaryWith([healthy]))

    assert.equal(problemCard(page.container), undefined)

    await page.unmount()
  })

  // D2:判据只有一套,在监控服务的报警里。前端再拍一套「错误率多高算高」的阈值,
  // 迟早出现「这里说这条渠道有问题、待办清单里却没有它」。这条钉住来源:经营结论
  // 是「赚」的渠道,只因为报警点了它的名才进清单。
  test('lists a channel the alarms flagged even though its margin is healthy', async () => {
    const page = await render(summaryWith([healthy, tripping], [BREAKER_ALARM]))

    const card = problemCard(page.container)
    assert.ok(card, 'the flagged channel did not make it onto the page')
    assert.match(card.textContent ?? '', /zrocode-relay/)
    assert.doesNotMatch(card.textContent ?? '', /ccmax-partner/)

    await page.unmount()
  })

  // 关键数字要跟着结论一起给,否则老板还得再点一次才知道有多严重。次数带千分位:
  // 生产上单条渠道 7 天熔断过 2,279 次。
  test('shows the error rate and the breaker count next to the verdict', async () => {
    const page = await render(summaryWith([tripping], [BREAKER_ALARM]))

    const card = problemCard(page.container)
    assert.ok(card)
    assert.match(card.textContent ?? '', /34% \(680\/2,000\)/)
    assert.match(card.textContent ?? '', /Tripped 14 times in the last hour/)

    await page.unmount()
  })

  test('puts the losing channel above the one tripping its breaker', async () => {
    const page = await render(summaryWith([tripping, losing], [BREAKER_ALARM]))

    const card = problemCard(page.container)
    assert.ok(card)
    const text = card.textContent ?? ''
    assert.ok(
      text.indexOf('gptproo-main') < text.indexOf('zrocode-relay'),
      'the channel that is losing money is not first'
    )

    await page.unmount()
  })

  // 状态条里的禁用数只数自动禁用,这里的口径必须一致:老板自己关掉的渠道不是
  // 待处理信号,否则同一件事一个数说 21、一个数说 3。
  test('leaves out a channel the owner switched off himself', async () => {
    const page = await render(summaryWith([ownerTurnedOff]))

    assert.equal(problemCard(page.container), undefined)

    await page.unmount()
  })

  test('keeps the suggestion out of the way until it is asked for', async () => {
    const page = await render(summaryWith([losing]))

    const card = problemCard(page.container)
    assert.ok(card)
    assert.doesNotMatch(card.textContent ?? '', /把该分组卖价提上去/)

    const toggle = buttonWith(card, 'suggestions')
    assert.equal(toggle.getAttribute('aria-expanded'), 'false')
    await act(async () => toggle.click())

    assert.match(card.textContent ?? '', /把该分组卖价提上去/)
    assert.equal(toggle.getAttribute('aria-expanded'), 'true')

    await page.unmount()
  })

  // 裁决口径 4:每个数字点开见公式。
  test('reveals the formula behind the verdict when it is expanded', async () => {
    const page = await render(summaryWith([losing]))

    const card = problemCard(page.container)
    assert.ok(card)
    assert.doesNotMatch(card.textContent ?? '', /客户付/)

    await act(async () => buttonWith(card, 'Show the formula').click())
    assert.match(card.textContent ?? '', /客户付 ¥1\.0/)
    assert.match(card.textContent ?? '', /= 亏 ¥0\.2/)

    await page.unmount()
  })

  // 裁决口径 5:算不出来就说算不出来。缺进货汇率的那一档金额全是 null,一个
  // 金额都不能出现 —— 拿 0 顶上等于告诉老板这一档不赚不亏。
  test('names the tier it cannot price without putting a number on it', async () => {
    const page = await render(summaryWith([losing]))

    const card = problemCard(page.container)
    assert.ok(card)
    await act(async () => buttonWith(card, 'Show the formula').click())

    const tier = [...card.querySelectorAll<HTMLElement>('li')].find((item) =>
      item.textContent?.includes('claudemax-v5')
    )
    assert.ok(tier, 'the tier without a purchase rate is not listed at all')
    assert.match(tier.textContent ?? '', /No purchase rate for this tier yet/)
    assert.equal(
      (tier.textContent ?? '').includes('¥'),
      false,
      'an amount is shown for a tier that has none'
    )

    await page.unmount()
  })

  // 平台库取不到时,渠道状态、错误率、熔断次数会以结构性的 0 到达前端 ——
  // 「一次都没熔断」和「不知道熔断了几次」长得一模一样。半真的清单比没有清单
  // 更糟:老板会照着它下判断。
  test('renders nothing while the data source is down', async () => {
    const page = await render(
      summaryWith([losing], [BREAKER_ALARM], 'Error 1045: access denied')
    )

    assert.equal(problemCard(page.container), undefined)

    await page.unmount()
  })

  // 裁决口径 5:算不出来就不说。这条渠道被关掉后一次请求都没有,「错误率 0%」和
  // 「熔断 0 次」会让人以为它现在很健康 —— 那两个 0 是结构性的,不是实测。
  test('puts no number on a channel that has had no traffic at all', async () => {
    const page = await render(summaryWith([autoDisabled]))

    const card = problemCard(page.container)
    assert.ok(card)
    assert.doesNotMatch(card.textContent ?? '', /error rate/)
    assert.doesNotMatch(card.textContent ?? '', /Tripped/)

    await page.unmount()
  })

  // 裁决口径 6:内部编号不进 UI。名字缺失时也得说得出这是一条渠道,而入口照样
  // 要落到它身上。
  test('names an unnamed channel without showing its internal number', async () => {
    const page = await render(summaryWith([autoDisabled]))

    const card = problemCard(page.container)
    assert.ok(card, 'an auto-disabled channel did not make it onto the page')
    assert.match(card.textContent ?? '', /Unnamed channel/)
    assert.doesNotMatch(card.textContent ?? '', /22/)
    assert.match(
      decodeURIComponent(
        card
          .querySelector<HTMLAnchorElement>('a[href]')
          ?.getAttribute('href') ?? ''
      ),
      /filter="?22"?(&|$)/
    )

    await page.unmount()
  })
})
