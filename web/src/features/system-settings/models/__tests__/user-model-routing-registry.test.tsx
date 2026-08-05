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

import type { ReactElement } from 'react'

import { getModelsSectionContent } from '../section-registry'
import { UserModelRoutingSection } from '../user-model-routing-section'

test('user model routing section receives both independent settings', () => {
  const content = getModelsSectionContent('user-model-views', {
    UserModelView: '{"rules":[{"user_id":7}]}',
    UserChannelRouting: '{"rules":[{"id":"route-7"}]}',
  } as never) as ReactElement<{
    userModelView: string
    userChannelRouting: string
  }>

  assert.equal(content.type, UserModelRoutingSection)
  assert.equal(content.props.userModelView, '{"rules":[{"user_id":7}]}')
  assert.equal(content.props.userChannelRouting, '{"rules":[{"id":"route-7"}]}')
})
