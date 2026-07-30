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

import type { TaskLog } from '../../../types'

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
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { flexRender, getCoreRowModel, useReactTable } =
  await import('@tanstack/react-table')
const { formatLogQuota } = await import('@/lib/format')
const { UsageLogsProvider } = await import('../../usage-logs-provider')
const { useTaskLogsColumns } = await import('../task-logs-columns')

const i18n = createInstance()
await i18n.use(initReactI18next).init({ lng: 'en' })

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

const taskLog: TaskLog = {
  id: 1,
  user_id: 7,
  username: 'admin-test-user',
  platform: '1',
  task_id: 'task_admin_metrics',
  action: 'textGenerate',
  channel_id: 42,
  channel_name: 'primary-video-channel',
  group: 'vip',
  model_name: 'public-video-model',
  quota: 125000,
  refund_quota: 25000,
  submit_time: 1_700_000_000,
  finish_time: 1_700_000_010,
  video_duration: 10,
  progress: '100%',
  fail_reason: 'upstream failed',
  status: 'FAILURE',
  data: { state: 'failed' },
  properties: {
    has_reference_video: true,
    reference_video_seconds: 4,
    video_seconds: 10,
    origin_model_name: 'public-video-model',
    upstream_model_name: 'secret-upstream-model',
  },
}

function TaskColumnsHarness(props: { isAdmin: boolean }) {
  const columns = useTaskLogsColumns(props.isAdmin)
  const table = useReactTable({
    columns,
    data: [taskLog],
    getCoreRowModel: getCoreRowModel(),
  })
  const row = table.getRowModel().rows[0]

  return (
    <div>
      {table.getFlatHeaders().map((header) => (
        <div key={header.id} data-header-id={header.column.id}>
          {header.isPlaceholder
            ? null
            : flexRender(header.column.columnDef.header, header.getContext())}
        </div>
      ))}
      {row.getVisibleCells().map((cell) => (
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
          <TaskColumnsHarness isAdmin={isAdmin} />
        </UsageLogsProvider>
      </I18nextProvider>
    )
  })

  return { container, root }
}

after(() => {
  domWindow.close()
})

test('admin task columns show channel and video billing metrics', async () => {
  const rendered = await renderColumns(true)

  assert.match(
    rendered.container.querySelector('[data-cell-id="channel"]')?.textContent ??
      '',
    /#42.*primary-video-channel/
  )
  assert.equal(
    rendered.container.querySelector('[data-header-id="model_name"]')
      ?.textContent,
    'Model'
  )
  assert.equal(
    rendered.container.querySelector('[data-cell-id="model_name"]')
      ?.textContent,
    'secret-upstream-modelpublic-video-model'
  )
  assert.equal(
    rendered.container.querySelector('[data-cell-id="video_duration"]')
      ?.textContent,
    '10s'
  )
  const billing = rendered.container.querySelector('[data-cell-id="billing"]')
  assert.match(billing?.textContent ?? '', /Fee/)
  assert.match(billing?.textContent ?? '', /Refund/)
  assert.equal(
    billing?.textContent?.includes(formatLogQuota(taskLog.quota ?? 0)),
    true
  )
  assert.equal(
    billing?.textContent?.includes(formatLogQuota(taskLog.refund_quota ?? 0)),
    true
  )
  assert.match(
    rendered.container.querySelector('[data-cell-id="reference_video"]')
      ?.textContent ?? '',
    /Yes.*4s/
  )

  await act(async () => rendered.root.unmount())
  rendered.container.remove()
})

test('non-admin task columns hide channel and billing metrics', async () => {
  const rendered = await renderColumns(false)

  for (const columnId of [
    'channel',
    'model_name',
    'video_duration',
    'billing',
    'reference_video',
  ]) {
    assert.equal(
      rendered.container.querySelector(`[data-cell-id="${columnId}"]`),
      null
    )
  }

  await act(async () => rendered.root.unmount())
  rendered.container.remove()
})

test('task id supports separate copy and details actions without the old subtitle', async () => {
  const rendered = await renderColumns(true)
  const taskIdCell = rendered.container.querySelector(
    '[data-cell-id="task_id"]'
  )
  assert.equal(taskIdCell?.textContent, taskLog.task_id)
  assert.equal(taskIdCell?.textContent?.includes('1'), false)
  assert.equal(taskIdCell?.textContent?.includes('Text to Video'), false)

  let copiedTaskId = ''
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: {
      writeText: async (text: string) => {
        copiedTaskId = text
      },
    },
  })
  const copyButton = taskIdCell?.querySelector<HTMLButtonElement>(
    `[aria-label="Copy to clipboard: ${taskLog.task_id}"]`
  )
  assert.ok(copyButton)
  await act(async () =>
    copyButton.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  )
  assert.equal(copiedTaskId, taskLog.task_id)
  assert.equal(document.querySelector('[role="dialog"]'), null)

  const trigger = taskIdCell?.querySelector<HTMLButtonElement>(
    `[aria-label="Task Details: ${taskLog.task_id}"]`
  )
  assert.ok(trigger)
  await act(async () =>
    trigger.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  )

  const dialog = document.querySelector('[role="dialog"]')
  assert.ok(dialog)
  const text = dialog.textContent ?? ''
  assert.match(text, /Task Details/)
  assert.match(text, /OpenAI/)
  assert.match(text, /primary-video-channel/)
  assert.match(text, /admin-test-user/)
  assert.match(text, /secret-upstream-model/)
  assert.match(text, /Billing/)
  assert.match(text, /Raw Data/)
  assert.equal(text.includes(formatLogQuota(taskLog.quota ?? 0)), true)
  assert.equal(text.includes(formatLogQuota(taskLog.refund_quota ?? 0)), true)
  assert.ok(dialog.querySelector('[aria-label="Copy to clipboard"]'))

  await act(async () => rendered.root.unmount())
  rendered.container.remove()
})

test('task details keep administrator fields out of the user dialog', async () => {
  const rendered = await renderColumns(false)
  const trigger = rendered.container.querySelector<HTMLButtonElement>(
    '[data-cell-id="task_id"] button'
  )
  assert.ok(trigger)

  await act(async () =>
    trigger.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  )

  const dialog = document.querySelector('[role="dialog"]')
  assert.ok(dialog)
  const text = dialog.textContent ?? ''
  assert.match(text, /public-video-model/)
  assert.match(text, /Video Duration10s/)
  assert.match(text, /Reference videoStatusYesDuration4s/)
  for (const adminOnlyValue of [
    'Internal ID',
    'primary-video-channel',
    'admin-test-user',
    'secret-upstream-model',
    'Billing',
    'Raw Data',
  ]) {
    assert.equal(text.includes(adminOnlyValue), false)
  }

  await act(async () => rendered.root.unmount())
  rendered.container.remove()
})
