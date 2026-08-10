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

export interface WorkbenchUncoveredSite {
  host: string
  name: string
  sell24: number
  reason: string
}

export interface WorkbenchStatusBar {
  pnl24: number | null
  /** 盈亏覆盖了多少比例的营收(0~1)。缺上游成本的站算不出利润，
   *  不标出来的话这个数看着像全站合计、实际只是子集。 */
  pnl24_coverage: number | null
  pnl24_uncovered_sell: number | null
  pnl24_uncovered_sites: WorkbenchUncoveredSite[] | null
  alarm_bad: number
  alarm_warn: number
  disabled_channels: number
  breaking_channels: number
  low_balance_sites: number
  last_collect_ts: number | null
  hub_db_error: string | null
}

export interface WorkbenchAlarm {
  level: 'bad' | 'warn'
  kind: string
  title: string
  detail: string
  link: string
}

export interface WorkbenchWatermark {
  peak_hour_reqs: number | null
  peak_rpm: number | null
  peak_rpm_line: number
  logs_rows: number | null
  logs_rows_line: number
}

export interface WorkbenchDailyPoint {
  day_ts: number
  requests: number
  quota: number
}

export interface WorkbenchSite {
  host: string
  name: string
  balance: number | null
  est_days: number | null
  daily_burn: number | null
  needs_topup: boolean
  error: string | null
}

export interface WorkbenchSummary {
  now: number
  status_bar: WorkbenchStatusBar
  alarms: WorkbenchAlarm[]
  watermark: WorkbenchWatermark | null
  daily: WorkbenchDailyPoint[]
  sites: WorkbenchSite[]
}

export interface WorkbenchSummaryResponse {
  success: boolean
  message?: string
  data?: WorkbenchSummary
}
