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
import { useQuery, useQueryClient, useIsFetching } from '@tanstack/react-query'
import { useNavigate, getRouteApi } from '@tanstack/react-router'
import type { Table } from '@tanstack/react-table'
import { Eye, EyeOff } from 'lucide-react'
import { useState, useCallback, useDeferredValue, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { MultiSelect } from '@/components/multi-select'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getGroups, searchUsers } from '@/features/users/api'
import { useMediaQuery } from '@/hooks'
import { requireServerSuccess } from '@/lib/server-error-message'

import {
  LOG_TYPE_ALL_VALUE,
  LOG_TYPE_ENUM,
  LOG_TYPE_FILTERS,
} from '../constants'
import { buildSearchParams } from '../lib/filter'
import { getDefaultTimeRange } from '../lib/utils'
import type { CommonLogFilters } from '../types'
import { CommonLogsStats } from './common-logs-stats'
import { CompactDateTimeRangePicker } from './compact-date-time-range-picker'
import {
  LogsFilterField,
  LogsFilterInput,
  LogsFilterToolbar,
} from './logs-filter-toolbar'
import { useLogsViewScope, useUsageLogsContext } from './usage-logs-provider'
import { UsageLogsStatSheet } from './usage-logs-stat-sheet'

const route = getRouteApi('/_authenticated/usage-logs/$section')

type LogTypeValue = (typeof LOG_TYPE_FILTERS)[number]['value']
const logTypeValueSet = new Set<string>(
  LOG_TYPE_FILTERS.map((type) => type.value)
)

type CommonLogDraft = {
  sourceKey: string
  filters: CommonLogFilters
  logType: LogTypeValue
}

const ERROR_LOG_TYPE_VALUE = String(LOG_TYPE_ENUM.ERROR)

// 错误日志结局筛选:客户可见失败 vs 已被重试救回(仅类型=错误时展示)
const ERROR_OUTCOME_FILTERS = [
  { label: 'All Errors', value: 'all' },
  { label: 'Customer-Visible Failures', value: 'visible' },
  { label: 'Recovered by Retry', value: 'recovered' },
] as const

function isLogTypeValue(value: string): value is LogTypeValue {
  return logTypeValueSet.has(value)
}

function getLogTypeValue(value: unknown): LogTypeValue {
  return Array.isArray(value) &&
    value.length === 1 &&
    typeof value[0] === 'string' &&
    isLogTypeValue(value[0])
    ? value[0]
    : LOG_TYPE_ALL_VALUE
}

function buildSearchSourceKey(values: {
  startTime?: unknown
  endTime?: unknown
  channel?: unknown
  model?: unknown
  token?: unknown
  group?: unknown
  username?: unknown
  usernames?: unknown
  requestId?: unknown
  upstreamRequestId?: unknown
  errorOutcome?: unknown
  type?: unknown
}) {
  return [
    values.startTime,
    values.endTime,
    values.channel,
    values.model,
    values.token,
    values.group,
    values.username,
    values.usernames,
    values.requestId,
    values.upstreamRequestId,
    values.errorOutcome,
    Array.isArray(values.type) ? values.type.join(',') : values.type,
  ]
    .map((value) => String(value ?? ''))
    .join('\u001f')
}

interface CommonLogsFilterBarProps<TData> {
  table: Table<TData>
}

