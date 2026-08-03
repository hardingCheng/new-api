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
const { flexRender, getCoreRowModel, useReactTable } =
  await import('@tanstack/react-table')
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
  content: '',
  username: 'test-user',
  token_name: 'gemini-token',
  model_name: 'gemini-2.5-pro',
  quota: 1000,
  prompt_tokens: 100,
  completion_tokens: 50,
  use_time: 2,
  is_stream: false,
  channel: 1,
  channel_name: 'test-channel',
  token_id: 2,
  group: 'image-group',
  ip: '',
  other: JSON.stringify({ group: 'fallback-group', user_group_ratio: 3 }),
  request_id: 'request-id',
  upstream_request_id: '',
}

function CommonColumnsHarness(props: { isAdmin: boolean }) {
  const columns = useCommonLogsColumns(props.isAdmin)
  const table = useReactTable({
    columns,
    data: [log],
    getCoreRowModel: getCoreRowModel(),
  })
  const targetIds = new Set(['group', 'token_name'])
  const headers = table
    .getFlatHeaders()
    .filter((header) => targetIds.has(header.column.id))
  const cells = table
    .getRowModel()
    .rows[0].getVisibleCells()
    .filter((cell) => targetIds.has(cell.column.id))

  return (
    <div data-column-ids={table.getAllLeafColumns().map((column) => column.id)}>
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

async function renderColumns(isAdmin: boolean) {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <UsageLogsProvider>
          <CommonColumnsHarness isAdmin={isAdmin} />
        </UsageLogsProvider>
      </I18nextProvider>
    )
  })

  return { container, root }
}

after(() => {
  domWindow.close()
})

test('admin common logs render group and token in separate columns', async () => {
  const rendered = await renderColumns(true)

  assert.equal(
    rendered.container.querySelector('[data-header-id="group"]')?.textContent,
    'Group'
  )
  assert.equal(
    rendered.container.querySelector('[data-header-id="token_name"]')
      ?.textContent,
    'Token'
  )
  assert.equal(
    rendered.container.querySelector('[data-cell-id="group"]')?.textContent,
    'image-group3x'
  )
  assert.equal(
    rendered.container.querySelector('[data-cell-id="token_name"]')
      ?.textContent,
    'gemini-token'
  )

  await act(async () => rendered.root.unmount())
  rendered.container.remove()
})

test('user common logs omit the group column and keep the token column clean', async () => {
  const rendered = await renderColumns(false)

  assert.equal(
    rendered.container.querySelector('[data-header-id="group"]'),
    null
  )
  assert.equal(rendered.container.querySelector('[data-cell-id="group"]'), null)
  const tokenText =
    rendered.container.querySelector('[data-cell-id="token_name"]')
      ?.textContent ?? ''
  assert.equal(tokenText, 'gemini-token')
  assert.equal(tokenText.includes('image-group'), false)
  assert.equal(tokenText.includes('3x'), false)

  await act(async () => rendered.root.unmount())
  rendered.container.remove()
})
