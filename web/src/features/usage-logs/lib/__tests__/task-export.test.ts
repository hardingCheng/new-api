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
import test from 'node:test'

import type { TaskLog } from '../../types'
import { buildSearchParams } from '../filter'
import {
  buildTaskExportParams,
  buildTaskExportRanges,
  buildTaskExportRows,
  getTaskExportValidationError,
} from '../task-export'

test('task export converts current URL filters to backend parameters', () => {
  const params = buildTaskExportParams({
    startTime: 1_783_180_800_000,
    endTime: 1_786_291_200_000,
    filter: 'task_public_id',
    channel: '390',
    channels: '390,391',
    usernames: 'samuel0630,alice',
    action: 'firstAndLastFrames',
    model: 'seedance-2.0',
    status: 'SUCCESS',
  })

  assert.deepEqual(params, {
    start_timestamp: 1_783_180_800,
    end_timestamp: 1_786_291_200,
    task_id: 'task_public_id',
    channel_id: '390',
    channel_ids: '390,391',
    usernames: 'samuel0630,alice',
    action: 'firstAndLastFrames',
    model_name: 'seedance-2.0',
    status: 'SUCCESS',
  })
})

test('task filters serialize multi-select and task fields to URL parameters', () => {
  const params = buildSearchParams(
    {
      taskId: 'task_public_id',
      usernames: ['samuel0630', 'alice'],
      channels: ['390', '391'],
      action: 'firstAndLastFrames',
      model: 'seedance-2.0',
      status: 'SUCCESS',
    },
    'task'
  )

  assert.deepEqual(params, {
    filter: 'task_public_id',
    usernames: 'samuel0630,alice',
    channels: '390,391',
    action: 'firstAndLastFrames',
    model: 'seedance-2.0',
    status: 'SUCCESS',
  })
})

test('task export splits a range longer than 31 days without gaps', () => {
  const params = buildTaskExportParams({
    startTime: 1_783_180_800_000,
    endTime: 1_786_291_200_000,
  })

  assert.equal(getTaskExportValidationError(params), null)
  assert.deepEqual(buildTaskExportRanges(params), [
    {
      start_timestamp: 1_783_612_800,
      end_timestamp: 1_786_291_200,
    },
    {
      start_timestamp: 1_783_180_800,
      end_timestamp: 1_783_612_799,
    },
  ])
})

test('task export rejects non-finite time ranges before splitting', () => {
  const params = {
    start_timestamp: 1_783_180_800,
    end_timestamp: Number.POSITIVE_INFINITY,
  }

  assert.equal(
    getTaskExportValidationError(params),
    'Export failed. Please try again.'
  )
  assert.deepEqual(buildTaskExportRanges(params), [])
})

test('task export rows include task, billing, and status data', () => {
  const task: TaskLog = {
    id: 1,
    user_id: 210,
    username: 'samuel0630',
    platform: '1',
    task_id: 'task_public_id',
    action: 'firstAndLastFrames',
    channel_id: 390,
    channel_name: 'video-channel',
    group: 'sd2',
    model_name: 'seedance-2.0-720p',
    quota: 500_000,
    refund_quota: 500_000,
    submit_time: 1_786_283_735,
    finish_time: 1_786_283_758,
    video_duration: 10,
    progress: '100%',
    fail_reason: 'generation failed',
    status: 'FAILURE',
    properties: {
      has_reference_video: true,
      reference_video_seconds: 4.126,
      video_generation_mode: 'first_last_frame',
    },
  }

  const rows = buildTaskExportRows([task], (key) => key)

  assert.equal(rows.length, 1)
  assert.equal(rows[0]?.Channel, 'video-channel')
  assert.equal(rows[0]?.User, 'samuel0630')
  assert.equal(rows[0]?.['Task ID'], 'task_public_id')
  assert.equal(rows[0]?.['Task Type'], 'First/Last Frame to Video')
  assert.equal(rows[0]?.Status, 'Failed')
  assert.equal(rows[0]?.['Duration (s)'], 23)
  assert.equal(rows[0]?.['Reference video'], 'Yes')
  assert.equal(rows[0]?.['Reference Duration (s)'], 4.13)
  assert.equal(rows[0]?.Details, 'generation failed')
})
