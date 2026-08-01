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
import { after, test } from 'node:test'

import { Window } from 'happy-dom'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'SVGElement',
  'Node',
  'Element',
  'Event',
  'MouseEvent',
  'CustomEvent',
  'MutationObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
  'matchMedia',
  'customElements',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}
document.write('<!doctype html><html><body></body></html>')
Object.defineProperty(document, 'compatMode', {
  configurable: true,
  value: 'CSS1Compat',
})

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { api } = await import('@/lib/api')
const { ChannelBreakerSection } = await import('../channel-breaker-section')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const defaultValues = {
  ChannelBreakerEnabled: true,
  ChannelBreakerFailureLimit: '5',
  ChannelBreakerCooldownSeconds: '60',
  ChannelBreakerProbeCount: '5',
  ChannelBreakerProbeSuccessCount: '3',
  ChannelBreakerExcludePaths: '',
  ChannelBreakerFailureStatusCodes: '429,500-599',
  ChannelBreakerRules: '[]',
  ChannelBreakerExemptChannels: '[]',
  AutomaticDisableKeywords: '',
  'monitor_setting.bark_alert_enabled': false,
  'monitor_setting.bark_alert_url': '',
  'monitor_setting.bark_alert_volume': 0,
  'monitor_setting.low_balance_alert_enabled': false,
  'monitor_setting.low_balance_threshold_cny': 0,
  'monitor_setting.low_balance_alert_sound': '',
  'monitor_setting.channel_breaker_alert_enabled': false,
  'monitor_setting.channel_breaker_alert_sound': '',
  'monitor_setting.channel_disable_alert_enabled': false,
  'monitor_setting.channel_disable_alert_sound': '',
  'monitor_setting.channel_disable_alert_cooldown_second': 300,
  'monitor_setting.retest_disabled_channel_enabled': false,
  'monitor_setting.retest_disabled_channel_seconds': 60,
} as never

const reason =
  '命中立即禁用规则「Disable on upstream balance exhaustion」(status_code=403, error_code=insufficient_user_quota)'

after(() => {
  domWindow.close()
})

test('breaker history shows why an immediate disable was triggered', async () => {
  const originalGet = api.get
  api.get = (async (url: string) => {
    if (url === '/api/option/channel_breaker/logs') {
      return {
        data: {
          success: true,
          data: {
            items: [
              {
                id: 1,
                created_at: 1_700_000_000,
                channel_id: 42,
                channel_name: 'Image upstream',
                model_name: 'gpt-image-2-pro',
                using_group: 'gpt image',
                rule_name: 'Disable on upstream balance exhaustion',
                failures: 0,
                cooldown_secs: 0,
                reason,
              },
            ],
            total: 1,
          },
        },
      }
    }
    if (url === '/api/option/channel_breaker/statuses') {
      return { data: { success: true, data: { items: [] } } }
    }
    if (url === '/api/channel/breaker_exempt') {
      return { data: { success: true, data: [] } }
    }
    return { data: { success: true, data: {} } }
  }) as typeof api.get

  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  try {
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <QueryClientProvider client={queryClient}>
            <ChannelBreakerSection defaultValues={defaultValues} />
          </QueryClientProvider>
        </I18nextProvider>
      )
      await Promise.resolve()
    })

    const reasonCell = container.querySelector(
      '[data-slot="channel-breaker-reason"]'
    )
    assert.ok(reasonCell)
    assert.equal(reasonCell.textContent, reason)
  } finally {
    api.get = originalGet
    await act(async () => root.unmount())
    queryClient.clear()
    container.remove()
  }
})
