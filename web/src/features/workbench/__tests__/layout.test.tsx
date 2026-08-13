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

import type { WorkbenchSummary } from '../types'

// 时间格式化必须自己钉住北京时间,不能跟着开发机的时区走。进程时区设成 UTC 之后,
// 少写那个时区参数的实现在这里就会露出来 —— 在中国的开发机上本来看不出差别。
process.env.TZ = 'UTC'

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
const { WorkbenchBody } = await import('../index')

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

// 入口都是 <Link>,脱离 router context 会直接抛错。空的 route tree 就够用:
// Link 的 href 由 to / params 插值得出,不需要挂上真实页面。
const router = createRouter({
  routeTree: createRootRoute(),
  history: createMemoryHistory({ initialEntries: ['/workbench'] }),
})

const summary: WorkbenchSummary = {
  now: Date.UTC(2026, 7, 10, 0, 30) / 1000,
  status_bar: {
    pnl24: 28_913,
    pnl24_coverage: 0.76,
    pnl24_uncovered_sell: 10_476,
    pnl24_uncovered_sites: [
      {
        host: 'missing.example.com',
        name: 'Missing upstream',
        sell24: 10_476,
        // 监控服务对「这个上游根本没配凭据」填的原因,是它唯一一句业务语言。
        reason: '未配置上游账号凭据',
      },
    ],
    alarm_bad: 1,
    alarm_warn: 2,
    disabled_channels: 2,
    breaking_channels: 1,
    low_balance_sites: 1,
    last_collect_ts: Date.UTC(2026, 7, 10, 0, 0) / 1000,
    hub_db_error: null,
  },
  // 三条报警覆盖三种去处:调价 → 定价设置、熔断 → 渠道列表(都是站内路由),
  // 余额 → 同页锚点。最紧急的那条(调价)同时决定简报给出的入口。
  alarms: [
    {
      level: 'bad',
      kind: 'price_up',
      title: 'An upstream raised its price',
      detail: 'Up 20% since yesterday',
      link: '',
      occurred_at: Date.UTC(2026, 7, 10, 0, 20) / 1000,
    },
    {
      level: 'warn',
      kind: 'breaker',
      title: 'A channel keeps tripping its breaker',
      detail: 'Tripped 5 times in 24 hours',
      link: '/channels?filter=video-upstream',
      occurred_at: Date.UTC(2026, 7, 10, 0, 15) / 1000,
    },
    {
      level: 'warn',
      kind: 'balance',
      title: 'Upstream balance is low',
      detail: 'About two days remaining',
      link: '',
      occurred_at: Date.UTC(2026, 7, 10, 0, 5) / 1000,
    },
  ],
  watermark: {
    peak_hour_reqs: 60_000,
    peak_rpm: 1_000,
    peak_rpm_line: 1_500,
    logs_rows: 1_000_000,
    logs_rows_line: 50_000_000,
  },
  daily: [
    {
      day_ts: Date.UTC(2026, 7, 10) / 1000,
      requests: 12_000,
      quota: 1,
    },
  ],
  sites: [
    {
      host: 'low.example.com',
      name: 'Low balance upstream',
      balance: 2.5,
      est_days: 2,
      daily_burn: 1.25,
      needs_topup: true,
      error: null,
    },
  ],
}

/** 采集异常的原文来自上游的响应,实测出现过凭据片段。
 *
 *  断言认这个哨兵串,不认原文里的 `401`:同一份 fixture 里有 10,476 这个金额,
 *  格式化产物一旦出现 401(`$1,401.00` 之类)用例就会因为跟泄漏无关的原因变红。
 *  哨兵串撞不上任何格式化产物,它出现在页面上只有一个解释 —— 泄漏了。 */
const LEAK_SENTINEL = 'LEAKED-9f3a-token'
const RAW_COLLECTION_ERROR = `HTTP 401 Unauthorized token=sk-${LEAK_SENTINEL}`

const collectionFailure: WorkbenchSummary = {
  ...summary,
  status_bar: {
    ...summary.status_bar,
    pnl24_uncovered_sites: [
      {
        host: 'missing.example.com',
        name: 'Missing upstream',
        sell24: 10_476,
        reason: RAW_COLLECTION_ERROR,
      },
    ],
  },
  sites: summary.sites.map((site) => ({
    ...site,
    error: RAW_COLLECTION_ERROR,
  })),
}

const dataSourceFailure: WorkbenchSummary = {
  ...summary,
  status_bar: {
    ...summary.status_bar,
    hub_db_error: 'Error 1045: access denied for user',
  },
}

