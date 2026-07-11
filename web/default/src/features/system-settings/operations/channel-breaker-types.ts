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
export type BreakerScope = 'global' | 'group' | 'model' | 'channel'

export type BreakerRule = {
  id: string
  name: string
  enabled: boolean
  scope: BreakerScope
  targets: string[]
  failure_limit: number
  cooldown_seconds: number
  probe_count: number
  probe_success_count: number
  failure_status_codes: string
  failure_keywords: string
  exclude_paths: string
  disable_breaker: boolean
  only_key_breaker: boolean
  ignore_client_error_4xx: boolean
  instant_disable_enabled: boolean
  instant_disable_status_codes: string
  instant_disable_keywords: string
}

export type BreakerStatus = {
  state_key: string
  channel_id: number
  channel_name?: string
  channel_group?: string
  key_hash?: string
  state: 'open' | 'half-open' | 'closed'
  failures: number
  opened_at?: string
  probe_total: number
  probe_success: number
  cooldown_remaining_seconds: number
  rule_name?: string
  group?: string
  model?: string
  rule_probe_count?: number
  rule_probe_success_count?: number
}

export type BreakerHistory = {
  id: number
  created_at: number
  channel_id: number
  channel_name?: string
  model_name?: string
  using_group?: string
  rule_name?: string
  failures: number
  cooldown_secs: number
  reason?: string
}

export const DEFAULT_BREAKER_RULE: BreakerRule = {
  id: '',
  name: 'Reliable default',
  enabled: true,
  scope: 'group',
  targets: [],
  failure_limit: 5,
  cooldown_seconds: 60,
  probe_count: 5,
  probe_success_count: 3,
  failure_status_codes: '429,500-599',
  failure_keywords:
    'rate limit\ntemporarily unavailable\noverloaded\nserver error',
  exclude_paths: '/v1/videos',
  disable_breaker: false,
  only_key_breaker: false,
  ignore_client_error_4xx: false,
  instant_disable_enabled: false,
  instant_disable_status_codes: '',
  instant_disable_keywords: '',
}

export const BREAKER_TEMPLATES: Array<{
  label: string
  rule: Partial<BreakerRule>
}> = [
  { label: 'Reliable default', rule: {} },
  {
    label: 'Strict protection',
    rule: {
      failure_limit: 3,
      cooldown_seconds: 120,
      probe_success_count: 4,
      failure_status_codes: '401,403,429,500-599',
      failure_keywords:
        'insufficient quota\npermission denied\nrate limit\noverloaded',
    },
  },
  {
    label: 'Lenient tolerance',
    rule: {
      failure_limit: 10,
      cooldown_seconds: 30,
      probe_success_count: 2,
      failure_status_codes: '500-599',
    },
  },
  {
    label: 'Exclude video and async tasks',
    rule: {
      failure_limit: 20,
      cooldown_seconds: 30,
      probe_success_count: 2,
      disable_breaker: true,
    },
  },
  {
    label: 'Disable on upstream balance exhaustion',
    rule: {
      instant_disable_enabled: true,
      instant_disable_status_codes: '403',
      instant_disable_keywords:
        'insufficient account balance\ninsufficient_user_quota\n预扣费额度失败',
    },
  },
]

export function normalizeBreakerRule(raw: Partial<BreakerRule>): BreakerRule {
  const probeCount = Math.max(1, Number(raw.probe_count) || 5)
  return {
    ...DEFAULT_BREAKER_RULE,
    ...raw,
    id:
      raw.id || `rule-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
    targets: Array.isArray(raw.targets)
      ? raw.targets
      : String(raw.targets || '')
          .split(/[\n,，]/)
          .map((item) => item.trim())
          .filter(Boolean),
    failure_limit: Math.max(1, Number(raw.failure_limit) || 5),
    cooldown_seconds: Math.max(1, Number(raw.cooldown_seconds) || 60),
    probe_count: probeCount,
    probe_success_count: Math.min(
      probeCount,
      Math.max(1, Number(raw.probe_success_count) || 3)
    ),
  }
}
