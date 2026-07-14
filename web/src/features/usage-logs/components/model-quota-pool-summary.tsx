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
import { ChevronRight, Gauge } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { api } from '@/lib/api'
import { formatQuota } from '@/lib/format'

import type { LogsViewScope } from './usage-logs-provider'

type QuotaPoolUsage = {
  scope: 'global' | 'user'
  metric: 'requests' | 'total_tokens' | 'prompt_tokens' | 'quota'
  period_key: string
  limit: number
  used: number
  remaining: number
  rule?: {
    id?: string
    model?: string
    period?: string
    metric?: string
    user_id?: number
    username?: string
    user_group?: string
  }
}

function formatMetric(
  value: number,
  metric: QuotaPoolUsage['metric'],
  t: (key: string) => string
) {
  if (metric === 'quota') return formatQuota(value)
  return `${value} ${metric === 'requests' ? t('requests') : 'tokens'}`
}

type ModelQuotaPoolSummaryProps = {
  scope: LogsViewScope
}

export function ModelQuotaPoolSummary(props: ModelQuotaPoolSummaryProps) {
  const { t } = useTranslation()
  const [pools, setPools] = useState<QuotaPoolUsage[]>([])
  const [open, setOpen] = useState(false)

  useEffect(() => {
    let active = true
    setPools([])
    void api
      .get<{ success: boolean; data?: QuotaPoolUsage[] }>(
        '/api/task/model_quota_pools',
        { params: { scope: props.scope } }
      )
      .then((response) => {
        if (
          active &&
          response.data.success &&
          Array.isArray(response.data.data)
        ) {
          setPools(response.data.data)
        }
      })
      .catch(() => {
        // The task log remains usable if the optional summary cannot load.
      })
    return () => {
      active = false
    }
  }, [props.scope])

  const summary = useMemo(() => {
    let lowestRemaining = 100
    let lowCount = 0
    pools.forEach((pool) => {
      const percent =
        pool.limit > 0
          ? Math.max(0, Math.round((pool.remaining / pool.limit) * 100))
          : 100
      lowestRemaining = Math.min(lowestRemaining, percent)
      if (percent <= 20) lowCount += 1
    })
    return { lowestRemaining, lowCount }
  }, [pools])

  if (!pools.length) return null

  return (
    <>
      <div className='flex max-w-full flex-wrap items-center gap-2 rounded-lg border px-3 py-2'>
        <Gauge className='text-primary size-4' />
        <span className='text-sm font-medium'>{t('Model Quota Pools')}</span>
        <Badge variant='secondary'>
          {t('{{count}} pools', { count: pools.length })}
        </Badge>
        {summary.lowCount ? (
          <Badge variant='destructive'>
            {t('{{count}} low', { count: summary.lowCount })}
          </Badge>
        ) : null}
        <span className='text-muted-foreground text-xs'>
          {t('Lowest remaining')}: {summary.lowestRemaining}%
        </span>
        <Button
          variant='ghost'
          size='sm'
          className='ml-auto'
          onClick={() => setOpen(true)}
        >
          {t('View details')}
          <ChevronRight data-icon='inline-end' />
        </Button>
      </div>

      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent className='sm:max-w-2xl'>
          <SheetHeader>
            <SheetTitle>{t('Model quota pool details')}</SheetTitle>
            <SheetDescription>{t('Details')}</SheetDescription>
          </SheetHeader>
          <div className='min-h-0 flex-1 space-y-2 overflow-y-auto px-4 pb-4'>
            {pools.map((pool) => {
              const metric =
                pool.metric === 'prompt_tokens' ? 'total_tokens' : pool.metric
              const usedPercent =
                pool.limit > 0
                  ? Math.min(100, Math.round((pool.used / pool.limit) * 100))
                  : 0
              return (
                <div
                  key={`${pool.rule?.id || pool.rule?.model}-${pool.scope}-${pool.period_key}`}
                  className='grid gap-2 rounded-lg border p-3'
                >
                  <div className='flex flex-wrap items-center gap-2'>
                    <span className='font-mono text-sm font-medium'>
                      {pool.rule?.model || '-'}
                    </span>
                    <Badge variant='outline'>
                      {pool.scope === 'user'
                        ? t('User pool')
                        : t('Global pool')}
                    </Badge>
                    <Badge variant='secondary'>{t(metric)}</Badge>
                    <span className='text-muted-foreground ml-auto text-xs'>
                      {t(pool.rule?.period || '')} / {pool.period_key}
                    </span>
                  </div>
                  {pool.scope === 'user' ? (
                    <p className='text-muted-foreground text-xs'>
                      {pool.rule?.user_id} / {pool.rule?.username || '-'} /{' '}
                      {pool.rule?.user_group || '-'}
                    </p>
                  ) : null}
                  <div className='flex items-center gap-3'>
                    <Progress value={usedPercent} className='min-w-0 flex-1' />
                    <span className='w-12 text-right text-xs tabular-nums'>
                      {usedPercent}%
                    </span>
                  </div>
                  <div className='text-muted-foreground flex flex-wrap justify-between gap-2 text-xs'>
                    <span>
                      {t('Remaining')}:{' '}
                      {formatMetric(pool.remaining, metric, t)}
                    </span>
                    <span>
                      {formatMetric(pool.used, metric, t)} /{' '}
                      {formatMetric(pool.limit, metric, t)}
                    </span>
                  </div>
                </div>
              )
            })}
          </div>
        </SheetContent>
      </Sheet>
    </>
  )
}
