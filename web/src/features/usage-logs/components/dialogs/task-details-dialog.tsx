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
import {
  AlertTriangle,
  Check,
  Clock3,
  Copy,
  FileJson2,
  Info,
  ReceiptText,
  Route,
  Video,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { IconBadge, type IconBadgeTone } from '@/components/ui/icon-badge'
import { Label } from '@/components/ui/label'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { formatLogQuota, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { formatDuration } from '../../lib/format'
import {
  getTaskPlatformName,
  taskActionMapper,
  taskStatusMapper,
} from '../../lib/mappers'
import { resolveTaskVideoPreviewUrl } from '../../lib/task-video-preview'
import type { TaskLog } from '../../types'
import { TaskVideoPreview } from './video-preview-dialog'

function DetailRow(props: {
  label: React.ReactNode
  value: React.ReactNode
  mono?: boolean
}) {
  return (
    <div className='grid min-w-0 grid-cols-[6rem_minmax(0,1fr)] gap-2 sm:grid-cols-[8rem_minmax(0,1fr)] sm:gap-3'>
      <span className='text-muted-foreground min-w-0 text-xs'>
        {props.label}
      </span>
      <span
        className={cn(
          'max-w-full min-w-0 text-xs break-all sm:wrap-break-word',
          props.mono && 'font-mono'
        )}
      >
        {props.value}
      </span>
    </div>
  )
}

function DetailSection(props: {
  icon: React.ReactNode
  iconTone?: IconBadgeTone
  label: string
  action?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <section className='min-w-0 space-y-1.5'>
      <div className='flex items-center justify-between gap-2'>
        <Label className='flex items-center gap-1.5 text-xs font-semibold'>
          <IconBadge tone={props.iconTone} size='xs'>
            {props.icon}
          </IconBadge>
          {props.label}
        </Label>
        {props.action}
      </div>
      <div className='bg-muted/30 min-w-0 space-y-1.5 overflow-hidden rounded-md border p-2.5 max-sm:p-2'>
        {props.children}
      </div>
    </section>
  )
}

function CopyButton(props: { copied: boolean; onCopy: () => void }) {
  const { t } = useTranslation()

  return (
    <Button
      type='button'
      variant='ghost'
      size='icon-xs'
      onClick={props.onCopy}
      title={t('Copy to clipboard')}
      aria-label={t('Copy to clipboard')}
    >
      {props.copied ? <Check className='text-success' /> : <Copy />}
    </Button>
  )
}

function stringifyData(value: unknown): string {
  if (value == null || value === '') return ''
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2)
    } catch {
      return value
    }
  }

  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

