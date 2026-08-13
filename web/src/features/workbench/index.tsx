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
import { useQuery, type UseQueryResult } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import { getChatDumpViewerUrl, getWorkbenchSummary } from './api'
import { ActionCenter } from './components/action-center'
import { ManagementAreas } from './components/management-areas'
import { OperatingEvidence } from './components/operating-evidence'
import { PartnerBrief } from './components/partner-brief'
import type { WorkbenchSummary, WorkbenchSummaryResponse } from './types'

const POLL_INTERVAL_MS = 60_000

export function WorkbenchBody({
  summary,
  onRetry,
  retrying,
  onOpenCaptures,
  openingCaptures,
}: {
  summary: WorkbenchSummary
  onRetry: () => void
  retrying: boolean
  onOpenCaptures: () => Promise<void>
  openingCaptures: boolean
}) {
  const dataSourceError = summary.status_bar.hub_db_error

  // 原文来自监控服务的数据源异常,里面是连接和查询细节。留给排障的人看控制台,
  // 页面上只说结论 —— 技术细节不进 UI。
  useEffect(() => {
    if (dataSourceError) {
      console.warn('[workbench] monitor data source failed:', dataSourceError)
    }
  }, [dataSourceError])

  return (
    <div className='space-y-3'>
      {/* 数据源挂了的时候,「先修数据源」这句结论和唯一能解决它的动作(重试)
          都在简报卡里 —— 不再在下面另起一条提示条把同一件事说第二遍。 */}
      <PartnerBrief summary={summary} onRetry={onRetry} retrying={retrying} />
      <div className='grid gap-3 lg:grid-cols-5'>
        <div className='lg:col-span-3'>
          <ActionCenter alarms={summary.alarms} />
        </div>
        <div className='lg:col-span-2'>
          <ManagementAreas
            statusBar={summary.status_bar}
            onOpenCaptures={onOpenCaptures}
            openingCaptures={openingCaptures}
          />
        </div>
      </div>
      <OperatingEvidence summary={summary} />
    </div>
  )
}

/** 内容区的三种状态:还在取数 / 取不到 / 正常。写成组件而不是多参数的渲染函数,
 *  调用点上每个入参都带着名字,加一个状态也不用再数第几个位置。 */
function WorkbenchContent({
  query,
  onOpenCaptures,
  openingCaptures,
}: {
  query: UseQueryResult<WorkbenchSummaryResponse>
  onOpenCaptures: () => Promise<void>
  openingCaptures: boolean
}) {
  const { t } = useTranslation()

  if (query.isLoading) {
    return (
      <div className='space-y-3'>
        <Skeleton className='h-28 w-full' />
        <div className='grid gap-3 lg:grid-cols-5'>
          <Skeleton className='h-72 lg:col-span-3' />
          <Skeleton className='h-72 lg:col-span-2' />
        </div>
        <Skeleton className='h-52 w-full' />
      </div>
    )
  }

  const response = query.data
  if (!response?.success || !response.data) {
    return (
      <ErrorState
        title={t('Workbench data service unavailable')}
        description={response?.message}
        onRetry={() => query.refetch()}
      />
    )
  }

  return (
    <WorkbenchBody
      summary={response.data}
      onRetry={() => query.refetch()}
      retrying={query.isFetching}
      onOpenCaptures={onOpenCaptures}
      openingCaptures={openingCaptures}
    />
  )
}

export function Workbench() {
  const { t } = useTranslation()
  const [openingCaptures, setOpeningCaptures] = useState(false)
  const query = useQuery({
    queryKey: ['workbench-summary'],
    queryFn: getWorkbenchSummary,
    refetchInterval: POLL_INTERVAL_MS,
  })

  const openCaptures = async () => {
    if (openingCaptures) return
    setOpeningCaptures(true)
    const opened = window.open('', '_blank')
    try {
      const url = await getChatDumpViewerUrl()
      if (!url) throw new Error('empty viewer url')
      if (opened) opened.location.href = url
      else window.location.href = url
    } catch {
      opened?.close()
      toast.error(t('Failed to open, please try again'))
    } finally {
      setOpeningCaptures(false)
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='inline-flex min-w-0 items-center gap-2'>
          <span className='truncate'>{t('Ops Workbench')}</span>
          <StatusBadge variant='neutral' className='shrink-0'>
            Root
          </StatusBadge>
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <Button
          variant='outline'
          size='sm'
          onClick={() => query.refetch()}
          disabled={query.isFetching}
        >
          <RefreshCw
            className={cn(query.isFetching && 'animate-spin')}
            aria-hidden='true'
          />
          {t('Refresh')}
        </Button>
      </SectionPageLayout.Actions>
      <SectionPageLayout.Content>
        <WorkbenchContent
          query={query}
          onOpenCaptures={openCaptures}
          openingCaptures={openingCaptures}
        />
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
