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

import type { TFunction } from 'i18next'

import { getSidebarData } from '../use-sidebar-data'

test('admin sidebar exposes channel breaker next to other quick admin actions', () => {
  const identityT = ((key: string) => key) as TFunction
  const sidebar = getSidebarData(identityT)
  const adminGroup = sidebar.navGroups.find((group) => group.id === 'admin')
  assert.ok(adminGroup)

  const items = adminGroup.items
  const breakerIndex = items.findIndex(
    (item) => item.url === '/system-settings/operations/channel-breaker'
  )
  const subscriptionsIndex = items.findIndex(
    (item) => item.url === '/subscriptions'
  )
  const systemInfoIndex = items.findIndex((item) => item.url === '/system-info')

  assert.ok(breakerIndex > subscriptionsIndex)
  assert.ok(breakerIndex < systemInfoIndex)
})