/** 取不到营收时,覆盖率和未计入列表会同时是空的 —— 那是算不出来,不是全覆盖。 */
const coverageUnknown: WorkbenchSummary = {
  ...summary,
  status_bar: {
    ...summary.status_bar,
    pnl24: null,
    pnl24_coverage: null,
    pnl24_uncovered_sell: null,
    pnl24_uncovered_sites: [],
  },
}

/** 覆盖率 100% 且一个未计入的上游都没有 —— 「已全覆盖」唯一说得出口的情形。 */
const everyUpstreamCovered: WorkbenchSummary = {
  ...summary,
  status_bar: {
    ...summary.status_bar,
    pnl24_coverage: 1,
    pnl24_uncovered_sell: null,
    pnl24_uncovered_sites: [],
  },
}

/** 覆盖率报成 0、却又列不出一个未计入的上游:自相矛盾。一分营收都没算出成本,就
 *  总得说得出是哪些上游没算进去;更可能是这 24 小时没有营收、0/0 被报成 0。 */
const coverageContradiction: WorkbenchSummary = {
  ...summary,
  status_bar: {
    ...summary.status_bar,
    pnl24_coverage: 0,
    pnl24_uncovered_sell: null,
    pnl24_uncovered_sites: [],
  },
}

/** 真实的 0% 覆盖:一分营收都没算出成本,而且说得出是哪个上游没算进去。 */
const zeroCoverageExplained: WorkbenchSummary = {
  ...summary,
  status_bar: { ...summary.status_bar, pnl24_coverage: 0 },
}

/** 按卡片自己的标题文案定位一张卡,不依赖 class 串或 DOM 层级。 */
function cardWith(container: HTMLElement, heading: string): HTMLElement {
  const card = [
    ...container.querySelectorAll<HTMLElement>('[data-slot="card"]'),
  ].find((candidate) => candidate.textContent?.includes(heading))
  assert.ok(card, `the "${heading}" card is no longer on the page`)
  return card
}

/** 整页出现次数。裁决口径 1 要数的是「同一件事说了几遍」,不是「有没有说」。 */
function occurrences(text: string, phrase: string): number {
  return text.split(phrase).length - 1
}

async function renderWorkbench(
  data: WorkbenchSummary = summary,
  onRetry: () => void = () => {},
  retrying = false
) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () =>
    root.render(
      <RouterContextProvider router={router}>
        <I18nextProvider i18n={i18n}>
          <WorkbenchBody
            summary={data}
            onRetry={onRetry}
            retrying={retrying}
            openingCaptures={false}
            onOpenCaptures={async () => {}}
          />
        </I18nextProvider>
      </RouterContextProvider>
    )
  )

  return {
    container,
    text: container.textContent ?? '',
    async unmount() {
      await act(async () => root.unmount())
      container.remove()
    },
  }
}

