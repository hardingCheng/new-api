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
import type { ColumnDef } from '@tanstack/react-table'
import { Check, Copy, Music } from 'lucide-react'
/* eslint-disable react-refresh/only-export-components */
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getUserAvatarFallback, getUserAvatarStyle } from '@/lib/avatar'
import { formatLogQuota, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { TASK_STATUS } from '../../constants'
import { taskStatusMapper } from '../../lib/mappers'
import { resolveTaskVideoPreviewUrl } from '../../lib/task-video-preview'
import type { TaskLog } from '../../types'
import {
  AudioPreviewDialog,
  type AudioClip,
} from '../dialogs/audio-preview-dialog'
import { FailReasonDialog } from '../dialogs/fail-reason-dialog'
import { TaskDetailsDialog } from '../dialogs/task-details-dialog'
import { TaskVideoPreview } from '../dialogs/video-preview-dialog'
import { useUsageLogsContext } from '../usage-logs-provider'
import { createDurationColumn, createProgressColumn } from './column-helpers'

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
      header: t('Submit Time'),
      cell: ({ row }) => {
        const log = row.original
        const submitTime = row.getValue('submit_time') as number

        return (
          <div className='flex min-w-0 flex-col gap-0.5'>
            <span className='truncate font-mono text-xs tabular-nums'>
              {formatTimestampToDate(submitTime, 'seconds')}
            </span>
            {log.finish_time ? (
              <span className='text-muted-foreground/60 truncate font-mono text-[11px] tabular-nums'>
                {formatTimestampToDate(log.finish_time, 'seconds')}
              </span>
            ) : (
              <span className='text-muted-foreground/50 text-[11px]'>-</span>
            )}
          </div>
        )
      },
      size: 180,
    },
  ]

  if (isAdmin) {
    columns.push(
      {
        id: 'channel',
        accessorFn: (row) => row.channel_id,
        header: t('Channel'),
        cell: function ChannelCell({ row }) {
          const { sensitiveVisible } = useUsageLogsContext()
          const log = row.original
          const channelName = sensitiveVisible ? log.channel_name : '••••'

          if (!log.channel_id && !log.channel_name) {
            return <span className='text-muted-foreground/60 text-xs'>-</span>
          }

          return (
            <div className='flex max-w-[160px] min-w-0 flex-col gap-0.5'>
              {log.channel_id ? (
                <StatusBadge
                  label={`#${log.channel_id}`}
                  autoColor={String(log.channel_id)}
                  copyText={String(log.channel_id)}
                  size='sm'
                  showDot={false}
                  className='font-mono'
                />
              ) : null}
              {log.channel_name ? (
                <span className='text-muted-foreground truncate text-[11px]'>
                  {channelName}
                </span>
              ) : null}
            </div>
          )
        },
      },
      {
        id: 'user',
        header: t('User'),
        accessorFn: (row) => row.username || row.user_id,
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
              <Avatar className='ring-border/60 size-6 ring-1 max-sm:hidden'>
                <AvatarFallback
                  className={cn(
                    'text-[11px] font-semibold',
                    !sensitiveVisible && 'bg-muted text-muted-foreground'
                  )}
                  style={
                    sensitiveVisible
                      ? getUserAvatarStyle(displayName)
                      : undefined
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
      }
    )
  }

  columns.push({
    accessorKey: 'task_id',
    header: t('Task ID'),
    cell: ({ row }) => {
      const taskId = row.getValue('task_id') as string
      if (!taskId) {
        return <span className='text-muted-foreground/60 text-xs'>-</span>
      }
      return <TaskIdCell log={row.original} isAdmin={isAdmin} />
    },
    meta: { mobileTitle: true },
  })

  if (isAdmin) {
    columns.push({
      accessorKey: 'model_name',
      header: t('Model'),
      cell: ({ row }) => {
        const log = row.original
        const publicModel =
          log.model_name || log.properties?.origin_model_name || ''
        const upstreamModel = log.properties?.upstream_model_name || publicModel
        if (!upstreamModel) {
          return <span className='text-muted-foreground/60 text-xs'>-</span>
        }

        return (
          <div className='flex max-w-[190px] min-w-0 flex-col gap-0.5'>
            <StatusBadge
              label={upstreamModel}
              copyText={upstreamModel}
              autoColor={upstreamModel}
              size='sm'
              className='max-w-full font-mono'
            />
            {publicModel && publicModel !== upstreamModel ? (
              <span className='text-muted-foreground/60 truncate font-mono text-[11px]'>
                {publicModel}
              </span>
            ) : null}
          </div>
        )
      },
      size: 180,
    })
  }

  columns.push(
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
        accessorKey: 'video_duration',
        header: t('Video Duration'),
        cell: ({ row }) => {
          const log = row.original
          const seconds = log.video_duration ?? log.properties?.video_seconds
          if (
            typeof seconds !== 'number' ||
            !Number.isFinite(seconds) ||
            seconds <= 0
          ) {
            return <span className='text-muted-foreground/60 text-xs'>-</span>
          }
          return (
            <span className='font-mono text-xs font-medium tabular-nums'>
              {seconds}s
            </span>
          )
        },
        size: 110,
      },
      {
        id: 'billing',
        header: t('Billing'),
        cell: function BillingCell({ row }) {
          const { sensitiveVisible } = useUsageLogsContext()
          const log = row.original
          const fee = log.quota ?? 0
          const refund = log.refund_quota ?? 0

          return (
            <div className='flex min-w-[130px] flex-col gap-0.5 font-mono text-xs tabular-nums'>
              <span>
                <span className='text-muted-foreground'>{t('Fee')}:</span>{' '}
                {sensitiveVisible ? formatLogQuota(fee) : '••••'}
              </span>
              <span
                className={cn(
                  refund > 0
                    ? 'text-blue-600 dark:text-blue-400'
                    : 'text-muted-foreground/60'
                )}
              >
                {t('Refund')}:{' '}
                {sensitiveVisible ? formatLogQuota(refund) : '••••'}
              </span>
            </div>
          )
        },
        size: 155,
      },
      {
        id: 'reference_video',
        header: t('Reference video'),
        cell: ({ row }) => {
          const properties = row.original.properties
          const referenceSeconds = properties?.reference_video_seconds
          const hasReference =
            properties?.has_reference_video === true ||
            (typeof referenceSeconds === 'number' && referenceSeconds > 0)

          return (
            <StatusBadge
              label={hasReference ? t('Yes') : t('No')}
              variant={hasReference ? 'blue' : 'neutral'}
              size='sm'
              copyable={false}
            />
          )
        },
        size: 115,
      },
      {
        id: 'reference_video_duration',
        header: t('Reference Duration'),
        cell: ({ row }) => {
          const seconds = row.original.properties?.reference_video_seconds
          if (
            typeof seconds !== 'number' ||
            !Number.isFinite(seconds) ||
            seconds <= 0
          ) {
            return <span className='text-muted-foreground/60 text-xs'>-</span>
          }

          return (
            <span className='font-mono text-xs font-medium tabular-nums'>
              {seconds}s
            </span>
          )
        },
        size: 135,
      }
    )
  }

  columns.push(
    {
      accessorKey: 'status',
      header: t('Status'),
      cell: ({ row }) => {
        const status = row.getValue('status') as string
        return (
          <StatusBadge
            label={t(taskStatusMapper.getLabel(status, status || 'Submitting'))}
            variant={taskStatusMapper.getVariant(status)}
            size='sm'
            copyable={false}
            className='-ml-1.5'
          />
        )
      },
    },
    createProgressColumn<TaskLog>({ headerLabel: t('Progress') }),
    {
      accessorKey: 'fail_reason',
      header: t('Details'),
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

        const videoUrl = resolveTaskVideoPreviewUrl(log)
        if (videoUrl) {
          return (
            <TaskVideoPreview
              taskId={log.task_id}
              sourceUrl={videoUrl}
              ownerUserId={isAdmin ? log.user_id : undefined}
            />
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
      size: 200,
      maxSize: 220,
    }
  )

  return columns
}

function TaskIdCell(props: { log: TaskLog; isAdmin: boolean }) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const [open, setOpen] = useState(false)

  return (
    <>
      <div className='flex max-w-[190px] min-w-0 items-center gap-0.5'>
        <button
          type='button'
          className='block min-w-0 flex-1 text-left'
          onClick={(event) => {
            event.stopPropagation()
            setOpen(true)
          }}
          title={t('View the complete details for this task')}
          aria-label={`${t('Task Details')}: ${props.log.task_id}`}
          aria-haspopup='dialog'
          aria-expanded={open}
        >
          <StatusBadge
            label={props.log.task_id}
            variant='neutral'
            size='sm'
            copyable={false}
            className='border-border/60 bg-muted/30 !text-foreground max-w-full rounded-md border px-1.5 py-0.5 font-mono hover:underline'
          />
        </button>
        <Button
          type='button'
          variant='ghost'
          size='icon-xs'
          className='text-muted-foreground hover:text-foreground shrink-0'
          onClick={(event) => {
            event.stopPropagation()
            void copyToClipboard(props.log.task_id)
          }}
          title={t('Copy to clipboard')}
          aria-label={`${t('Copy to clipboard')}: ${props.log.task_id}`}
        >
          {copiedText === props.log.task_id ? <Check /> : <Copy />}
        </Button>
      </div>
      {open ? (
        <TaskDetailsDialog
          log={props.log}
          isAdmin={props.isAdmin}
          open={open}
          onOpenChange={setOpen}
        />
      ) : null}
    </>
  )
}
