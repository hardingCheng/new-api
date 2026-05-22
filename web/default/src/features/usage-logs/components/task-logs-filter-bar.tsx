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
import { useState, useEffect, useCallback, useMemo } from 'react'
import { useQueryClient, useIsFetching, useQuery } from '@tanstack/react-query'
import { useNavigate, getRouteApi } from '@tanstack/react-router'
import { type Table } from '@tanstack/react-table'
import { Download, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useIsAdmin } from '@/hooks/use-admin'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { DataTableToolbar } from '@/components/data-table'
import { MultiSelect, type Option } from '@/components/multi-select'
import { searchUsers } from '@/features/users/api'
import { exportAllTaskLogs } from '../api'
import { TASK_STATUS, TASK_STATUS_MAPPINGS } from '../constants'
import { buildSearchParams } from '../lib/filter'
import { buildBaseParams, getDefaultTimeRange } from '../lib/utils'
import type { DrawingLogFilters, LogCategory, TaskLogFilters } from '../types'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'

const route = getRouteApi('/_authenticated/usage-logs/$section')

type TaskLikeLogCategory = Extract<LogCategory, 'drawing' | 'task'>
type TaskLogsFilters = DrawingLogFilters | TaskLogFilters
const taskStatusValues = Object.values(TASK_STATUS)

