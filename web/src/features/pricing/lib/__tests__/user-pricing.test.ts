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
import { describe, test } from 'node:test'

import type { PricingModel, PricingUserPricing } from '../../types'
import { getDynamicDisplayGroupRatio } from '../dynamic-price'
import { mergeUserPricingGroupRatios } from '../model-helpers'

function userPricing(groupRatios: Record<string, number>): PricingUserPricing {
  return {
    groups: Object.fromEntries(
      Object.entries(groupRatios).map(([group, groupRatio]) => [
        group,
        {
          use_price: false,
          model_price: 0,
          model_ratio: 1,
          group_ratio: groupRatio,
        },
      ])
    ),
  }
}

describe('pricing user group ratio overrides', () => {
  test('shows a tiered model with the viewer-specific group discount', () => {
    const model: PricingModel = {
      id: 1,
      model_name: 'tiered-model',
      quota_type: 0,
      model_ratio: 1,
      completion_ratio: 1,
      enable_groups: ['vip'],
      billing_mode: 'tiered_expr',
      billing_expr: 'input_tokens: [0, $inf] -> inputPrice: 1',
      group_ratio: mergeUserPricingGroupRatios(
        { default: 1, vip: 1.2 },
        userPricing({ vip: 0.5 })
      ),
    }

    assert.equal(getDynamicDisplayGroupRatio(model, 'vip'), 0.5)
  })

  test('preserves an explicit zero override', () => {
    const result = mergeUserPricingGroupRatios(
      { default: 1, vip: 1.2 },
      userPricing({ vip: 0 })
    )

    assert.equal(result.vip, 0)
  })

  test('does not mutate the shared base group ratios', () => {
    const baseRatios = { default: 1, vip: 1.2 }

    const result = mergeUserPricingGroupRatios(
      baseRatios,
      userPricing({ vip: 0.5 })
    )

    assert.deepEqual(baseRatios, { default: 1, vip: 1.2 })
    assert.notEqual(result, baseRatios)
  })

  test('ignores invalid overrides and keeps the base ratios', () => {
    const result = mergeUserPricingGroupRatios(
      { default: 1, vip: 1.2, affiliate: 0.8 },
      userPricing({
        default: Number.NaN,
        vip: Number.POSITIVE_INFINITY,
        affiliate: -1,
      })
    )

    assert.deepEqual(result, { default: 1, vip: 1.2, affiliate: 0.8 })
  })
})