interface TaskDetailsDialogProps {
  log: TaskLog
  isAdmin: boolean
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function TaskDetailsDialog(props: TaskDetailsDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const { log, isAdmin } = props

  const duration = formatDuration(log.submit_time, log.finish_time, 'seconds')
  const videoUrl = resolveTaskVideoPreviewUrl(log)
  const taskData = useMemo(() => stringifyData(log.data), [log.data])
  const rawData = useMemo(
    () => (isAdmin ? stringifyData(log) : ''),
    [isAdmin, log]
  )
  const properties = log.properties
  const referenceDuration = properties?.reference_video_seconds
  const hasReferenceVideo =
    properties?.has_reference_video === true ||
    (typeof referenceDuration === 'number' && referenceDuration > 0)
  const videoDuration = log.video_duration ?? properties?.video_seconds
  const publicModel = log.model_name || properties?.origin_model_name
  const upstreamModel = properties?.upstream_model_name
  const resultUrl = (log.result_url || log.video_url || log.url || '').trim()

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        <>
          <IconBadge tone='info' size='sm'>
            <Info />
          </IconBadge>
          {t('Task Details')}
          <StatusBadge
            label={t(
              taskStatusMapper.getLabel(log.status, log.status || 'Submitting')
            )}
            variant={taskStatusMapper.getVariant(log.status)}
            size='sm'
            copyable={false}
          />
        </>
      }
      description={t('View the complete details for this task')}
      contentClassName='min-w-0 overflow-hidden max-sm:max-h-[calc(100dvh-1.5rem)] max-sm:w-[calc(100vw-1.5rem)] max-sm:max-w-[calc(100vw-1.5rem)] max-sm:p-4 sm:max-w-xl'
      headerClassName='max-sm:gap-1'
      titleClassName='flex items-center gap-2 text-base'
      descriptionClassName='sr-only'
      contentHeight='min(78dvh, 760px)'
      bodyClassName='pr-2 sm:pr-4'
    >
      <div className='w-full max-w-full min-w-0 space-y-3 overflow-x-hidden py-1'>
        <DetailSection icon={<Info />} iconTone='info' label={t('Overview')}>
          <DetailRow
            label={t('Task ID')}
            value={
              <span className='flex min-w-0 items-start gap-1'>
                <span className='min-w-0 break-all'>{log.task_id}</span>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-xs'
                  className='-my-1 shrink-0'
                  onClick={() => void copyToClipboard(log.task_id)}
                  title={t('Copy to clipboard')}
                  aria-label={t('Copy to clipboard')}
                >
                  {copiedText === log.task_id ? (
                    <Check className='text-success' />
                  ) : (
                    <Copy />
                  )}
                </Button>
              </span>
            }
            mono
          />
          {isAdmin && log.id > 0 ? (
            <DetailRow label={t('Internal ID')} value={String(log.id)} mono />
          ) : null}
          <DetailRow
            label={t('Platform')}
            value={getTaskPlatformName(log.platform) || '-'}
          />
          <DetailRow
            label={t('Action')}
            value={t(taskActionMapper.getLabel(log.action, log.action || '-'))}
          />
          <DetailRow label={t('Progress')} value={log.progress || '-'} mono />
          {publicModel ? (
            <DetailRow label={t('Model')} value={publicModel} mono />
          ) : null}
        </DetailSection>

        <DetailSection icon={<Clock3 />} iconTone='chart-2' label={t('Timing')}>
          <DetailRow
            label={t('Submit Time')}
            value={formatTimestampToDate(log.submit_time, 'seconds')}
            mono
          />
          {log.start_time ? (
            <DetailRow
              label={t('Start Time')}
              value={formatTimestampToDate(log.start_time, 'seconds')}
              mono
            />
          ) : null}
          {log.finish_time ? (
            <DetailRow
              label={t('Finish Time')}
              value={formatTimestampToDate(log.finish_time, 'seconds')}
              mono
            />
          ) : null}
          {duration ? (
            <DetailRow
              label={t('Duration')}
              value={`${duration.durationSec.toFixed(1)}s`}
              mono
            />
          ) : null}
          {typeof videoDuration === 'number' && videoDuration > 0 ? (
            <DetailRow
              label={t('Video Duration')}
              value={`${videoDuration}s`}
              mono
            />
          ) : null}
        </DetailSection>

        <DetailSection
          icon={<Video />}
          iconTone='chart-4'
          label={t('Reference video')}
        >
          <DetailRow
            label={t('Status')}
            value={hasReferenceVideo ? t('Yes') : t('No')}
          />
          {hasReferenceVideo &&
          typeof referenceDuration === 'number' &&
          referenceDuration > 0 ? (
            <DetailRow
              label={t('Duration')}
              value={`${referenceDuration}s`}
              mono
            />
          ) : null}
        </DetailSection>

