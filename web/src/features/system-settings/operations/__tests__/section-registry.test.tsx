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
import type { ReactElement } from 'react'

import { MonitoringSettingsSection } from '../../integrations/monitoring-settings-section'
import { ChannelBreakerSection } from '../channel-breaker-section'
import {
  getOperationsSectionContent,
  getOperationsSectionNavItems,
  OPERATIONS_SECTION_IDS,
} from '../section-registry'

const identityT = ((key: string) => key) as TFunction

test('channel breaker owns the former monitoring and alerts page', () => {
  assert.ok(OPERATIONS_SECTION_IDS.includes('channel-breaker'))
  assert.ok(!OPERATIONS_SECTION_IDS.includes('alerts' as never))

  const navItems = getOperationsSectionNavItems(identityT)
  assert.ok(
    !navItems.some(
      (item) =>
        item.url === '/system-settings/operations/channel-breaker' ||
        item.url === '/system-settings/operations/alerts'
    )
  )
})

test('channel breaker content includes monitoring settings with its own heading', () => {
  const content = getOperationsSectionContent(
    'channel-breaker',
    {} as never,
    undefined,
    undefined
  ) as ReactElement<{
    children: ReactElement<{ showTitle?: boolean }>[]
  }>
  const children = content.props.children

  assert.equal(children[0]?.type, ChannelBreakerSection)
  assert.equal(children[1]?.type, MonitoringSettingsSection)
  assert.equal(children[1]?.props.showTitle, true)
})
