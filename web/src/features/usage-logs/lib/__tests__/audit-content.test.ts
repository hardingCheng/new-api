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
import { test } from 'node:test'

import { renderAuditContent } from '../format'

function translate(key: string, options?: Record<string, unknown>): string {
  const labels: Record<string, string> = {
    Disabled: 'Disabled',
    Enabled: 'Enabled',
    ID: 'ID',
    'User ID': 'User ID',
  }
  const template = labels[key] ?? key
  return template.replaceAll(/\{\{(\w+)\}\}/g, (_, name: string) =>
    String(options?.[name] ?? '')
  )
}

test('quota audit identifies the target user when structured target data exists', () => {
  const text = renderAuditContent(
    {
      op: {
        action: 'user.quota_add',
        params: {
          quota: '¥1000.000000 quota',
          target_user_id: 180,
          target_username: 'waule',
        },
      },
    },
    translate
  )

  assert.equal(
    text,
    'Increased user quota for waule (ID: 180) by ¥1000.000000 quota'
  )
})

test('quota audit falls back to target user ID for historical records', () => {
  const text = renderAuditContent(
    {
      op: {
        action: 'user.quota_add',
        params: { quota: '¥1000.000000 quota', target_user_id: 180 },
      },
    },
    translate
  )

  assert.equal(
    text,
    'Increased user quota for User ID: 180 by ¥1000.000000 quota'
  )
})

test('channel status audits render localized status labels instead of action keys', () => {
  assert.equal(
    renderAuditContent(
      {
        op: {
          action: 'channel.status_update',
          params: { id: 433, status: 2, changed: true },
        },
      },
      translate
    ),
    'Updated channel 433 status to Disabled'
  )
  assert.equal(
    renderAuditContent(
      {
        op: {
          action: 'channel.status_update_batch',
          params: { count: 3, total: 4, status: 1 },
        },
      },
      translate
    ),
    'Updated 3 of 4 channel statuses to Enabled'
  )
})
