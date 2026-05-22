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
/* eslint-disable react-refresh/only-export-components */
import { useState, useMemo } from 'react'
import type { ColumnDef } from '@tanstack/react-table'
import { Music } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { formatLogQuota } from '@/lib/format'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { DataTableColumnHeader } from '@/components/data-table'
import { StatusBadge } from '@/components/status-badge'
import { TASK_ACTIONS, TASK_STATUS } from '../../constants'
import {
  taskActionMapper,
  taskPlatformMapper,
  taskStatusMapper,
} from '../../lib/mappers'
import type { TaskLog } from '../../types'
import {
  AudioPreviewDialog,
  type AudioClip,
} from '../dialogs/audio-preview-dialog'
import { FailReasonDialog } from '../dialogs/fail-reason-dialog'
import { useUsageLogsContext } from '../usage-logs-provider'
import {
  createDurationColumn,
  createChannelColumn,
  createProgressColumn,
} from './column-helpers'

function parseTaskData(data: unknown): unknown[] {
  if (Array.isArray(data)) return data
  if (typeof data === 'string') {
    try {
      const parsed = JSON.parse(data)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }
  return []
}

function AudioPreviewCell({ log }: { log: TaskLog }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const clips = useMemo(() => {
    const data = parseTaskData(log.data)
    return data.filter(
      (c) =>
        c && typeof c === 'object' && (c as Record<string, unknown>).audio_url
    )
  }, [log.data])

  if (clips.length === 0) return null

  return (
    <>
      <button
        type='button'
        className='group flex items-center gap-1 text-left text-xs'
        onClick={() => setOpen(true)}
      >
        <Music className='text-muted-foreground size-3' />
        <span className='text-foreground leading-snug group-hover:underline'>
          {t('Click to preview audio')}
        </span>
      </button>
      <AudioPreviewDialog
        open={open}
        onOpenChange={setOpen}
        clips={clips as AudioClip[]}
      />
    </>
  )
}

export function useTaskLogsColumns(isAdmin: boolean): ColumnDef<TaskLog>[] {
  const { t } = useTranslation()
  const columns: ColumnDef<TaskLog>[] = [
    {
      accessorKey: 'submit_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Submit Time')} />
      ),
      cell: ({ row }) => {
        const submitTime = row.getValue('submit_time') as number

        return (
          <span className='font-mono text-xs tabular-nums'>
            {formatTimestampToDate(submitTime, 'seconds')}
          </span>
        )
      },
      meta: { label: t('Submit Time') },
    },
    {
      accessorKey: 'finish_time',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Finish Time')} />
      ),
      cell: ({ row }) => {
        const finishTime = row.getValue('finish_time') as number
        if (!finishTime) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <span className='font-mono text-xs tabular-nums'>
            {formatTimestampToDate(finishTime, 'seconds')}
          </span>
        )
      },
      meta: { label: t('Finish Time') },
    },
  ]

  if (isAdmin) {
    columns.push(createChannelColumn<TaskLog>({ headerLabel: t('Channel') }), {
      accessorKey: 'channel_name',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Channel Name')} />
      ),
      cell: ({ row }) => {
        const channelName = row.getValue('channel_name') as string | undefined
        if (!channelName) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <StatusBadge
            label={channelName}
            autoColor={channelName}
            copyText={channelName}
            size='sm'
            showDot={false}
            className='max-w-[180px] truncate'
          />
        )
      },
      meta: { label: t('Channel Name'), mobileHidden: true },
    }, {
      id: 'user',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('User')} />
      ),
      cell: function UserCell({ row }) {
        const { sensitiveVisible, setSelectedUserId, setUserInfoDialogOpen } =
          useUsageLogsContext()
        const log = row.original
        const displayName = log.username || String(log.user_id || '?')

        return (
          <button
            type='button'
            className='flex items-center gap-1.5 text-left'
            onClick={(e) => {
              e.stopPropagation()
              setSelectedUserId(log.user_id)
              setUserInfoDialogOpen(true)
            }}
          >
            <Avatar className='ring-border/60 size-6 ring-1'>
              <AvatarFallback
                className={cn(
                  'text-[11px] font-semibold',
                  !sensitiveVisible && 'bg-muted text-muted-foreground'
                )}
                style={
                  sensitiveVisible ? getUserAvatarStyle(displayName) : undefined
                }
              >
                {sensitiveVisible ? getUserAvatarFallback(displayName) : '•'}
              </AvatarFallback>
            </Avatar>
            <span className='text-muted-foreground truncate text-sm hover:underline'>
              {sensitiveVisible ? displayName : '••••'}
            </span>
          </button>
        )
      },
      meta: { label: t('User'), mobileHidden: true },
    })
  }

  columns.push(
    {
      accessorKey: 'task_id',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Task ID')} />
      ),
      cell: ({ row }) => {
        const taskId = row.getValue('task_id') as string
        if (!taskId) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <StatusBadge
            label={taskId}
            autoColor={taskId}
            size='sm'
            showDot={false}
            className='border-border/60 bg-muted/30 max-w-[170px] truncate rounded-md border px-1.5 py-0.5 font-mono'
          />
        )
      },
      meta: { label: t('Task ID'), mobileTitle: true },
    },
    {
      accessorKey: 'platform',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Platform')} />
      ),
      cell: ({ row }) => {
        const platform = row.getValue('platform') as string
        if (!platform) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <StatusBadge
            label={t(taskPlatformMapper.getLabel(platform, platform))}
            variant={taskPlatformMapper.getVariant(platform)}
            size='sm'
            copyable={false}
            showDot
          />
        )
      },
      meta: { label: t('Platform'), mobileHidden: true },
    },
    {
      accessorKey: 'model_name',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Model')} />
      ),
      cell: ({ row }) => {
        const modelName = row.getValue('model_name') as string | undefined
        if (!modelName) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <StatusBadge
            label={modelName}
            autoColor={modelName}
            size='sm'
            copyText={modelName}
            showDot={false}
            className='max-w-[180px] truncate'
          />
        )
      },
      meta: { label: t('Model'), mobileHidden: true },
    },
    {
      accessorKey: 'action',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Task Type')} />
      ),
      cell: ({ row }) => {
        const action = row.getValue('action') as string
        if (!action) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }
        return (
          <StatusBadge
            label={t(taskActionMapper.getLabel(action, action))}
            variant={taskActionMapper.getVariant(action)}
            size='sm'
            copyable={false}
            showDot
          />
        )
      },
      meta: { label: t('Task Type'), mobileHidden: true },
    },
    createDurationColumn<TaskLog>({
      submitTimeKey: 'submit_time',
      finishTimeKey: 'finish_time',
      unit: 'seconds',
      headerLabel: t('Duration'),
      warningThresholdSec: 300,
    })
  )

  if (isAdmin) {
    columns.push(
      {
        accessorKey: 'consumed_quota',
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Consumed Quota')} />
        ),
        cell: ({ row }) => {
          const log = row.original
          const quota = log.consumed_quota ?? log.quota ?? 0
          return (
            <span className='border-border/80 bg-muted/60 inline-flex w-fit items-center rounded-md border px-1.5 py-0.5 font-mono text-xs font-semibold tabular-nums'>
              {formatLogQuota(quota)}
            </span>
          )
        },
        meta: { label: t('Consumed Quota') },
      },
      {
        accessorKey: 'video_seconds',
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Video Duration')} />
        ),
        cell: ({ row }) => {
          const seconds = row.getValue('video_seconds') as string | undefined
          return (
            <span className='font-mono text-xs tabular-nums'>
              {seconds ? `${seconds}s` : '-'}
            </span>
          )
        },
        meta: { label: t('Video Duration') },
      },
      {
        accessorKey: 'refund_quota',
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Refund Quota')} />
        ),
        cell: ({ row }) => {
          const refundQuota = row.getValue('refund_quota') as number | undefined
          if (!refundQuota) {
            return <span className='text-muted-foreground/60 text-xs'>-</span>
          }
          return (
            <span className='inline-flex w-fit items-center rounded-md border border-blue-200 bg-blue-50 px-1.5 py-0.5 font-mono text-xs font-semibold text-blue-700 tabular-nums dark:border-blue-800 dark:bg-blue-950/40 dark:text-blue-300'>
              {formatLogQuota(refundQuota)}
            </span>
          )
        },
        meta: { label: t('Refund Quota') },
      },
      {
        accessorKey: 'has_video_reference',
        header: ({ column }) => (
          <DataTableColumnHeader column={column} title={t('Video Reference')} />
        ),
        cell: ({ row }) => {
          const hasReference = row.getValue('has_video_reference') as boolean
          return (
            <StatusBadge
              label={hasReference ? t('Yes') : t('No')}
              variant={hasReference ? 'blue' : 'neutral'}
              size='sm'
              copyable={false}
              showDot
            />
          )
        },
        meta: { label: t('Video Reference') },
      }
    )
  }

  columns.push(
    {
      accessorKey: 'status',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Status')} />
      ),
      cell: ({ row }) => {
        const status = row.getValue('status') as string
        return (
          <StatusBadge
            label={t(taskStatusMapper.getLabel(status, status || 'Submitting'))}
            variant={taskStatusMapper.getVariant(status)}
            size='sm'
            copyable={false}
            showDot
          />
        )
      },
      meta: { label: t('Status') },
    },
    createProgressColumn<TaskLog>({ headerLabel: t('Progress') }),
    {
      accessorKey: 'fail_reason',
      header: ({ column }) => (
        <DataTableColumnHeader column={column} title={t('Details')} />
      ),
      cell: function DetailsCell({ row }) {
        const log = row.original
        const failReason = row.getValue('fail_reason') as string
        const status = log.status
        const [dialogOpen, setDialogOpen] = useState(false)

        const isSunoSuccess =
          log.platform === 'suno' && status === TASK_STATUS.SUCCESS
        if (isSunoSuccess) {
          const data = parseTaskData(log.data)
          if (
            data.some(
              (c) =>
                c &&
                typeof c === 'object' &&
                (c as Record<string, unknown>).audio_url
            )
          ) {
            return <AudioPreviewCell log={log} />
          }
        }

        const isVideoTask =
          log.action === TASK_ACTIONS.GENERATE ||
          log.action === TASK_ACTIONS.TEXT_GENERATE ||
          log.action === TASK_ACTIONS.FIRST_TAIL_GENERATE ||
          log.action === TASK_ACTIONS.REFERENCE_GENERATE ||
          log.action === TASK_ACTIONS.REMIX_GENERATE
        const isSuccess = status === TASK_STATUS.SUCCESS
        const isUrl = failReason?.startsWith('http')

        if (isSuccess && isVideoTask && isUrl) {
          const videoUrl = `/v1/videos/${log.task_id}/content`
          return (
            <a
              href={videoUrl}
              target='_blank'
              rel='noopener noreferrer'
              className='text-foreground text-xs hover:underline'
            >
              {t('Click to preview video')}
            </a>
          )
        }

        if (!failReason) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }

        return (
          <>
            <button
              type='button'
              className='group flex max-w-[200px] items-center gap-1 text-left text-xs'
              onClick={() => setDialogOpen(true)}
              title={t('Click to view full error message')}
            >
              <span className='truncate leading-snug text-red-600 group-hover:underline dark:text-red-400'>
                {failReason}
              </span>
            </button>
            <FailReasonDialog
              failReason={failReason}
              open={dialogOpen}
              onOpenChange={setDialogOpen}
            />
          </>
        )
      },
      meta: { label: t('Details') },
      size: 200,
      maxSize: 220,
    }
  )

  return columns
}
