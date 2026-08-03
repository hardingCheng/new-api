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

import type { ApiRequestConfig } from '@/lib/api'

import { TASK_ACTIONS, TASK_STATUS } from '../../../constants'
import type { TaskLog } from '../../../types'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLMediaElement',
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
const { resolveTaskVideoPreviewUrl } =
  await import('../../../lib/task-video-preview')
const { TaskVideoPreview } = await import('../video-preview-dialog')
const { api } = await import('@/lib/api')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Click to preview video': 'Click to preview video',
        'Video Preview': 'Video Preview',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

describe('task video preview', () => {
  after(() => {
    domWindow.close()
  })

  test('keeps successful video tasks previewable when fail_reason is empty', () => {
    const log = {
      task_id: 'task_public_1',
      action: TASK_ACTIONS.TEXT_GENERATE,
      status: TASK_STATUS.SUCCESS,
      fail_reason: '',
    } as TaskLog

    assert.equal(
      resolveTaskVideoPreviewUrl(log),
      '/v1/videos/task_public_1/content'
    )
  })

  test('opens a dialog and loads protected video content through the API client', async () => {
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    const originalGet = api.get
    let requestedUrl = ''
    let requestedConfig: ApiRequestConfig | undefined
    api.get = (async (url: string, config?: ApiRequestConfig) => {
      requestedUrl = url
      requestedConfig = config
      return { data: new Blob(['video'], { type: 'video/mp4' }) }
    }) as typeof api.get

    await act(async () => {
      root.render(
        <I18nextProvider i18n={i18n}>
          <TaskVideoPreview
            taskId='task_public_1'
            sourceUrl='/v1/videos/task_public_1/content'
            ownerUserId={42}
          />
        </I18nextProvider>
      )
    })

    const trigger = container.querySelector('button')
    assert.ok(trigger)

    await act(async () => {
      trigger.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await Promise.resolve()
    })

    const dialog = document.body.querySelector('[role="dialog"]')
    assert.ok(dialog)
    assert.equal(dialog.textContent?.includes('Video Preview'), true)
    assert.equal(requestedUrl, '/v1/videos/task_public_1/content')
    assert.equal(requestedConfig?.responseType, 'blob')
    assert.deepEqual(requestedConfig?.params, { user_id: 42 })
    assert.equal(
      dialog.querySelector('video')?.getAttribute('src')?.startsWith('blob:'),
      true
    )

    await act(async () => root.unmount())
    api.get = originalGet
    container.remove()
  })
})
