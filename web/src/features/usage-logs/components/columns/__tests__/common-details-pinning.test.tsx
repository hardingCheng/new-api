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

import type { UsageLog } from '../../../data/schema'

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
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { getCoreRowModel, useReactTable } = await import('@tanstack/react-table')
const { DataTableView } = await import('@/components/data-table')
const { UsageLogsProvider } = await import('../../usage-logs-provider')
const { useCommonLogsColumns } = await import('../common-logs-columns')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const log: UsageLog = {
  id: 1,
  user_id: 7,
  created_at: 1_700_000_000,
  type: 2,
  content: 'Per-call billing details',
  username: '',
  token_name: '',
  model_name: '',
  quota: 0,
  prompt_tokens: 0,
  completion_tokens: 0,
  use_time: 0,
  is_stream: false,
  channel: 0,
  channel_name: '',
  token_id: 0,
  group: '',
  ip: '',
  other: '',
  request_id: 'request-id',
  upstream_request_id: '',
}

function CommonDetailsTableHarness() {
  const columns = useCommonLogsColumns(false)
  const table = useReactTable({
    columns,
    data: [log],
    getCoreRowModel: getCoreRowModel(),
  })

  return <DataTableView table={table} />
}

after(() => {
  domWindow.close()
})

test('common log details header and cells stay pinned to the right edge', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <UsageLogsProvider>
          <CommonDetailsTableHarness />
        </UsageLogsProvider>
      </I18nextProvider>
    )
  })

  const header = container.querySelector('th[data-column-id="content"]')
  const cell = container.querySelector('td[data-column-id="content"]')
  assert.ok(header)
  assert.ok(cell)
  assert.equal(header.classList.contains('sticky'), true)
  assert.equal(header.classList.contains('right-0'), true)
  assert.equal(cell.classList.contains('sticky'), true)
  assert.equal(cell.classList.contains('right-0'), true)

  await act(async () => root.unmount())
  container.remove()
})