export function CommonLogsFilterBar<TData>(
  props: CommonLogsFilterBarProps<TData>
) {
  const { t } = useTranslation()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const searchParams = route.useSearch()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const { sensitiveVisible, setSensitiveVisible } = useUsageLogsContext()
  const fetchingLogs = useIsFetching({ queryKey: ['logs'] })
  const { data: adminGroups } = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()),
    enabled: isAdmin,
  })
  // 分组筛选只在管理员视图出现(普通用户日志不返回分组)
  const groupOptions = useMemo(
    () =>
      (adminGroups?.data ?? [])
        .filter((group) => group !== 'auto')
        .map((group) => ({ label: group, value: group })),
    [adminGroups]
  )

  const searchState = useMemo<CommonLogDraft>(() => {
    const { start, end } = getDefaultTimeRange()
    const sourceValues = {
      startTime: searchParams.startTime,
      endTime: searchParams.endTime,
      channel: searchParams.channel,
      model: searchParams.model,
      token: searchParams.token,
      group: isAdmin ? searchParams.group : undefined,
      username: searchParams.username,
      usernames: searchParams.usernames,
      requestId: searchParams.requestId,
      upstreamRequestId: searchParams.upstreamRequestId,
      errorOutcome: searchParams.errorOutcome,
      type: searchParams.type,
    }
    const filters: CommonLogFilters = {
      startTime: searchParams.startTime
        ? new Date(searchParams.startTime)
        : start,
      endTime: searchParams.endTime ? new Date(searchParams.endTime) : end,
      channel: searchParams.channel || undefined,
      model: searchParams.model || undefined,
      token: searchParams.token || undefined,
      group: isAdmin ? searchParams.group || undefined : undefined,
      username: searchParams.username || undefined,
      usernames: searchParams.usernames
        ? searchParams.usernames.split(',').filter(Boolean)
        : undefined,
      requestId: searchParams.requestId || undefined,
      upstreamRequestId: searchParams.upstreamRequestId || undefined,
      errorOutcome: isAdmin
        ? searchParams.errorOutcome || undefined
        : undefined,
    }
    return {
      sourceKey: buildSearchSourceKey(sourceValues),
      filters,
      logType: getLogTypeValue(searchParams.type),
    }
  }, [
    searchParams.startTime,
    searchParams.endTime,
    searchParams.channel,
    searchParams.model,
    searchParams.token,
    searchParams.group,
    searchParams.username,
    searchParams.usernames,
    searchParams.requestId,
    searchParams.upstreamRequestId,
    searchParams.errorOutcome,
    searchParams.type,
    isAdmin,
  ])
  const [draft, setDraft] = useState<CommonLogDraft>(() => searchState)
  const [usernameSearch, setUsernameSearch] = useState('')
  const deferredUsernameSearch = useDeferredValue(usernameSearch.trim())
  const { data: usernameSearchResult, isFetching: isSearchingUsers } = useQuery(
    {
      queryKey: ['usage-logs-user-search', deferredUsernameSearch],
      queryFn: () =>
        searchUsers({
          keyword: deferredUsernameSearch,
          p: 1,
          page_size: 20,
        }),
      enabled: isAdmin && deferredUsernameSearch.length > 0,
      staleTime: 30_000,
    }
  )
  const activeDraft =
    draft.sourceKey === searchState.sourceKey ? draft : searchState
  const filters = activeDraft.filters
  const logType = activeDraft.logType

  const handleChange = useCallback(
    (
      field: keyof CommonLogFilters,
      value: Date | string | string[] | undefined
    ) => {
      setDraft((current) => {
        const base =
          current.sourceKey === searchState.sourceKey ? current : searchState
        return {
          sourceKey: searchState.sourceKey,
          filters: { ...base.filters, [field]: value },
          logType: base.logType,
        }
      })
    },
    [searchState]
  )

  const handleApply = useCallback(
    (nextFilters: CommonLogFilters = filters) => {
      const filterParams = buildSearchParams(nextFilters, 'common')
      navigate({
        to: '/usage-logs/$section',
        params: { section: 'common' },
        search: {
          ...filterParams,
          type: [logType],
          page: 1,
        },
      })
      queryClient.invalidateQueries({ queryKey: ['logs'] })
      queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
    },
    [filters, logType, navigate, queryClient]
  )

  const handleReset = useCallback(() => {
    const { start, end } = getDefaultTimeRange()
    const resetFilters: CommonLogFilters = { startTime: start, endTime: end }
    const resetSearch = {
      type: [LOG_TYPE_ALL_VALUE],
      startTime: start.getTime(),
      endTime: end.getTime(),
    }
    setDraft({
      sourceKey: buildSearchSourceKey(resetSearch),
      filters: resetFilters,
      logType: LOG_TYPE_ALL_VALUE,
    })

    navigate({
      to: '/usage-logs/$section',
      params: { section: 'common' },
      search: {
        page: 1,
        ...resetSearch,
      },
    })
    queryClient.invalidateQueries({ queryKey: ['logs'] })
    queryClient.invalidateQueries({ queryKey: ['usage-logs-stats'] })
  }, [navigate, queryClient])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') handleApply()
    },
    [handleApply]
  )

  const hasExpandedFilters =
    !!filters.token ||
    !!filters.username ||
    !!filters.usernames?.length ||
    !!filters.channel ||
    !!filters.requestId ||
    !!filters.upstreamRequestId

  const hasTypeFilter = logType !== LOG_TYPE_ALL_VALUE
  const hasAdditionalFilters =
    !!filters.model ||
    (isAdmin && !!filters.group) ||
    hasTypeFilter ||
    hasExpandedFilters

  const expandedFilterCount = [
    filters.token,
    isAdmin ? filters.username || filters.usernames?.length : undefined,
    isAdmin ? filters.channel : undefined,
    filters.requestId,
    filters.upstreamRequestId,
  ].filter(Boolean).length
  const sensitiveInputClass = sensitiveVisible
    ? undefined
    : '[-webkit-text-security:disc]'
  const logTypeItems = useMemo(
    () =>
      LOG_TYPE_FILTERS.map((type) => ({
        value: type.value,
        label: t(type.label),
        deprecated: type.deprecated,
      })),
    [t]
  )
  const selectedLogType = logTypeItems.find((type) => type.value === logType)
  const deprecatedTypeDescription = t(
    'Only used to find historical logs. New records are available in Audit Logs.'
  )

  const statsBar = <CommonLogsStats />
  const sensitiveToggle = (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            onClick={() => setSensitiveVisible(!sensitiveVisible)}
            aria-label={sensitiveVisible ? t('Hide') : t('Show')}
            className='text-muted-foreground hover:text-foreground size-7 max-sm:size-11'
          />
        }
      >
        {sensitiveVisible ? <Eye /> : <EyeOff />}
      </TooltipTrigger>
      <TooltipContent>
        {sensitiveVisible ? t('Hide') : t('Show')}
      </TooltipContent>
    </Tooltip>
  )
  const toolbarActions = (
    <div className='flex items-center gap-1'>
      {isAdmin && <UsageLogsStatSheet />}
      {sensitiveToggle}
    </div>
  )

  const dateRangeFilter = (
    <LogsFilterField wide>
      <CompactDateTimeRangePicker
        start={filters.startTime}
        end={filters.endTime}
        onChange={({ start, end }) => {
          handleChange('startTime', start)
          handleChange('endTime', end)
          if (isMobile) {
            handleApply({ ...filters, startTime: start, endTime: end })
          }
        }}
      />
    </LogsFilterField>
  )
  const modelFilter = (
    <LogsFilterField>
      <LogsFilterInput
        placeholder={t('Model Name')}
        value={filters.model || ''}
        onChange={(e) => handleChange('model', e.target.value)}
        onKeyDown={handleKeyDown}
      />
    </LogsFilterField>
  )
  const groupFilter = (
    <LogsFilterField className={sensitiveInputClass}>
      <Combobox
        options={groupOptions}
        allowCustomValue
        aria-label={t('Group')}
        emptyText={t('No group found.')}
        placeholder={t('Group')}
        className='h-8 min-w-0 text-sm leading-5'
        popupClassName={sensitiveInputClass}
        value={filters.group || ''}
        onValueChange={(value) => handleChange('group', value ?? '')}
        onKeyDown={handleKeyDown}
      />
    </LogsFilterField>
  )
  const typeFilter = (
    <LogsFilterField>
      <Select
        items={logTypeItems}
        value={logType}
        onValueChange={(value) => {
          const nextLogType =
            value !== null && isLogTypeValue(value) ? value : LOG_TYPE_ALL_VALUE
          setDraft((current) => {
            const base =
              current.sourceKey === searchState.sourceKey
                ? current
                : searchState
            return {
              sourceKey: searchState.sourceKey,
              // 结局筛选只对错误类型有意义,切走时一并清掉
              filters:
                nextLogType === ERROR_LOG_TYPE_VALUE
                  ? base.filters
                  : { ...base.filters, errorOutcome: undefined },
              logType: nextLogType,
            }
          })
        }}
      >
        <SelectTrigger
          aria-label={t('Type')}
          aria-description={
            selectedLogType?.deprecated ? deprecatedTypeDescription : undefined
          }
        >
          <SelectValue className='min-w-0'>
            <span className='truncate'>
              {selectedLogType?.label ?? t('All Types')}
            </span>
            {selectedLogType?.deprecated && (
              <Badge
                variant='secondary'
                className='h-4 px-1.5 text-[10px] font-normal'
                title={deprecatedTypeDescription}
              >
                {t('Deprecated')}
              </Badge>
            )}
          </SelectValue>
        </SelectTrigger>
        <SelectContent
          alignItemWithTrigger={false}
          className='max-w-[calc(100vw-2rem)] min-w-52'
        >
          <SelectGroup>
            {LOG_TYPE_FILTERS.map((type) => (
              <SelectItem
                key={type.value}
                value={type.value}
                className='[&_[data-slot=select-item-text]]:items-center'
                aria-description={
                  type.deprecated ? deprecatedTypeDescription : undefined
                }
              >
                {t(type.label)}
                {type.deprecated && (
                  <Badge
                    variant='secondary'
                    className='h-4 px-1.5 text-[10px] font-normal'
                    title={deprecatedTypeDescription}
                  >
                    {t('Deprecated')}
                  </Badge>
                )}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </LogsFilterField>
  )
  const errorOutcomeValue = filters.errorOutcome || 'all'
  const errorOutcomeLabel = t(
    ERROR_OUTCOME_FILTERS.find((outcome) => outcome.value === errorOutcomeValue)
      ?.label ?? 'All Errors'
  )
  const errorOutcomeFilter =
    isAdmin && logType === ERROR_LOG_TYPE_VALUE ? (
      <LogsFilterField>
        <Select
          items={ERROR_OUTCOME_FILTERS.map((outcome) => ({
            value: outcome.value,
            label: t(outcome.label),
          }))}
          value={errorOutcomeValue}
          onValueChange={(value) => {
            handleChange(
              'errorOutcome',
              value === 'visible' || value === 'recovered' ? value : undefined
            )
          }}
        >
          <SelectTrigger>
            <SelectValue>{errorOutcomeLabel}</SelectValue>
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {ERROR_OUTCOME_FILTERS.map((outcome) => (
                <SelectItem key={outcome.value} value={outcome.value}>
                  {t(outcome.label)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </LogsFilterField>
    ) : null
  const advancedFilters = (
    <>
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Token Name')}
          className={sensitiveInputClass}
          value={filters.token || ''}
          onChange={(e) => handleChange('token', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
      {isAdmin && (
        <LogsFilterField>
          <LogsFilterInput
            placeholder={t('Username')}
            className={sensitiveInputClass}
            value={filters.username || ''}
            onChange={(e) => {
              handleChange('username', e.target.value)
              if (filters.usernames?.length) {
                handleChange('usernames', undefined)
              }
            }}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
      )}
      {isAdmin && (
        <LogsFilterField wide>
          <MultiSelect
            options={(usernameSearchResult?.data?.items ?? []).map((user) => ({
              value: user.username,
              label: user.username,
            }))}
            selected={filters.usernames ?? []}
            onChange={(usernames) => {
              handleChange(
                'usernames',
                usernames.length ? usernames : undefined
              )
              if (usernames.length && filters.username) {
                handleChange('username', undefined)
              }
            }}
            onSearchChange={setUsernameSearch}
            placeholder={t('Select users')}
            emptyText={
              isSearchingUsers ? t('Searching...') : t('No matching users')
            }
            maxVisibleChips={1}
            className='min-h-8 py-0 text-sm'
          />
        </LogsFilterField>
      )}
      {isAdmin && (
        <LogsFilterField>
          <LogsFilterInput
            placeholder={t('Channel ID')}
            value={filters.channel || ''}
            onChange={(e) => handleChange('channel', e.target.value)}
            onKeyDown={handleKeyDown}
          />
        </LogsFilterField>
      )}
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Request ID')}
          value={filters.requestId || ''}
          onChange={(e) => handleChange('requestId', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
      <LogsFilterField>
        <LogsFilterInput
          placeholder={t('Upstream Request ID')}
          value={filters.upstreamRequestId || ''}
          onChange={(e) => handleChange('upstreamRequestId', e.target.value)}
          onKeyDown={handleKeyDown}
        />
      </LogsFilterField>
    </>
  )

  return (
    <LogsFilterToolbar
      table={props.table}
      compactMobile
      stats={statsBar}
      actionStart={toolbarActions}
      primaryFilters={
        <>
          {dateRangeFilter}
          {modelFilter}
          {isAdmin ? groupFilter : null}
          {typeFilter}
          {errorOutcomeFilter}
        </>
      }
      advancedFilters={advancedFilters}
      mobilePinnedFilters={dateRangeFilter}
      mobileFilters={
        <>
          {modelFilter}
          {isAdmin ? groupFilter : null}
          {typeFilter}
          {errorOutcomeFilter}
          {advancedFilters}
        </>
      }
      mobileFilterCount={
        [
          filters.model,
          isAdmin ? filters.group : undefined,
          hasTypeFilter,
        ].filter(Boolean).length + expandedFilterCount
      }
      hasAdvancedActiveFilters={hasExpandedFilters}
      advancedFilterCount={expandedFilterCount}
      hasActiveFilters={hasAdditionalFilters}
      onSearch={() => handleApply()}
      searchLoading={fetchingLogs > 0}
      onReset={handleReset}
    />
  )
}
