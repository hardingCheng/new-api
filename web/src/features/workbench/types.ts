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
  /** 未计入的上游**总数**。`pnl24_uncovered_sites` 后端已经截过一刀,
   *  拿它的长度算「还有几个没列出」会少报,必须用这个数。
   *  老监控服务没有这个字段,所以可能缺。 */
  pnl24_uncovered_total?: number | null
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
  occurred_at?: number | null
  /** 这条报警说的是哪个实体(渠道 / 上游站 / 分组)。列表 key 要用它:
   *  报警文本不足以区分两条 —— 渠道名在库里没有唯一约束
   *  (`model/channel.go` 的 Name 是普通 index),两个同名渠道同时亏损时
   *  标题、明细、link 会逐字相同。老监控服务没有这个字段,所以可能缺。 */
  ref?: string
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

/** 硬失败按渠道的分布。 */
export interface WorkbenchHardFailChannel {
  /** 0 表示这些请求还没落到任何渠道就被拒了,此时 `name` 是空串。
   *  这个编号只用来分辨那一种情形,不上屏 —— 内部编号不进 UI。 */
  channel_id: number
  name: string
  count: number
}

/** 硬失败按客户分组的分布。 */
export interface WorkbenchHardFailGroup {
  group: string
  /** 已经是人话(如「z站客户」),直接渲染。分组名的翻译在监控服务侧维护,
   *  前端再维护一套必然和它对不上。 */
  label: string
  count: number
}

/** 客户最终吃到的失败:重试和换渠道都救不回来、真的返回给客户的那些请求。
 *
 *  和「上游失败数」(`errors`)是两个数,不能互相代替:上游失败里绝大多数被重试
 *  换渠道救回来了,它反映供给质量;这里的 `hard_fails` 才是客户实际的体验。
 *
 *  监控服务这一轮查不到时整块缺失(页面另有全局故障态),所以字段全部可缺。 */
export interface WorkbenchHardFail {
  /** 统计窗口的长度(秒)。标签上的时间窗跟着它走,不写死。 */
  window_sec?: number | null
  /** null = 算不出来(判据失效或分布被截断),**不是 0**。监控服务保证不用 0
   *  代替 null,所以这两种情形必须分开渲染:显示成 0 就是把「不知道」说成
   *  「没事」,而这一格存在的理由正是回答客户到底疼不疼。 */
  hard_fails?: number | null
  /** 窗口内成功计费的请求数。占比由监控服务算,前端不再自己除一遍。 */
  requests?: number | null
  /** 窗口内全部失败数,含被重试救回来的。供给侧质量,不是客户体验。 */
  errors?: number | null
  /** `hard_fails / (requests + hard_fails)`;`hard_fails` 为 null 时必为 null。 */
  rate?: number | null
  by_channel?: WorkbenchHardFailChannel[] | null
  by_group?: WorkbenchHardFailGroup[] | null
}

export interface WorkbenchSummary {
  now: number
  status_bar: WorkbenchStatusBar
  alarms: WorkbenchAlarm[]
  watermark: WorkbenchWatermark | null
  daily: WorkbenchDailyPoint[]
  sites: WorkbenchSite[]
  hard_fail?: WorkbenchHardFail | null
}

export interface WorkbenchSummaryResponse {
  success: boolean
  message?: string
  data?: WorkbenchSummary
}