describe('workbench page body', () => {
  after(() => {
    domWindow.close()
  })

  test('management brief and action center appear before operating evidence', async () => {
    const page = await renderWorkbench()

    const brief = page.text.indexOf('Management brief')
    const actions = page.text.indexOf('Needs your attention')
    const evidence = page.text.indexOf('Operating snapshot')
    assert.ok(brief >= 0)
    assert.ok(actions > brief)
    assert.ok(evidence > actions)

    await page.unmount()
  })

  // 后台是单页应用:裸 <a href='/...'> 会重载整个应用(丢掉登录后的内存状态、
  // 白屏几秒)。所以每个站内入口都必须拦掉浏览器默认跳转、走路由。
  test('every in-app entry point navigates inside the app instead of reloading it', async () => {
    const page = await renderWorkbench()

    const links = [
      ...page.container.querySelectorAll<HTMLAnchorElement>('a[href^="/"]'),
    ]
    // 报警的去处如果全是同页锚点,这个循环就一条链接也扫不到,把简报和报警清单
    // 退回裸 <a> 也照样全绿。所以先确认「今天要不要动手」这两块自己产出了站内
    // 入口 —— 用它们兜底,而不是依赖「管理区域」那张卡还在:那张卡早晚会被拿掉,
    // 靠它兜底的话删卡当天这条会假红。也不要求两块各出一条 —— 最紧急那条报警的
    // 去处是路由还是同页锚点由数据决定,写成「各出一条」换个 fixture 就假红。
    const decisionBlocks = ['Management brief', 'Needs your attention'].map(
      (heading) => cardWith(page.container, heading)
    )
    assert.ok(
      decisionBlocks.some((card) => card.querySelector('a[href^="/"]')),
      'neither the brief nor the alarm list renders an in-app entry point, this case would pass vacuously'
    )

    for (const link of links) {
      const click = new MouseEvent('click', { bubbles: true, cancelable: true })
      await act(async () => {
        link.dispatchEvent(click)
      })
      assert.equal(
        click.defaultPrevented,
        true,
        `${link.getAttribute('href')} falls through to a full page load`
      )
    }

    await page.unmount()
  })

  // /_watch/ 是给排障用的内部监控页,不是老板每天要点的入口。
  test('keeps entry points inside the admin app and off the internal monitor page', async () => {
    const page = await renderWorkbench()

    for (const link of page.container.querySelectorAll('a[href]')) {
      const href = link.getAttribute('href') ?? ''
      assert.ok(
        href.startsWith('/') || href.startsWith('#'),
        `unexpected entry point outside the admin app: ${href}`
      )
      assert.equal(href.startsWith('/_watch/'), false)
    }

    await page.unmount()
  })

  // 裁决口径 2:需要脚注的指标本身就是错的指标。毛利的时间窗必须写在标签里 ——
  // 「今日」按 UTC 日界算,中国老板早 8 点看到的是归零后的数。
  test('labels gross profit with its 24-hour window instead of an ambiguous today', async () => {
    const page = await renderWorkbench()

    assert.match(page.text, /Estimated gross profit \(last 24h\)/)
    assert.doesNotMatch(page.text, /Today gross profit \(quota basis\)/)

    await page.unmount()
  })

  // 裁决口径 2:「余量」要拿什么减什么全靠猜,标签得直接说这个数是怎么聚合的。
  test('labels the RPM metric by the aggregation it reports instead of a headroom', async () => {
    const page = await renderWorkbench()

    assert.match(page.text, /Busy-hour average RPM/)
    assert.doesNotMatch(page.text, /Peak RPM headroom/)

    await page.unmount()
  })

  // 裁决口径 7:同一屏只用一个时区,全站统一北京时间。fixture 的快照时刻是
  // 2026-08-10 00:30 UTC,页面上必须是 08:30。
  test('renders the snapshot timestamp in Beijing time rather than in UTC', async () => {
    const page = await renderWorkbench()

    assert.match(page.text, /08\/10 08:30/)
    assert.doesNotMatch(page.text, /08\/10 00:30/)

    await page.unmount()
  })

  // 裁决口径 1:同一件事只说一次。数据源挂了的时候,「先修数据源」这句结论和唯一
  // 能解决它的动作(重试)都在简报卡里 —— 陈述一次、动作一次。此前这句结论在简报
  // 标题里、重试在另一条提示条里,同一件事在同一屏说了两遍。
  test('states the data source failure once and offers retry as the only action', async () => {
    const page = await renderWorkbench(dataSourceFailure)

    assert.equal(
      occurrences(
        page.text,
        'Monitoring data is incomplete. Fix the data source first.'
      ),
      1,
      'the data source failure is stated more than once'
    )

    const brief = cardWith(page.container, 'Management brief')
    assert.deepEqual(
      [...brief.querySelectorAll('a[href], button')].map((action) =>
        action.textContent?.trim()
      ),
      ['Retry'],
      'the brief offers something other than retry while the data source is down'
    )

    const retries = [
      ...page.container.querySelectorAll<HTMLButtonElement>('button'),
    ].filter((button) => button.textContent?.trim() === 'Retry')
    assert.equal(retries.length, 1, 'retry is offered more than once')

    await page.unmount()
  })

  // 上一条只证明按钮在,不证明它接线了。数据源挂了的时候重试是唯一出路,它是
  // 个摆设就等于这个状态没有出路。
  test('refetches the summary when the only action left is clicked', async () => {
    let retries = 0
    const page = await renderWorkbench(dataSourceFailure, () => {
      retries += 1
    })

    const brief = cardWith(page.container, 'Management brief')
    const retry = [...brief.querySelectorAll<HTMLButtonElement>('button')].find(
      (button) => button.textContent?.trim() === 'Retry'
    )
    assert.ok(retry, 'the brief offers no retry while the data source is down')
    await act(async () => retry.click())

    assert.equal(retries, 1, 'retry is wired to nothing')

    await page.unmount()
  })

  // 裁决口径 6:技术细节不进 UI。采集异常的原文是上游的响应内容,实测带过凭据片段。
  // 只查 textContent 挡不住属性:状态列换成 <span title={site.error}> 就能让原文
  // 回到 tooltip 和无障碍树里,而用例照样全绿。所以查整段 innerHTML,属性值也在
  // 里面。余额表状态列和未计入表原因列都从同一个 error 取值,两处都要查。
  test('states that collection failed without putting the upstream error in the markup', async () => {
    const page = await renderWorkbench(collectionFailure)

    const balances = cardWith(page.container, 'Days remaining')
    const quality = cardWith(page.container, 'Gross profit data quality')
    for (const card of [balances, quality]) {
      assert.equal(
        card.innerHTML.includes(LEAK_SENTINEL),
        false,
        'the raw upstream error reached the page, in text or in an attribute'
      )
      assert.match(card.textContent ?? '', /Collection failed/)
    }

    // 修过头一样是错的:哪个上游失败了必须还看得见,否则两张表只剩清一色
    // 「采集失败」,老板不知道该去看哪个上游。
    assert.match(balances.textContent ?? '', /Low balance upstream/)
    assert.match(quality.textContent ?? '', /Missing upstream/)

    await page.unmount()
  })

  // 「没配凭据」是监控服务唯一一句业务语言的原因,它得照常显示,否则未计入营收的
  // 那张表只剩清一色「采集失败」。措辞由监控服务决定,那边改了这里会先红。
  test('keeps the one business reason for uncovered revenue in business language', async () => {
    const page = await renderWorkbench()

    assert.match(page.text, /No account credentials/)

    await page.unmount()
  })

  // 裁决口径 5:没有数据就说没有,不用示例数据假装。
  test('says coverage is unknown instead of fully covered when coverage cannot be worked out', async () => {
    const page = await renderWorkbench(coverageUnknown)

    assert.doesNotMatch(page.text, /Fully covered/)
    assert.match(page.text, /Can't work out coverage right now/)

    await page.unmount()
  })

  // 上一条只钉了否定面。基准 fixture(覆盖率 76% + 1 个未计入上游)永远渲染不出
  // 这个徽章,所以此前把判据写成常量 false 也全绿 —— 等于没有护栏。这条钉正面。
  test('shows the fully covered badge when coverage is complete and nothing is left out', async () => {
    const page = await renderWorkbench(everyUpstreamCovered)

    assert.match(page.text, /Fully covered/)
    assert.doesNotMatch(page.text, /Can't work out coverage right now/)

    await page.unmount()
  })

  // 覆盖率 0 却列不出未计入的上游是自相矛盾的数据,那个 0 更可能是 0/0。既不能
  // 当成「覆盖率确实是零」摆上屏,更不能算成全覆盖。
  test('treats a zero coverage with nothing left out as unknown rather than fully covered', async () => {
    const page = await renderWorkbench(coverageContradiction)

    assert.doesNotMatch(page.text, /Fully covered/)
    assert.match(page.text, /Can't work out coverage right now/)
    assert.doesNotMatch(
      cardWith(page.container, 'Gross profit data quality').textContent ?? '',
      /\d%/,
      'the card still states a coverage percentage it cannot back up'
    )

    await page.unmount()
  })

  // 裁决口径 1:覆盖率在这一屏出现两次(顶部指标格的注脚、毛利质量卡)。判断只有
  // 一处才不会出现注脚说「0% of revenue covered」、卡片说「算不出覆盖率」并排。
  test('drops the coverage footnote as well when coverage cannot be stated', async () => {
    const page = await renderWorkbench(coverageContradiction)

    assert.doesNotMatch(page.text, /% of revenue covered/)
    assert.match(page.text, /Coverage unavailable/)

    await page.unmount()
  })

  // 反向护栏:真实的 0% 覆盖(说得出是哪个上游没算进去)必须照原样显示,别把上面
  // 两条修过头,把真的零覆盖也吞成「算不出来」。
  test('still reports a real zero coverage when it can name the upstream left out', async () => {
    const page = await renderWorkbench(zeroCoverageExplained)

    assert.match(page.text, /0% of revenue covered/)
    assert.match(page.text, /Missing upstream/)
    assert.doesNotMatch(page.text, /Can't work out coverage right now/)
    assert.doesNotMatch(page.text, /Fully covered/)

    await page.unmount()
  })

  // 余额报警的去处是同页锚点,落点在余额卡上。两处各写一遍字面量的话,常量改名
  // 后按钮就静默滚不动了 —— typecheck 和文本断言都看不出来。所以反查落点。
  test('lands every in-page anchor on an element that exists on the page', async () => {
    const page = await renderWorkbench()

    const anchors = [
      ...page.container.querySelectorAll<HTMLAnchorElement>('a[href^="#"]'),
    ]
    assert.ok(
      anchors.length > 0,
      'no alarm resolved to an in-page anchor, this case would pass vacuously'
    )
    for (const anchor of anchors) {
      const id = (anchor.getAttribute('href') ?? '').slice(1)
      assert.ok(
        page.container.querySelector(`[id="${id}"]`),
        `the anchor #${id} has no landing element on the page`
      )
    }

    await page.unmount()
  })
})
