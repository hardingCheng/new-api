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
import { TASK_ACTIONS, TASK_STATUS } from '../constants'
import type { TaskLog } from '../types'

const VIDEO_ACTIONS = new Set<string>([
  TASK_ACTIONS.GENERATE,
  TASK_ACTIONS.TEXT_GENERATE,
  TASK_ACTIONS.FIRST_TAIL_GENERATE,
  TASK_ACTIONS.REFERENCE_GENERATE,
  TASK_ACTIONS.REMIX_GENERATE,
])

function isPlayableVideoUrl(value: string): boolean {
  return (
    /^https?:\/\//i.test(value) ||
    value.startsWith('/v1/videos/') ||
    /^data:video\//i.test(value)
  )
}

export function resolveTaskVideoPreviewUrl(log: TaskLog): string {
  for (const value of [log.result_url, log.video_url, log.url]) {
    const candidate = value?.trim() ?? ''
    if (isPlayableVideoUrl(candidate)) return candidate
  }

  if (
    log.status === TASK_STATUS.SUCCESS &&
    VIDEO_ACTIONS.has(log.action) &&
    log.task_id
  ) {
    return `/v1/videos/${encodeURIComponent(log.task_id)}/content`
  }

  return ''
}
