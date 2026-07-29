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
import {
  getEffectiveModelPrice,
  getEffectiveModelRatio,
  mergeUserPricingGroupRatios,
} from '../model-helpers'
import { formatFixedPrice } from '../price'

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

describe('pricing user overrides', () => {
  test('uses a user model price override for the selected group', () => {
    const model: PricingModel = {
      id: 1,
      model_name: 'video-model',
      quota_type: 1,
      model_ratio: 0,
      completion_ratio: 0,
      model_price: 1,
      enable_groups: ['sd2'],
      group_ratio: { sd2: 1 },
      user_pricing: {
        groups: {
          sd2: {
            use_price: true,
            model_price: 0.75,
            model_ratio: 0,
            group_ratio: 1,
          },
        },
      },
    }

    assert.equal(getEffectiveModelPrice(model, 'sd2'), 0.75)
    assert.equal(
      formatFixedPrice(model, 'sd2', false, 1, 1, { sd2: 1 }),
      '$0.75'
    )
  })

  test('uses a user model ratio override for token pricing', () => {
    const model: PricingModel = {
      id: 1,
      model_name: 'token-model',
      quota_type: 0,
      model_ratio: 2,
      completion_ratio: 1,
      enable_groups: ['vip'],
      group_ratio: { vip: 1 },
      user_pricing: {
        groups: {
          vip: {
            use_price: false,
            model_price: -1,
            model_ratio: 0.5,
            group_ratio: 1,
          },
        },
      },
    }

    assert.equal(getEffectiveModelRatio(model, 'vip'), 0.5)
  })

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