        {isAdmin ? (
          <>
            <DetailSection
              icon={<ReceiptText />}
              iconTone='chart-3'
              label={t('Billing')}
            >
              <DetailRow
                label={t('Fee')}
                value={formatLogQuota(log.quota ?? 0)}
                mono
              />
              <DetailRow
                label={t('Refund')}
                value={formatLogQuota(log.refund_quota ?? 0)}
                mono
              />
              {log.group ? (
                <DetailRow label={t('Group')} value={log.group} mono />
              ) : null}
            </DetailSection>

            <DetailSection
              icon={<Route />}
              iconTone='chart-1'
              label={t('Request Properties')}
            >
              <DetailRow
                label={t('User')}
                value={
                  log.username
                    ? `${log.username} (#${log.user_id})`
                    : `#${log.user_id}`
                }
              />
              <DetailRow
                label={t('Channel')}
                value={
                  log.channel_name
                    ? `${log.channel_name} (#${log.channel_id})`
                    : `#${log.channel_id}`
                }
              />
              {upstreamModel ? (
                <DetailRow
                  label={t('Upstream Model Name')}
                  value={upstreamModel}
                  mono
                />
              ) : null}
            </DetailSection>
          </>
        ) : null}

        {log.fail_reason ? (
          <Alert variant='destructive'>
            <AlertTriangle />
            <AlertTitle>{t('Fail Reason')}</AlertTitle>
            <AlertDescription className='break-all whitespace-pre-wrap'>
              {log.fail_reason}
            </AlertDescription>
            <div className='absolute top-1.5 right-1.5'>
              <Button
                type='button'
                variant='ghost'
                size='icon-xs'
                onClick={() => void copyToClipboard(log.fail_reason || '')}
                title={t('Copy to clipboard')}
                aria-label={t('Copy to clipboard')}
              >
                {copiedText === log.fail_reason ? (
                  <Check className='text-success' />
                ) : (
                  <Copy />
                )}
              </Button>
            </div>
          </Alert>
        ) : null}

        {videoUrl || resultUrl ? (
          <DetailSection
            icon={<Video />}
            iconTone='chart-4'
            label={t('Result')}
          >
            {resultUrl ? (
              <DetailRow
                label={t('Result URL')}
                value={
                  <span className='flex min-w-0 items-start gap-1'>
                    <span className='min-w-0 break-all'>{resultUrl}</span>
                    <Button
                      type='button'
                      variant='ghost'
                      size='icon-xs'
                      className='-my-1 shrink-0'
                      onClick={() => void copyToClipboard(resultUrl)}
                      title={t('Copy to clipboard')}
                      aria-label={t('Copy to clipboard')}
                    >
                      {copiedText === resultUrl ? (
                        <Check className='text-success' />
                      ) : (
                        <Copy />
                      )}
                    </Button>
                  </span>
                }
                mono
              />
            ) : null}
            {videoUrl ? (
              <TaskVideoPreview
                taskId={log.task_id}
                sourceUrl={videoUrl}
                ownerUserId={isAdmin ? log.user_id : undefined}
              />
            ) : null}
          </DetailSection>
        ) : null}

        {taskData ? (
          <DetailSection
            icon={<FileJson2 />}
            iconTone='neutral'
            label={t('Upstream Response')}
            action={
              <CopyButton
                copied={copiedText === taskData}
                onCopy={() => void copyToClipboard(taskData)}
              />
            }
          >
            <pre className='bg-background/60 max-h-64 min-w-0 overflow-auto rounded border p-2 font-mono text-xs leading-relaxed whitespace-pre-wrap'>
              {taskData}
            </pre>
          </DetailSection>
        ) : null}

        {isAdmin && rawData ? (
          <DetailSection
            icon={<FileJson2 />}
            iconTone='neutral'
            label={t('Raw Data')}
            action={
              <CopyButton
                copied={copiedText === rawData}
                onCopy={() => void copyToClipboard(rawData)}
              />
            }
          >
            <pre className='bg-background/60 max-h-72 min-w-0 overflow-auto rounded border p-2 font-mono text-xs leading-relaxed whitespace-pre-wrap'>
              {rawData}
            </pre>
          </DetailSection>
        ) : null}
      </div>
    </Dialog>
  )
}
