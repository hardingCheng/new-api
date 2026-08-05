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

const domWindow = new Window()
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
  'CustomEvent',
  'MutationObserver',
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
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { SelectedChannelBadges } =
  await import('../user-channel-routing-section')
const { parseUserChannelRouting, serializeUserChannelRouting } =
  await import('../user-channel-routing-config')
const { UserModelRoutingSection } =
  await import('../user-model-routing-section')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Remove {{value}}': 'Remove {{value}}',
        Remove: 'Remove',
        'User Model Views': 'User Model Views',
        'User Channel Routing': 'User Channel Routing',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

describe('user channel routing configuration', () => {
  after(() => {
    domWindow.close()
  })

  test('parses channel IDs and serializes the selected failure policy', () => {
    const rules = parseUserChannelRouting(
      '{"rules":[{"id":"rule-a","name":"Rule A","user_id":7,"group_pattern":"sd2","channel_ids":["2",1,0],"fallback":"default"}]}'
    )

    assert.deepEqual(rules[0]?.channel_ids, [2, 1])
    assert.equal(rules[0]?.fallback, 'default')
    assert.deepEqual(JSON.parse(serializeUserChannelRouting(rules)), {
      rules,
    })
  })

  test('shows model-view and channel-routing tabs', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const queryClient = new QueryClient({
      defaultOptions: { mutations: { retry: false } },
    })

    await act(async () => {
      root.render(
        <QueryClientProvider client={queryClient}>
          <I18nextProvider i18n={i18n}>
            <UserModelRoutingSection
              userModelView='{"rules":[]}'
              userChannelRouting='{"rules":[]}'
            />
          </I18nextProvider>
        </QueryClientProvider>
      )
    })

    const tabs = [...container.querySelectorAll('[role="tab"]')].map(
      (tab) => tab.textContent
    )
    assert.deepEqual(tabs, ['User Model Views', 'User Channel Routing'])

    await act(async () => root.unmount())
    container.remove()
    queryClient.clear()
  })

  test('removes a selected historical channel from its badge', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const removed: number[] = []

    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <SelectedChannelBadges
            channelIDs={[12]}
            channelsByID={{ 12: null }}
            onRemove={(channelID) => removed.push(channelID)}
          />
        </I18nextProvider>
      )
    })

    const removeButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Remove #12"]'
    )
    assert.ok(removeButton)
    await act(async () => removeButton.click())
    assert.deepEqual(removed, [12])

    await act(async () => root.unmount())
    container.remove()
  })
})
