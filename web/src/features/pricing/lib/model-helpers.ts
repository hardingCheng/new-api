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
import { EXCLUDED_GROUPS, FILTER_ALL, QUOTA_TYPE_VALUES } from '../constants'
import type {
  PricingModel,
  PricingUserPricing,
  PricingUserPricingGroup,
} from '../types'

// ----------------------------------------------------------------------------
// Model Helper Utilities
// ----------------------------------------------------------------------------

/**
 * Get available groups for a model
 */
export function getAvailableGroups(
  model: PricingModel,
  usableGroup: Record<string, { desc: string; ratio: number }>
): string[] {
  const modelEnableGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []

  return Object.keys(usableGroup)
    .filter((g) => !EXCLUDED_GROUPS.includes(g))
    .filter((g) => modelEnableGroups.includes(g))
}

/**
 * Read a configured group ratio while preserving valid zero ratios.
 */
export function getConfiguredGroupRatio(
  groupRatio: Record<string, number>,
  group: string
): number {
  const ratio = groupRatio[group]
  return typeof ratio === 'number' && Number.isFinite(ratio) ? ratio : 1
}

/**
 * Apply per-user group ratio overrides without mutating the shared API data.
 */
export function mergeUserPricingGroupRatios(
  baseRatios: Record<string, number>,
  userPricing?: PricingUserPricing
): Record<string, number> {
  const mergedRatios = { ...baseRatios }

  for (const [group, pricing] of Object.entries(userPricing?.groups ?? {})) {
    if (Number.isFinite(pricing.group_ratio) && pricing.group_ratio >= 0) {
      mergedRatios[group] = pricing.group_ratio
    }
  }

  return mergedRatios
}

/**
 * Resolve the group used for summary pricing. The same group selection is
 * shared by the model card and the request/token price formatters.
 */
export function getDisplayGroup(
  model: PricingModel,
  selectedGroup?: string
): string | undefined {
  const modelEnableGroups = Array.isArray(model.enable_groups)
    ? model.enable_groups
    : []

  if (
    selectedGroup &&
    selectedGroup !== FILTER_ALL &&
    modelEnableGroups.includes(selectedGroup)
  ) {
    return selectedGroup
  }

  let bestGroup: string | undefined
  let minRatio = Number.POSITIVE_INFINITY
  for (const group of modelEnableGroups) {
    const ratio = model.group_ratio?.[group]
    if (typeof ratio !== 'number' || !Number.isFinite(ratio)) continue
    if (ratio < minRatio) {
      minRatio = ratio
      bestGroup = group
    }
  }
  return bestGroup
}

export function getUserPricingGroup(
  model: PricingModel,
  group?: string
): PricingUserPricingGroup | undefined {
  if (!group) return undefined
  return model.user_pricing?.groups?.[group]
}

export function getEffectiveModelRatio(
  model: PricingModel,
  group?: string
): number {
  const override = getUserPricingGroup(model, group)
  if (
    override &&
    !override.use_price &&
    Number.isFinite(override.model_ratio) &&
    override.model_ratio >= 0
  ) {
    return override.model_ratio
  }
  return model.model_ratio
}

export function getEffectiveModelPrice(
  model: PricingModel,
  group?: string
): number {
  const override = getUserPricingGroup(model, group)
  if (
    override?.use_price &&
    Number.isFinite(override.model_price) &&
    override.model_price >= 0
  ) {
    return override.model_price
  }
  return model.model_price || 0
}

/**
 * Resolve the group ratio used by model square summary prices.
 *
 * When no specific group is selected, the model square shows the best price
 * available to the viewer. When a group filter is active, it shows that
 * group's price instead.
 */
export function getDisplayGroupRatio(
  model: PricingModel,
  selectedGroup?: string
): number {
  const group = getDisplayGroup(model, selectedGroup)
  return group ? getConfiguredGroupRatio(model.group_ratio || {}, group) : 1
}

/**
 * Replace model placeholder in endpoint path
 */
export function replaceModelInPath(path: string, modelName: string): string {
  return path.replaceAll('{model}', modelName)
}

/**
 * Check if model is token-based pricing
 */
export function isTokenBasedModel(model: PricingModel): boolean {
  return model.quota_type === QUOTA_TYPE_VALUES.TOKEN
}
