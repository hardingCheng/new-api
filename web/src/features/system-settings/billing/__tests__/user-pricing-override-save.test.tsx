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
  'HTMLButtonElement',
  'HTMLInputElement',
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
const { UserPricingOverrideSection } =
  await import('../user-pricing-override-section')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function changeInputValue(input: HTMLInputElement, value: string) {
  const valueSetter = Object.getOwnPropertyDescriptor(
    domWindow.HTMLInputElement.prototype,
    'value'
  )?.set
  assert.ok(valueSetter)
  valueSetter.call(input, value)
  input.dispatchEvent(
    new domWindow.Event('input', { bubbles: true }) as unknown as Event
  )
}

after(() => {
  domWindow.close()
})

test('saving an edited user configuration persists the current pricing value', async () => {
  const originalGet = api.get
  const originalPut = api.put
  let savedValue = ''

  api.get = (async () => ({
    data: {
      success: true,
      data: {
        user: { id: 42, username: 'pricing-user', group: 'vip' },
        groups: [{ name: 'sd2', ratio: 1, models: [] }],
        models: [],
        model_prices: {},
      },
    },
  })) as typeof api.get
  api.put = (async (_url: string, request: { value?: string }) => {
    savedValue = request.value ?? ''
    return { data: { success: true, message: '' } }
  }) as typeof api.put

  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  try {
    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <QueryClientProvider client={queryClient}>
            <UserPricingOverrideSection
              defaultValue={JSON.stringify({
                rules: [
                  {
                    user_id: 42,
                    username: 'pricing-user',
                    user_group: 'vip',
                    group_pattern: 'sd2',
                    model_pattern: '',
                    type: 'ratio',
                    value: 1,
                    disabled: false,
                  },
                ],
              })}
            />
          </QueryClientProvider>
        </I18nextProvider>
      )
    })

    const editButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="Edit"]'
    )
    assert.ok(editButton)
    await act(async () => {
      editButton.click()
      await Promise.resolve()
    })

    const dialog = document.querySelector<HTMLElement>(
      '[data-slot="dialog-content"]'
    )
    assert.ok(dialog)
    const editRuleButton = [...dialog.querySelectorAll('button')].find(
      (button) => button.getAttribute('aria-label') === 'Edit'
    )
    assert.ok(editRuleButton)
    await act(async () => editRuleButton.click())

    const valueInput = dialog.querySelector<HTMLInputElement>(
      'input[type="number"]'
    )
    assert.ok(valueInput)
    await act(async () => changeInputValue(valueInput, '0.5'))

    const saveButton = [...dialog.querySelectorAll('button')].find(
      (button) => button.textContent === 'Save user configuration'
    )
    assert.ok(saveButton)
    await act(async () => {
      saveButton.click()
      await Promise.resolve()
    })

    const savedConfig = JSON.parse(savedValue) as {
      rules: Array<{ user_id: number; value: number }>
    }
    assert.equal(savedConfig.rules[0]?.user_id, 42)
    assert.equal(savedConfig.rules[0]?.value, 0.5)

    await act(async () => {
      editButton.click()
      await Promise.resolve()
    })
    const reopenedDialog = document.querySelector<HTMLElement>(
      '[data-slot="dialog-content"]'
    )
    assert.ok(reopenedDialog)
    const reopenedEditRuleButton = [
      ...reopenedDialog.querySelectorAll('button'),
    ].find((button) => button.getAttribute('aria-label') === 'Edit')
    assert.ok(reopenedEditRuleButton)
    await act(async () => reopenedEditRuleButton.click())
    assert.equal(
      reopenedDialog.querySelector<HTMLInputElement>('input[type="number"]')
        ?.value,
      '0.5'
    )
  } finally {
    api.get = originalGet
    api.put = originalPut
    await act(async () => root.unmount())
    queryClient.clear()
    container.remove()
    document
      .querySelectorAll('[data-slot="dialog-portal"]')
      .forEach((element) => element.remove())
  }
})
