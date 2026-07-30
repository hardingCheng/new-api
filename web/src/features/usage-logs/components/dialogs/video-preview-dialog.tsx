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
import { AlertTriangle, Loader2, RefreshCw, Video } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Alert, AlertAction, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { api } from '@/lib/api'

interface TaskVideoPreviewProps {
  taskId: string
  sourceUrl: string
}

export function TaskVideoPreview(props: TaskVideoPreviewProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [loading, setLoading] = useState(false)
  const [loadFailed, setLoadFailed] = useState(false)
  const [playbackFailed, setPlaybackFailed] = useState(false)
  const [videoUrl, setVideoUrl] = useState('')
  const abortControllerRef = useRef<AbortController | null>(null)
  const objectUrlRef = useRef('')

  const loadVideo = useCallback(async () => {
    abortControllerRef.current?.abort()
    if (objectUrlRef.current) {
      URL.revokeObjectURL(objectUrlRef.current)
      objectUrlRef.current = ''
    }

    setVideoUrl('')
    setLoadFailed(false)
    setPlaybackFailed(false)

    if (!props.sourceUrl.startsWith('/v1/videos/')) {
      setLoading(false)
      setVideoUrl(props.sourceUrl)
      return
    }

    const abortController = new AbortController()
    abortControllerRef.current = abortController
    setLoading(true)

    try {
      const response = await api.get<Blob>(props.sourceUrl, {
        responseType: 'blob',
        signal: abortController.signal,
        disableDuplicate: true,
        skipBusinessError: true,
        skipErrorHandler: true,
      })
      if (abortController.signal.aborted) return

      const objectUrl = URL.createObjectURL(response.data)
      objectUrlRef.current = objectUrl
      setVideoUrl(objectUrl)
      setLoading(false)
    } catch {
      if (abortController.signal.aborted) return
      setLoading(false)
      setLoadFailed(true)
    }
  }, [props.sourceUrl])

  useEffect(() => {
    return () => {
      abortControllerRef.current?.abort()
      if (objectUrlRef.current) URL.revokeObjectURL(objectUrlRef.current)
    }
  }, [])

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen)
    if (nextOpen) return

    abortControllerRef.current?.abort()
    if (objectUrlRef.current) {
      URL.revokeObjectURL(objectUrlRef.current)
      objectUrlRef.current = ''
    }
    setVideoUrl('')
    setLoading(false)
    setLoadFailed(false)
    setPlaybackFailed(false)
  }

  return (
    <>
      <Button
        type='button'
        variant='link'
        size='xs'
        className='h-auto px-0 text-xs'
        onClick={(event) => {
          event.stopPropagation()
          setOpen(true)
          void loadVideo()
        }}
      >
        <Video data-icon='inline-start' />
        {t('Click to preview video')}
      </Button>
      <Dialog
        open={open}
        onOpenChange={handleOpenChange}
        title={
          <>
            <IconBadge tone='chart-1' size='sm'>
              <Video />
            </IconBadge>
            {t('Video Preview')}
          </>
        }
        contentClassName='sm:max-w-3xl'
        titleClassName='flex items-center gap-2'
        contentHeight='auto'
      >
        {loading ? (
          <div
            className='bg-muted/30 flex aspect-video w-full items-center justify-center rounded-md border'
            aria-live='polite'
          >
            <Loader2 className='text-muted-foreground size-6 animate-spin' />
            <span className='sr-only'>{t('Loading...')}</span>
          </div>
        ) : null}

        {loadFailed || playbackFailed ? (
          <Alert variant='destructive'>
            <AlertTriangle />
            <AlertTitle>{t('Failed to load video')}</AlertTitle>
            <AlertAction>
              <Button
                type='button'
                variant='outline'
                size='xs'
                onClick={() => void loadVideo()}
              >
                <RefreshCw data-icon='inline-start' />
                {t('Retry')}
              </Button>
            </AlertAction>
          </Alert>
        ) : null}

        {videoUrl && !playbackFailed ? (
          <video
            key={videoUrl}
            src={videoUrl}
            controls
            preload='metadata'
            onError={() => setPlaybackFailed(true)}
            className='bg-background aspect-video max-h-[70dvh] w-full rounded-md border object-contain'
            aria-label={`${t('Video Preview')}: ${props.taskId}`}
          />
        ) : null}
      </Dialog>
    </>
  )
}
