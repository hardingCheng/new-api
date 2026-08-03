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

import { buildApiParams } from '../utils'

test('common log API params expose group filtering only to administrators', () => {
  const config = {
    page: 1,
    pageSize: 20,
    searchParams: { group: 'url-group' },
    columnFilters: [{ id: 'group', value: 'column-group' }],
  }

  const userParams = buildApiParams({ ...config, isAdmin: false })
  assert.equal(userParams.group, undefined)

  const adminParams = buildApiParams({ ...config, isAdmin: true })
  assert.equal(adminParams.group, 'column-group')
})
