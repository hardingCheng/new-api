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
export type UserChannelRoutingRule = {
  id: string
  name: string
  user_id: number
  username: string
  user_group: string
  group_pattern: string
  model_pattern: string
  channel_ids: number[]
  fallback: 'strict' | 'default'
  disabled: boolean
}

export function parseUserChannelRouting(
  rawValue: string
): UserChannelRoutingRule[] {
  try {
    const parsed = JSON.parse(rawValue || '{"rules":[]}') as {
      rules?: UserChannelRoutingRule[]
    }
    if (!Array.isArray(parsed.rules)) return []
    return parsed.rules.map((rule) => ({
      id: String(rule.id ?? ''),
      name: String(rule.name ?? ''),
      user_id: Number(rule.user_id),
      username: String(rule.username ?? ''),
      user_group: String(rule.user_group ?? ''),
      group_pattern: String(rule.group_pattern ?? ''),
      model_pattern: String(rule.model_pattern || '*'),
      channel_ids: Array.isArray(rule.channel_ids)
        ? rule.channel_ids.map(Number).filter((id) => id > 0)
        : [],
      fallback: rule.fallback === 'default' ? 'default' : 'strict',
      disabled: Boolean(rule.disabled),
    }))
  } catch {
    return []
  }
}

export function serializeUserChannelRouting(rules: UserChannelRoutingRule[]) {
  return JSON.stringify({ rules })
}