function splitMultiValue(value?: string): string[] {
  return String(value || '')
    .split(/[\s,，]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}

function joinMultiValue(values: string[]): string {
  return values.join(',')
}

interface TaskLogsFilterBarProps<TData> {
  table: Table<TData>
  logCategory: TaskLikeLogCategory
}

function getFilterValue(
  filters: TaskLogsFilters,
  logCategory: TaskLikeLogCategory
): string {
  if (logCategory === 'drawing') {
    return (filters as DrawingLogFilters).mjId || ''
  }
  return (filters as TaskLogFilters).taskId || ''
}

function setFilterValue(
  filters: TaskLogsFilters,
  logCategory: TaskLikeLogCategory,
  value: string
): TaskLogsFilters {
  if (logCategory === 'drawing') {
    return { ...filters, mjId: value }
  }
  return { ...filters, taskId: value }
}

export function TaskLogsFilterBar<TData>(props: TaskLogsFilterBarProps<TData>) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const isAdmin = useIsAdmin()
  const fetchingLogs = useIsFetching({ queryKey: ['logs'] })
  const { data: usersData } = useQuery({
    queryKey: ['task-log-filter-users'],
    queryFn: () => searchUsers({ p: 1, page_size: 100 }),
    enabled: isAdmin && props.logCategory === 'task',
    staleTime: 5 * 60 * 1000,
  })
  const { data: modelsData } = useQuery({
    queryKey: ['task-log-filter-models'],
    queryFn: async () => {
      const res = await api.get('/api/models')
      return res.data as {
        success: boolean
        data?: Record<string, string[]>
      }
    },
    enabled: isAdmin && props.logCategory === 'task',
    staleTime: 5 * 60 * 1000,
  })

  const [filters, setFilters] = useState<TaskLogsFilters>(() => {
    const { start, end } = getDefaultTimeRange()
    return { startTime: start, endTime: end }
  })
  const [exporting, setExporting] = useState(false)

  useEffect(() => {
    const { start, end } = getDefaultTimeRange()
    const baseFilters = {
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : start,
      endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
      ...(searchParams.channel
        ? { channel: String(searchParams.channel) }
        : {}),
    }
    const next: TaskLogsFilters =
      props.logCategory === 'drawing'
        ? {
            ...baseFilters,
            ...(searchParams.filter ? { mjId: searchParams.filter } : {}),
          }
        : {
            ...baseFilters,
            ...(searchParams.filter ? { taskId: searchParams.filter } : {}),
            ...(searchParams.modelNames
              ? { modelNames: String(searchParams.modelNames) }
              : {}),
            ...(searchParams.status ? { status: searchParams.status } : {}),
            ...(searchParams.reference
              ? { reference: searchParams.reference }
              : {}),
            ...(searchParams.userIds
              ? { userIds: String(searchParams.userIds) }
              : {}),
          }

    setFilters(next)
  }, [
    props.logCategory,
    searchParams.startTime,
    searchParams.endTime,
    searchParams.channel,
    searchParams.filter,
    searchParams.modelNames,
    searchParams.status,
    searchParams.reference,
    searchParams.userIds,
  ])

  const handleChange = useCallback(
    (field: keyof TaskLogsFilters, value: Date | string | undefined) => {
      setFilters((prev) => ({ ...prev, [field]: value }))
    },
    []
  )

  const handleApply = useCallback(() => {
    const filterParams = buildSearchParams(filters, props.logCategory)
    navigate({
      to: '/usage-logs/$section',
      params: { section: props.logCategory },
      search: {
        ...filterParams,
        page: 1,
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
  }, [filters, navigate, props.logCategory, queryClient])

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    const resetFilters: TaskLogsFilters = { startTime: start, endTime: end }
    setFilters(resetFilters)

    navigate({
      to: '/usage-logs/$section',
      params: { section: props.logCategory },
      search: {
        page: 1,
        startTime: start.getTime(),
        endTime: end.getTime(),
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
  }, [navigate, props.logCategory, queryClient])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') handleApply()
    },
    [handleApply]
  )

  const handleFilterChange = useCallback(
    (value: string) => {
      setFilters((prev) => setFilterValue(prev, props.logCategory, value))
    },
    [props.logCategory]
  )

  const handleExport = useCallback(async () => {
    if (!isAdmin || props.logCategory !== 'task') return

    const filterParams = buildSearchParams(filters, 'task')
    const baseParams = buildBaseParams({
      page: 1,
      pageSize: 50000,
      searchParams: filterParams,
      useMilliseconds: false,
    })

    setExporting(true)
    try {
      const { blob, filename } = await exportAllTaskLogs({
        ...baseParams,
        limit: 50000,
        task_id: filterParams.filter as string | undefined,
        user_ids: filterParams.userIds as string | undefined,
        model_names: filterParams.modelNames as string | undefined,
        status: filterParams.status as string | undefined,
        reference: filterParams.reference as string | undefined,
      })
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = filename || 'task-bills.csv'
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.URL.revokeObjectURL(url)
    } catch {
      toast.error(t('Export failed'))
    } finally {
      setExporting(false)
    }
  }, [filters, isAdmin, props.logCategory, t])

  const filterValue = getFilterValue(filters, props.logCategory)
  const taskFilters = filters as TaskLogFilters
  const selectedUserValues = useMemo(
    () => splitMultiValue(taskFilters.userIds),
    [taskFilters.userIds]
  )
  const selectedModelValues = useMemo(
    () => splitMultiValue(taskFilters.modelNames),
    [taskFilters.modelNames]
  )
  const userOptions = useMemo<Option[]>(
    () =>
      (usersData?.data?.items || []).map((user) => ({
        value: String(user.id),
        label: `${user.username} (#${user.id})`,
      })),
    [usersData?.data?.items]
  )
  const modelOptions = useMemo<Option[]>(() => {
    const values = Object.values(modelsData?.data || {})
      .flat()
      .map((model) => String(model || '').trim())
      .filter(Boolean)
    return Array.from(new Set(values))
      .sort((a, b) => a.localeCompare(b))
      .map((model) => ({ value: model, label: model }))
  }, [modelsData?.data])
  const placeholder =
    props.logCategory === 'drawing'
      ? t('Filter by Midjourney task ID')
      : t('Filter by task ID')
  const inputClass = 'w-full sm:w-[180px] lg:w-[200px]'
  const hasTaskFilters =
    props.logCategory === 'task' &&
    (!!taskFilters.status ||
      (isAdmin &&
        (!!taskFilters.modelNames ||
          !!taskFilters.reference ||
          !!taskFilters.userIds)))
  const hasAdditionalFilters =
    !!filterValue || !!filters.channel || hasTaskFilters

  return (
    <DataTableToolbar
      table={props.table}
      customSearch={
        <CompactDateTimeRangePicker
          start={filters.startTime}
          end={filters.endTime}
          onChange={({ start, end }) => {
            handleChange('startTime', start)
            handleChange('endTime', end)
          }}
          className='w-full sm:w-[340px]'
        />
      }
      additionalSearch={
        <>
          <Input
            aria-label={t('Task ID')}
            placeholder={placeholder}
            value={filterValue}
            onChange={(e) => handleFilterChange(e.target.value)}
            onKeyDown={handleKeyDown}
            className={inputClass}
          />
          {isAdmin && (
            <Input
              placeholder={t('Channel ID')}
              value={filters.channel || ''}
              onChange={(e) => handleChange('channel', e.target.value)}
              onKeyDown={handleKeyDown}
              className={inputClass}
            />
          )}
          {props.logCategory === 'task' && (
            <>
              {isAdmin && (
                <div className={inputClass}>
                  <MultiSelect
                    options={userOptions}
                    selected={selectedUserValues}
                    onChange={(values) =>
                      handleChange('userIds', joinMultiValue(values))
                    }
                    placeholder={t('User Filter')}
                    className='text-xs'
                    dropdownClassName='z-100'
                    maxVisible={1}
                    keepSelectedOptionsVisible
                    createOption={(value) => {
                      const trimmed = value.trim()
                      return trimmed ? { label: trimmed, value: trimmed } : null
                    }}
                  />
                </div>
              )}
              {isAdmin && (
                <div className={inputClass}>
                  <MultiSelect
                    options={modelOptions}
                    selected={selectedModelValues}
                    onChange={(values) =>
                      handleChange('modelNames', joinMultiValue(values))
                    }
                    placeholder={t('Model Name')}
                    className='text-xs'
                    dropdownClassName='z-100'
                    maxVisible={1}
                    keepSelectedOptionsVisible
                    createOption={(value) => {
                      const trimmed = value.trim()
                      return trimmed
                        ? { label: trimmed, value: trimmed }
                        : null
                    }}
                  />
                </div>
              )}
              <Select
                items={[
                  { value: 'all', label: t('All Statuses') },
                  ...taskStatusValues.map((status) => ({
                    value: status,
                    label: t(TASK_STATUS_MAPPINGS[status].label),
                  })),
                ]}
                value={taskFilters.status || ''}
                onValueChange={(value) => {
                  handleChange(
                    'status',
                    value && value !== 'all' ? value : undefined
                  )
                }}
              >
                <SelectTrigger className={inputClass}>
                  <SelectValue placeholder={t('Task Status')} />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  <SelectGroup>
                    <SelectItem value='all'>{t('All Statuses')}</SelectItem>
                    {taskStatusValues.map((status) => (
                      <SelectItem key={status} value={status}>
                        {t(TASK_STATUS_MAPPINGS[status].label)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              {isAdmin && (
                <Select
                  items={[
                    { value: 'with', label: t('Has Video Reference') },
                    { value: 'without', label: t('No Video Reference') },
                  ]}
                  value={taskFilters.reference || ''}
                  onValueChange={(value) => {
                    handleChange('reference', value || undefined)
                  }}
                >
                  <SelectTrigger className={inputClass}>
                    <SelectValue placeholder={t('Video Reference')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      <SelectItem value='with'>
                        {t('Has Video Reference')}
                      </SelectItem>
                      <SelectItem value='without'>
                        {t('No Video Reference')}
                      </SelectItem>
                    </SelectGroup>
                  </SelectContent>
                </Select>
              )}
            </>
          )}
        </>
      }
      hasAdditionalFilters={hasAdditionalFilters}
      preActions={
        isAdmin && props.logCategory === 'task' ? (
          <Button variant='outline' onClick={handleExport} disabled={exporting}>
            {exporting ? (
              <Loader2 className='animate-spin' />
            ) : (
              <Download />
            )}
            {t('Export Bill')}
          </Button>
        ) : undefined
      }
      onSearch={handleApply}
      searchLoading={fetchingLogs > 0}
      onReset={handleReset}
    />
  )
}
