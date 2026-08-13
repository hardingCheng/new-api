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

import type { Channel } from '../../types'
import {
  buildSettingJSON,
  CHANNEL_FORM_DEFAULT_VALUES,
  transformChannelToFormDefaults,
} from '../channel-form'

function makeChannel(setting: string | null): Channel {
  return {
    id: 1,
    type: 1,
    key: '',
    openai_organization: '',
    test_model: '',
    status: 1,
    name: 'upstream',
    weight: 0,
    created_time: 0,
    test_time: 0,
    response_time: 0,
    base_url: '',
    other: '',
    balance: 0,
    balance_updated_time: 0,
    models: 'gpt-5',
    group: 'default',
    used_quota: 0,
    model_mapping: '',
    status_code_mapping: '',
    priority: 0,
    auto_ban: 1,
    other_info: '',
    tag: '',
    setting,
    param_override: '',
    header_override: '',
    remark: '',
    max_input_tokens: 0,
    channel_info: {
      is_multi_key: false,
      multi_key_size: 0,
      multi_key_polling_index: 0,
      multi_key_mode: 'random',
    },
    settings: '{}',
  }
}

test('channel capacity: legacy setting JSON without capacity defaults both limits to 0', () => {
  const defaults = transformChannelToFormDefaults(
    makeChannel('{"proxy":"","force_format":false}')
  )

  assert.equal(defaults.capacity_rpm, 0)
  assert.equal(defaults.capacity_max_concurrency, 0)
})

test('channel capacity: non-number capacity values fall back to 0', () => {
  const defaults = transformChannelToFormDefaults(
    makeChannel('{"capacity":{"rpm":"100","max_concurrency":null}}')
  )

  assert.equal(defaults.capacity_rpm, 0)
  assert.equal(defaults.capacity_max_concurrency, 0)
})

test('channel capacity: round-trips rpm and max concurrency through setting JSON', () => {
  const defaults = transformChannelToFormDefaults(
    makeChannel('{"capacity":{"rpm":100,"max_concurrency":45}}')
  )

  assert.equal(defaults.capacity_rpm, 100)
  assert.equal(defaults.capacity_max_concurrency, 45)
  assert.ok(
    buildSettingJSON(defaults).includes(
      '"capacity":{"rpm":100,"max_concurrency":45}'
    )
  )
})

test('channel capacity: both limits at 0 emit no capacity key', () => {
  const built = JSON.parse(
    buildSettingJSON({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      capacity_rpm: 0,
      capacity_max_concurrency: 0,
    })
  )

  assert.equal('capacity' in built, false)
})

test('channel capacity: zero-valued member is omitted inside the capacity object', () => {
  const built = JSON.parse(
    buildSettingJSON({
      ...CHANNEL_FORM_DEFAULT_VALUES,
      capacity_rpm: 100,
      capacity_max_concurrency: 0,
    })
  )

  assert.deepEqual(built.capacity, { rpm: 100 })
})

test('channel capacity: rebuilding setting JSON preserves proxy, http protocol, and capacity together', () => {
  const setting = JSON.stringify({
    proxy: 'socks5://127.0.0.1:1080',
    http_protocol: 'http1',
    capacity: { rpm: 10 },
  })

  const rebuilt = JSON.parse(
    buildSettingJSON(transformChannelToFormDefaults(makeChannel(setting)))
  )

  assert.equal(rebuilt.proxy, 'socks5://127.0.0.1:1080')
  assert.equal(rebuilt.http_protocol, 'http1')
  assert.deepEqual(rebuilt.capacity, { rpm: 10 })
})
