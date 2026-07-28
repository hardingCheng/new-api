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
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { BarChart3, Loader2, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatLogQuota } from '@/lib/format'

import { getLogStatBreakdown } from '../api'
import { buildApiParams, getDefaultTimeRange } from '../lib/utils'
import type { LogStatBreakdownRow, LogStatDimension } from '../types'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'
import { useUsageLogsContext } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')
const DIMENSIONS: Array<{ value: LogStatDimension; label: string }> = [
  { value: 'channel', label: 'Channel' },
  { value: 'model', label: 'Model' },
  { value: 'user', label: 'User' },
]

function StatTotal(props: { label: string; value: string }) {
  return (
    <div className='bg-muted/35 min-w-0 rounded-md border px-3 py-2'>
      <div className='text-muted-foreground text-xs'>{props.label}</div>
      <div className='mt-0.5 truncate font-mono text-sm font-semibold tabular-nums'>
        {props.value}
      </div>
    </div>
  )
}

function getRowName(row: LogStatBreakdownRow, dimension: LogStatDimension) {
  if (row.name) return row.name
  if (dimension === 'channel' && row.channel_id != null) {
    return `#${row.channel_id}`
  }
  return '-'
}

export function UsageLogsStatSheet() {
  const { t } = useTranslation()
  const searchParams = route.useSearch()
  const { sensitiveVisible } = useUsageLogsContext()
  const [open, setOpen] = useState(false)
  const [dimension, setDimension] = useState<LogStatDimension>('channel')
  const [range, setRange] = useState(() => {
    const fallback = getDefaultTimeRange()
    return {
      start: searchParams.startTime
        ? new Date(searchParams.startTime)
        : fallback.start,
      end: searchParams.endTime ? new Date(searchParams.endTime) : fallback.end,
    }
  })

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      const fallback = getDefaultTimeRange()
      setRange({
        start: searchParams.startTime
          ? new Date(searchParams.startTime)
          : fallback.start,
        end: searchParams.endTime
          ? new Date(searchParams.endTime)
          : fallback.end,
      })
    }
    setOpen(nextOpen)
  }

  const query = useQuery({
    queryKey: [
      'usage-logs-stat-breakdown',
      dimension,
      searchParams,
      range.start.getTime(),
      range.end.getTime(),
    ],
    queryFn: async () => {
      try {
        const {
          p: _page,
          page_size: _pageSize,
          type: _type,
          ...filters
        } = buildApiParams({
          page: 1,
          pageSize: 1,
          searchParams: {
            ...searchParams,
            startTime: range.start.getTime(),
            endTime: range.end.getTime(),
          },
          isAdmin: true,
        })
        const result = await getLogStatBreakdown({ ...filters, dimension })
        if (!result.success) {
          toast.error(result.message || t('Failed to load statistics'))
          return { rows: [], total_consume: 0, total_refund: 0 }
        }
        return result.data ?? { rows: [], total_consume: 0, total_refund: 0 }
      } catch {
        toast.error(t('Failed to load statistics'))
        return { rows: [], total_consume: 0, total_refund: 0 }
      }
    },
    enabled: open,
  })

  const rows = query.data?.rows ?? []
  const totalConsume = query.data?.total_consume ?? 0
  const totalRefund = query.data?.total_refund ?? 0
  const mask = (value: number) =>
    sensitiveVisible ? formatLogQuota(value) : '••••'

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetTrigger
        render={<Button variant='ghost' size='sm' className='gap-1.5 px-2' />}
      >
        <BarChart3 />
        {t('Statistics')}
      </SheetTrigger>
      <SheetContent className='w-full sm:max-w-3xl'>
        <SheetHeader className='border-b'>
          <SheetTitle>{t('Quota Statistics')}</SheetTitle>
          <SheetDescription className='sr-only'>
            {t('Quota statistics by dimension')}
          </SheetDescription>
        </SheetHeader>

        <div className='grid grid-cols-1 gap-2 px-4 sm:grid-cols-3'>
          <StatTotal label={t('Total Usage')} value={mask(totalConsume)} />
          <StatTotal label={t('Total Refund')} value={mask(totalRefund)} />
          <StatTotal
            label={t('Total Revenue')}
            value={mask(totalConsume - totalRefund)}
          />
        </div>

        <div className='flex min-h-0 flex-1 flex-col gap-3 px-4 pb-4'>
          <CompactDateTimeRangePicker
            start={range.start}
            end={range.end}
            onChange={({ start, end }) =>
              setRange((current) => ({
                start: start ?? current.start,
                end: end ?? current.end,
              }))
            }
          />
          <div className='flex items-center justify-between gap-2'>
            <Tabs
              value={dimension}
              onValueChange={(value) => setDimension(value as LogStatDimension)}
            >
              <TabsList>
                {DIMENSIONS.map((item) => (
                  <TabsTrigger key={item.value} value={item.value}>
                    {t(item.label)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
            <Button
              variant='outline'
              size='icon-sm'
              onClick={() => void query.refetch()}
              disabled={query.isFetching}
              aria-label={t('Refresh')}
              title={t('Refresh')}
            >
              {query.isFetching ? (
                <Loader2 className='animate-spin' />
              ) : (
                <RefreshCw />
              )}
            </Button>
          </div>

          <div className='min-h-0 flex-1 overflow-auto rounded-md border'>
            <Table>
              <TableHeader className='bg-muted/45 sticky top-0 z-10'>
                <TableRow>
                  <TableHead>
                    {t(
                      DIMENSIONS.find((item) => item.value === dimension)
                        ?.label ?? 'Name'
                    )}
                  </TableHead>
                  {dimension === 'channel' && (
                    <TableHead>{t('Channel ID')}</TableHead>
                  )}
                  <TableHead className='text-right'>{t('Usage')}</TableHead>
                  <TableHead className='text-right'>{t('Refund')}</TableHead>
                  <TableHead className='text-right'>{t('Revenue')}</TableHead>
                  <TableHead className='text-right'>{t('Count')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((row) => (
                  <TableRow
                    key={`${dimension}-${row.channel_id ?? ''}-${row.name ?? ''}`}
                  >
                    <TableCell className='max-w-48 truncate font-medium'>
                      {sensitiveVisible ? getRowName(row, dimension) : '••••'}
                    </TableCell>
                    {dimension === 'channel' && (
                      <TableCell className='font-mono'>
                        {row.channel_id != null ? `#${row.channel_id}` : '-'}
                      </TableCell>
                    )}
                    <TableCell className='text-right font-mono'>
                      {mask(row.consume_quota || 0)}
                    </TableCell>
                    <TableCell className='text-right font-mono'>
                      {mask(row.refund_quota || 0)}
                    </TableCell>
                    <TableCell className='text-right font-mono'>
                      {mask((row.consume_quota || 0) - (row.refund_quota || 0))}
                    </TableCell>
                    <TableCell className='text-right font-mono'>
                      {row.count || 0}
                    </TableCell>
                  </TableRow>
                ))}
                {!query.isLoading && rows.length === 0 && (
                  <TableRow>
                    <TableCell
                      colSpan={dimension === 'channel' ? 6 : 5}
                      className='text-muted-foreground h-24 text-center'
                    >
                      {t('No data')}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
            {query.isLoading && (
              <div className='flex h-24 items-center justify-center'>
                <Loader2 className='text-muted-foreground animate-spin' />
              </div>
            )}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  )
}
