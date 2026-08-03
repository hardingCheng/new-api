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

import type { UsageLog } from '../../data/schema'

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
const { flexRender, getCoreRowModel, useReactTable } =
  await import('@tanstack/react-table')
const { useCommonLogsColumns } = await import('../columns/common-logs-columns')
const { UsageLogsMobileList } = await import('../usage-logs-mobile-card')
const { UsageLogsProvider } = await import('../usage-logs-provider')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const consumeLog: UsageLog = {
  id: 1,
  user_id: 7,
  created_at: 1_700_000_000,
  type: 2,
  content: '',
  username: '',
  token_name: '',
  model_name: 'gpt-4o-mini',
  quota: 5000,
  prompt_tokens: 120,
  completion_tokens: 30,
  use_time: 1.5,
  is_stream: false,
  channel: 1,
  channel_name: '',
  token_id: 2,
  group: '',
  ip: '',
  other: '',
  request_id: 'request-id',
  upstream_request_id: '',
}

function CommonLogTypeHarness(props: { mobile: boolean }) {
  const columns = useCommonLogsColumns(false)
  const table = useReactTable({
    columns,
    data: [consumeLog],
    getCoreRowModel: getCoreRowModel(),
  })

  if (props.mobile) {
    return <UsageLogsMobileList table={table} logCategory='common' />
  }

  const targetIds = new Set(['created_at', 'type'])
  const headers = table
    .getFlatHeaders()
    .filter((header) => targetIds.has(header.column.id))
  const cells = table
    .getRowModel()
    .rows[0].getVisibleCells()
    .filter((cell) => targetIds.has(cell.column.id))

  return (
    <div>
      {headers.map((header) => (
        <div key={header.id} data-header-id={header.column.id}>
          {flexRender(header.column.columnDef.header, header.getContext())}
        </div>
      ))}
      {cells.map((cell) => (
        <div key={cell.id} data-cell-id={cell.column.id}>
          {flexRender(cell.column.columnDef.cell, cell.getContext())}
        </div>
      ))}
    </div>
  )
}

async function renderHarness(mobile: boolean) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <UsageLogsProvider>
          <CommonLogTypeHarness mobile={mobile} />
        </UsageLogsProvider>
      </I18nextProvider>
    )
  })

  return { container, root }
}

after(() => {
  domWindow.close()
})

test('desktop common logs show type in a dedicated column', async () => {
  const rendered = await renderHarness(false)

  assert.equal(
    rendered.container.querySelector('[data-header-id="created_at"]')
      ?.textContent,
    'Time'
  )
  assert.equal(
    rendered.container.querySelector('[data-header-id="type"]')?.textContent,
    'Type'
  )
  assert.equal(
    rendered.container
      .querySelector('[data-cell-id="created_at"]')
      ?.textContent?.includes('Consume'),
    false
  )
  assert.equal(
    rendered.container.querySelector('[data-cell-id="type"]')?.textContent,
    'Consume'
  )

  await act(async () => rendered.root.unmount())
  rendered.container.remove()
})

test('mobile common logs keep type prominent and separate from time', async () => {
  const rendered = await renderHarness(true)

  assert.equal(
    rendered.container.querySelector('[data-slot="usage-log-mobile-type"]')
      ?.textContent,
    'Consume'
  )
  const timeField = rendered.container.querySelector(
    '[data-slot="usage-log-mobile-time"]'
  )
  assert.ok(timeField)
  assert.equal(timeField.textContent?.includes('Time'), true)
  assert.equal(timeField.textContent?.includes('Consume'), false)

  await act(async () => rendered.root.unmount())
  rendered.container.remove()
})
