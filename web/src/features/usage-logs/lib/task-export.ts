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
import { formatTimestampToDate, quotaUnitsToDollars } from '@/lib/format'

import type { GetTaskExportParams, TaskLog } from '../types'
import {
  getTaskActionLabelKey,
  getTaskPlatformName,
  taskStatusMapper,
} from './mappers'
import { getDefaultTimeRange } from './utils'

export const TASK_EXPORT_MAX_RANGE_SECONDS = 31 * 24 * 60 * 60
export const TASK_EXPORT_PAGE_SIZE = 5000

export type TaskExportRow = Record<string, string | number>
type Translate = (key: string) => string

function formatQuotaAmount(quota: number): number {
  const amount = quotaUnitsToDollars(quota)
  if (!Number.isFinite(amount)) return 0
  return Number(amount.toFixed(6))
}

export function buildTaskExportParams(
  searchParams: Record<string, unknown>
): GetTaskExportParams {
  const defaultRange = getDefaultTimeRange()
  const startTime = Number(
    searchParams.startTime ?? defaultRange.start.getTime()
  )
  const endTime = Number(searchParams.endTime ?? defaultRange.end.getTime())

  return {
    start_timestamp: Math.floor(startTime / 1000),
    end_timestamp: Math.floor(endTime / 1000),
    ...(searchParams.filter ? { task_id: String(searchParams.filter) } : {}),
    ...(searchParams.channel
      ? { channel_id: String(searchParams.channel) }
      : {}),
    ...(searchParams.channels
      ? { channel_ids: String(searchParams.channels) }
      : {}),
    ...(searchParams.usernames
      ? { usernames: String(searchParams.usernames) }
      : {}),
    ...(searchParams.action ? { action: String(searchParams.action) } : {}),
    ...(searchParams.model ? { model_name: String(searchParams.model) } : {}),
    ...(searchParams.status ? { status: String(searchParams.status) } : {}),
  }
}

export function getTaskExportValidationError(
  params: GetTaskExportParams
): string | null {
  if (
    !Number.isFinite(params.start_timestamp) ||
    !Number.isFinite(params.end_timestamp) ||
    params.start_timestamp <= 0 ||
    params.end_timestamp <= 0 ||
    params.end_timestamp < params.start_timestamp
  ) {
    return 'Export failed. Please try again.'
  }
  return null
}

export function buildTaskExportRanges(
  params: GetTaskExportParams
): GetTaskExportParams[] {
  if (getTaskExportValidationError(params)) return []

  const ranges: GetTaskExportParams[] = []
  let rangeEnd = params.end_timestamp
  while (rangeEnd >= params.start_timestamp) {
    const rangeStart = Math.max(
      rangeEnd - TASK_EXPORT_MAX_RANGE_SECONDS,
      params.start_timestamp
    )
    ranges.push({
      ...params,
      start_timestamp: rangeStart,
      end_timestamp: rangeEnd,
    })
    rangeEnd = rangeStart - 1
  }
  return ranges
}

export function getTaskExportErrorKey(message?: string): string {
  const normalizedMessage = message?.toLowerCase() ?? ''
  if (normalizedMessage.includes('cannot exceed 31 days')) {
    return 'Task export range cannot exceed 31 days'
  }
  if (normalizedMessage.includes('exceeds 5000 rows')) {
    return 'Task export exceeds 5000 rows. Narrow the filters and try again.'
  }
  return 'Export failed. Please try again.'
}

export function buildTaskExportRows(
  items: TaskLog[],
  translate: Translate
): TaskExportRow[] {
  return items.map((task) => {
    const duration =
      task.submit_time && task.finish_time
        ? Math.max(0, task.finish_time - task.submit_time)
        : ''
    const refundQuota =
      task.refund_quota || (task.status === 'FAILURE' ? task.quota || 0 : 0)
    const videoDuration =
      task.video_duration ?? task.properties?.video_seconds ?? ''
    const rawReferenceDuration = task.properties?.reference_video_seconds
    const referenceDuration =
      typeof rawReferenceDuration === 'number' &&
      Number.isFinite(rawReferenceDuration) &&
      rawReferenceDuration > 0
        ? Number(rawReferenceDuration.toFixed(2))
        : ''
    const hasReferenceVideo =
      task.properties?.has_reference_video === true || referenceDuration !== ''

    return {
      [translate('Submit Time')]: task.submit_time
        ? formatTimestampToDate(task.submit_time, 'seconds')
        : '',
      [translate('Finish Time')]: task.finish_time
        ? formatTimestampToDate(task.finish_time, 'seconds')
        : '',
      [translate('Duration (s)')]: duration,
      [translate('Channel')]: task.channel_name || '',
      [translate('Channel ID')]: task.channel_id || '',
      [translate('User')]: task.username || '',
      [translate('Group')]: task.group || '',
      [translate('Platform')]: getTaskPlatformName(task.platform) || '',
      [translate('Model')]:
        task.model_name ||
        task.properties?.origin_model_name ||
        task.properties?.upstream_model_name ||
        '',
      [translate('Quota')]: formatQuotaAmount(task.quota || 0),
      [translate('Refund')]: refundQuota ? formatQuotaAmount(refundQuota) : '',
      [translate('Video Duration (s)')]: videoDuration,
      [translate('Reference video')]: hasReferenceVideo
        ? translate('Yes')
        : translate('No'),
      [translate('Reference Duration (s)')]: referenceDuration,
      [translate('Task Type')]: translate(
        getTaskActionLabelKey(
          task.action,
          task.properties?.video_generation_mode
        )
      ),
      [translate('Task ID')]: task.task_id || '',
      [translate('Status')]: translate(
        taskStatusMapper.getLabel(task.status, task.status || 'Submitting')
      ),
      [translate('Progress')]: task.progress || '',
      [translate('Details')]: task.fail_reason || '',
    }
  })
}
