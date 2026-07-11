import { useQueryClient } from '@tanstack/react-query'
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
import { Edit, Plus, RefreshCw, Save, Trash2, Unplug } from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { TableCell, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { TitledCard } from '@/components/ui/titled-card'
import { api } from '@/lib/api'

import { updateSystemOption } from '../api'
import { EditorField, RuleTable } from '../billing/rule-editor-ui'
import type { OperationsSettings } from '../types'
import { ChannelBreakerRuleDialog } from './channel-breaker-rule-dialog'
import {
  BREAKER_TEMPLATES,
  DEFAULT_BREAKER_RULE,
  normalizeBreakerRule,
  type BreakerHistory,
  type BreakerRule,
  type BreakerStatus,
} from './channel-breaker-types'

const CONFIG_KEYS = [
  'ChannelBreakerEnabled',
  'ChannelBreakerFailureLimit',
  'ChannelBreakerCooldownSeconds',
  'ChannelBreakerProbeCount',
  'ChannelBreakerProbeSuccessCount',
  'ChannelBreakerExcludePaths',
  'ChannelBreakerFailureStatusCodes',
  'AutomaticDisableKeywords',
  'monitor_setting.bark_alert_enabled',
  'monitor_setting.bark_alert_url',
  'monitor_setting.bark_alert_volume',
  'monitor_setting.low_balance_alert_enabled',
  'monitor_setting.low_balance_threshold_cny',
  'monitor_setting.low_balance_alert_sound',
  'monitor_setting.channel_breaker_alert_enabled',
  'monitor_setting.channel_breaker_alert_sound',
  'monitor_setting.channel_disable_alert_enabled',
  'monitor_setting.channel_disable_alert_sound',
  'monitor_setting.channel_disable_alert_cooldown_second',
  'monitor_setting.retest_disabled_channel_enabled',
  'monitor_setting.retest_disabled_channel_seconds',
] as const

type ConfigKey = (typeof CONFIG_KEYS)[number]
type BreakerConfig = Pick<OperationsSettings, ConfigKey>

type ChannelChoice = { id: number; name: string }

const BARK_SOUNDS = [
  'alarm',
  'bell',
  'birdsong',
  'bloom',
  'calypso',
  'chime',
  'electronic',
  'fanfare',
  'glass',
  'horn',
  'ladder',
  'minuet',
  'newsflash',
  'noir',
  'paymentsuccess',
  'shake',
  'suspense',
  'telegraph',
  'typewriters',
  'update',
  'silence',
]

const BUILTIN_BREAKER_RULE = normalizeBreakerRule({
  id: '__builtin_instant_disable__',
  name: 'Disable on upstream balance exhaustion',
  scope: 'global',
  targets: [],
  instant_disable_enabled: true,
  instant_disable_status_codes: '403',
  instant_disable_keywords:
    'insufficient account balance\ninsufficient user quota\ninsufficient_user_quota\n预扣费额度失败',
})

function pickConfig(settings: OperationsSettings): BreakerConfig {
  return Object.fromEntries(
    CONFIG_KEYS.map((key) => [key, settings[key]])
  ) as BreakerConfig
}

function parseRules(raw: string): BreakerRule[] {
  try {
    const parsed: unknown = JSON.parse(raw || '[]')
    return Array.isArray(parsed)
      ? parsed.map((rule) => normalizeBreakerRule(rule as Partial<BreakerRule>))
      : []
  } catch {
    return []
  }
}

function parseIds(raw: string): number[] {
  try {
    const parsed: unknown = JSON.parse(raw || '[]')
    return Array.isArray(parsed)
      ? parsed.map(Number).filter((id) => Number.isInteger(id) && id > 0)
      : []
  } catch {
    return []
  }
}

function validStatusCodes(value: string) {
  return value
    .split(',')
    .map((token) => token.trim())
    .filter(Boolean)
    .every((token) => {
      const match = token.match(/^(\d{3})(?:-(\d{3}))?$/)
      if (!match) return false
      const start = Number(match[1])
      const end = Number(match[2] ?? match[1])
      return start >= 100 && end <= 599 && start <= end
    })
}

function formatTime(value?: string | number) {
  if (!value) return '-'
  const date = new Date(typeof value === 'number' ? value * 1000 : value)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}

export function ChannelBreakerSection(props: {
  defaultValues: OperationsSettings
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [config, setConfig] = useState(() => pickConfig(props.defaultValues))
  const [rules, setRules] = useState(() =>
    parseRules(props.defaultValues.ChannelBreakerRules)
  )
  const [exemptIds, setExemptIds] = useState(() =>
    parseIds(props.defaultValues.ChannelBreakerExemptChannels)
  )
  const [channelNames, setChannelNames] = useState<Record<number, string>>({})
  const [channelKeyword, setChannelKeyword] = useState('')
  const [channelResults, setChannelResults] = useState<ChannelChoice[]>([])
  const [statuses, setStatuses] = useState<BreakerStatus[]>([])
  const [history, setHistory] = useState<BreakerHistory[]>([])
  const [historyPage, setHistoryPage] = useState(1)
  const [historyTotal, setHistoryTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [ruleDialogOpen, setRuleDialogOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<BreakerRule | null>(null)

  useEffect(() => {
    setConfig(pickConfig(props.defaultValues))
    setRules(parseRules(props.defaultValues.ChannelBreakerRules))
    setExemptIds(parseIds(props.defaultValues.ChannelBreakerExemptChannels))
  }, [props.defaultValues])

  const fetchStatuses = useCallback(async () => {
    try {
      const response = await api.get<{
        success: boolean
        message?: string
        data?: { items?: BreakerStatus[] }
      }>('/api/option/channel_breaker/statuses')
      if (!response.data.success) {
        toast.error(response.data.message || t('Failed to load'))
        return
      }
      setStatuses(response.data.data?.items ?? [])
    } catch {
      toast.error(t('Failed to load'))
    }
  }, [t])

  const fetchHistory = useCallback(
    async (page: number) => {
      try {
        const response = await api.get<{
          success: boolean
          message?: string
          data?: { items?: BreakerHistory[]; total?: number }
        }>('/api/option/channel_breaker/logs', {
          params: { page, page_size: 10 },
        })
        if (!response.data.success) {
          toast.error(response.data.message || t('Failed to load'))
          return
        }
        setHistory(response.data.data?.items ?? [])
        setHistoryTotal(response.data.data?.total ?? 0)
        setHistoryPage(page)
      } catch {
        toast.error(t('Failed to load'))
      }
    },
    [t]
  )

  const fetchExemptChannels = useCallback(async () => {
    try {
      const response = await api.get<{
        success: boolean
        data?: ChannelChoice[]
      }>('/api/channel/breaker_exempt')
      const channels = response.data.data
      if (!response.data.success || !Array.isArray(channels)) return
      setChannelNames((current) => ({
        ...current,
        ...Object.fromEntries(
          channels.map((channel) => [channel.id, channel.name])
        ),
      }))
    } catch {
      // IDs remain editable even when names cannot be loaded.
    }
  }, [])

  useEffect(() => {
    void Promise.all([fetchStatuses(), fetchHistory(1), fetchExemptChannels()])
  }, [fetchExemptChannels, fetchHistory, fetchStatuses])

  const searchChannels = async () => {
    try {
      const response = await api.get<{
        success: boolean
        message?: string
        data?: { items?: ChannelChoice[] } | ChannelChoice[]
      }>('/api/channel/search', {
        params: {
          keyword: channelKeyword,
          group: '',
          model: '',
          p: 1,
          page_size: 20,
        },
      })
      if (!response.data.success) {
        toast.error(response.data.message || t('Failed to load'))
        return
      }
      const data = response.data.data
      const items = Array.isArray(data) ? data : (data?.items ?? [])
      setChannelResults(items)
      setChannelNames((current) => ({
        ...current,
        ...Object.fromEntries(
          items.map((channel) => [channel.id, channel.name])
        ),
      }))
    } catch {
      toast.error(t('Failed to load'))
    }
  }

  const saveConfig = async () => {
    if (!validStatusCodes(config.ChannelBreakerFailureStatusCodes)) {
      toast.error(t('Invalid circuit breaker status code list'))
      return
    }
    if (
      Number(config.ChannelBreakerProbeSuccessCount) >
      Number(config.ChannelBreakerProbeCount)
    ) {
      toast.error(t('Required probe successes cannot exceed probe requests'))
      return
    }
    setLoading(true)
    try {
      const responses = await Promise.all(
        CONFIG_KEYS.map((key) =>
          updateSystemOption({ key, value: config[key] })
        )
      )
      const failed = responses.find((response) => !response.success)
      if (failed) throw new Error(failed.message || t('Failed to save'))
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Saved successfully'))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Failed to save'))
    } finally {
      setLoading(false)
    }
  }

  const saveRules = async () => {
    setLoading(true)
    try {
      const response = await updateSystemOption({
        key: 'ChannelBreakerRules',
        value: JSON.stringify(rules, null, 2),
      })
      if (!response.success) throw new Error(response.message)
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Saved successfully'))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Failed to save'))
    } finally {
      setLoading(false)
    }
  }

  const saveExemptions = async () => {
    setLoading(true)
    try {
      const response = await updateSystemOption({
        key: 'ChannelBreakerExemptChannels',
        value: JSON.stringify(exemptIds),
      })
      if (!response.success) throw new Error(response.message)
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Saved successfully'))
      await fetchExemptChannels()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Failed to save'))
    } finally {
      setLoading(false)
    }
  }

  const clearStatus = async (status: BreakerStatus) => {
    try {
      const response = await api.post<{ success: boolean; message?: string }>(
        '/api/option/channel_breaker/clear',
        { state_key: status.state_key }
      )
      if (!response.data.success) throw new Error(response.data.message)
      toast.success(t('Cleared'))
      await fetchStatuses()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Failed to load'))
    }
  }

  const saveEditedRule = (rule: BreakerRule) => {
    setRules((current) => {
      const index = current.findIndex((item) => item.id === rule.id)
      if (index < 0) return [...current, rule]
      return current.map((item, itemIndex) =>
        itemIndex === index ? rule : item
      )
    })
  }

  const addTemplate = (template: (typeof BREAKER_TEMPLATES)[number]) => {
    setRules((current) => [
      ...current,
      normalizeBreakerRule({
        ...DEFAULT_BREAKER_RULE,
        ...template.rule,
        id: '',
        name: template.label,
      }),
    ])
  }

  const statusCountText = useMemo(
    () => t('{{count}} active breakers', { count: statuses.length }),
    [statuses.length, t]
  )

  return (
    <div className='grid gap-4'>
      <TitledCard
        title={t('Channel Circuit Breaker')}
        action={
          <Badge variant={statuses.length ? 'destructive' : 'secondary'}>
            {statusCountText}
          </Badge>
        }
      />

      <Tabs defaultValue='status'>
        <TabsList className='max-w-full flex-wrap justify-start group-data-horizontal/tabs:h-auto'>
          <TabsTrigger value='status'>{t('Status')}</TabsTrigger>
          <TabsTrigger value='rules'>{t('Rules')}</TabsTrigger>
          <TabsTrigger value='exemptions'>{t('Exempt Channels')}</TabsTrigger>
          <TabsTrigger value='settings'>{t('Settings')}</TabsTrigger>
        </TabsList>

        <TabsContent value='status' className='grid gap-4 pt-4'>
          <TitledCard
            title={t('Current breaker status')}
            action={
              <Button variant='outline' onClick={() => void fetchStatuses()}>
                <RefreshCw data-icon='inline-start' />
                {t('Refresh')}
              </Button>
            }
            contentClassName='p-0 sm:p-0'
          >
            <RuleTable
              headers={[
                t('Channel'),
                t('Target'),
                t('State'),
                t('Probe progress'),
                t('Cooldown'),
                t('Rule'),
                t('Actions'),
              ]}
              empty={statuses.length === 0}
              emptyText={t('No data')}
            >
              {statuses.map((status) => (
                <TableRow key={status.state_key}>
                  <TableCell className='whitespace-nowrap'>
                    {status.channel_name || `#${status.channel_id}`}
                  </TableCell>
                  <TableCell className='font-mono text-xs'>
                    {status.key_hash || t('Whole channel')}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        status.state === 'open' ? 'destructive' : 'secondary'
                      }
                    >
                      {status.state === 'half-open' ? t('Probing') : t('Open')}
                    </Badge>
                  </TableCell>
                  <TableCell className='whitespace-nowrap'>
                    {status.probe_success || 0}/
                    {status.rule_probe_success_count ||
                      config.ChannelBreakerProbeSuccessCount}{' '}
                    / {status.probe_total || 0}/
                    {status.rule_probe_count || config.ChannelBreakerProbeCount}
                  </TableCell>
                  <TableCell>
                    {status.state === 'half-open'
                      ? t('Probing')
                      : `${status.cooldown_remaining_seconds || 0}s`}
                  </TableCell>
                  <TableCell>
                    {status.rule_name || '-'}
                    {status.group || status.model ? (
                      <span className='text-muted-foreground block text-xs'>
                        {[status.group, status.model]
                          .filter(Boolean)
                          .join(' / ')}
                      </span>
                    ) : null}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => void clearStatus(status)}
                    >
                      <Unplug data-icon='inline-start' />
                      {t('Clear')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </RuleTable>
          </TitledCard>

          <TitledCard
            title={t('Circuit breaker history')}
            action={
              <Button
                variant='outline'
                onClick={() => void fetchHistory(historyPage)}
              >
                <RefreshCw data-icon='inline-start' />
                {t('Refresh')}
              </Button>
            }
            contentClassName='p-0 sm:p-0'
          >
            <RuleTable
              headers={[
                t('Time'),
                t('Type'),
                t('Channel'),
                t('Model'),
                t('Group'),
                t('Rule'),
                t('Failures'),
                t('Cooldown'),
              ]}
              empty={history.length === 0}
              emptyText={t('No data')}
            >
              {history.map((item) => {
                const instant = item.reason?.startsWith('命中立即禁用规则')
                return (
                  <TableRow key={item.id}>
                    <TableCell className='whitespace-nowrap'>
                      {formatTime(item.created_at)}
                    </TableCell>
                    <TableCell>
                      <Badge variant={instant ? 'destructive' : 'secondary'}>
                        {instant ? t('Immediate disable') : t('Circuit break')}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      {item.channel_name || `#${item.channel_id}`}
                    </TableCell>
                    <TableCell>{item.model_name || '-'}</TableCell>
                    <TableCell>{item.using_group || '-'}</TableCell>
                    <TableCell>{item.rule_name || '-'}</TableCell>
                    <TableCell>{instant ? '-' : item.failures}</TableCell>
                    <TableCell>{instant ? '-' : item.cooldown_secs}</TableCell>
                  </TableRow>
                )
              })}
            </RuleTable>
            {historyTotal > 10 ? (
              <div className='flex items-center justify-end gap-2 p-3'>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={historyPage <= 1}
                  onClick={() => void fetchHistory(historyPage - 1)}
                >
                  {t('Previous')}
                </Button>
                <span className='text-muted-foreground text-sm'>
                  {historyPage} / {Math.ceil(historyTotal / 10)}
                </span>
                <Button
                  variant='outline'
                  size='sm'
                  disabled={historyPage * 10 >= historyTotal}
                  onClick={() => void fetchHistory(historyPage + 1)}
                >
                  {t('Next')}
                </Button>
              </div>
            ) : null}
          </TitledCard>
        </TabsContent>

        <TabsContent value='rules' className='grid gap-4 pt-4'>
          <TitledCard
            title={t('Circuit breaker rules')}
            action={
              <div className='flex flex-wrap gap-2'>
                <Button
                  variant='outline'
                  onClick={() => {
                    setEditingRule(null)
                    setRuleDialogOpen(true)
                  }}
                >
                  <Plus data-icon='inline-start' />
                  {t('Add Rule')}
                </Button>
                <Button onClick={() => void saveRules()} disabled={loading}>
                  <Save data-icon='inline-start' />
                  {t('Save rules')}
                </Button>
              </div>
            }
          >
            <div className='mb-4 flex flex-wrap gap-2'>
              {BREAKER_TEMPLATES.map((template) => (
                <Button
                  key={template.label}
                  variant='secondary'
                  size='sm'
                  onClick={() => addTemplate(template)}
                >
                  {t(template.label)}
                </Button>
              ))}
            </div>
            <RuleTable
              headers={[
                t('Rule name'),
                t('Scope'),
                t('Match targets'),
                t('Failure / cooldown'),
                t('Probe requirement'),
                t('Actions'),
              ]}
              empty={false}
              emptyText={t('No data')}
            >
              {[BUILTIN_BREAKER_RULE, ...rules].map((rule) => {
                const isBuiltIn = rule.id === BUILTIN_BREAKER_RULE.id
                let stateLabel = t('Disabled')
                if (isBuiltIn) {
                  stateLabel = t('Default')
                } else if (rule.enabled) {
                  stateLabel = t('Enabled')
                }
                return (
                  <TableRow key={rule.id}>
                    <TableCell>
                      <span className='font-medium'>
                        {isBuiltIn ? t(rule.name) : rule.name}
                      </span>
                      <div className='mt-1 flex flex-wrap gap-1'>
                        <Badge variant='secondary'>{stateLabel}</Badge>
                        {rule.disable_breaker ? (
                          <Badge variant='outline'>{t('Bypass')}</Badge>
                        ) : null}
                        {rule.instant_disable_enabled ? (
                          <Badge variant='destructive'>
                            {t('Immediate disable')}
                          </Badge>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell>{t(rule.scope)}</TableCell>
                    <TableCell>
                      {rule.targets.join(', ') || t('All requests')}
                    </TableCell>
                    <TableCell>
                      {rule.failure_limit} / {rule.cooldown_seconds}s
                    </TableCell>
                    <TableCell>
                      {rule.probe_success_count}/{rule.probe_count}
                    </TableCell>
                    <TableCell>
                      {isBuiltIn ? (
                        <span className='text-muted-foreground'>-</span>
                      ) : (
                        <div className='flex gap-1'>
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={t('Edit')}
                            title={t('Edit')}
                            onClick={() => {
                              setEditingRule(rule)
                              setRuleDialogOpen(true)
                            }}
                          >
                            <Edit />
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={
                              rule.enabled ? t('Disable') : t('Enable')
                            }
                            title={rule.enabled ? t('Disable') : t('Enable')}
                            onClick={() =>
                              setRules((current) =>
                                current.map((item) =>
                                  item.id === rule.id
                                    ? { ...item, enabled: !item.enabled }
                                    : item
                                )
                              )
                            }
                          >
                            <Switch size='sm' checked={rule.enabled} />
                          </Button>
                          <Button
                            variant='ghost'
                            size='icon-sm'
                            aria-label={t('Delete')}
                            title={t('Delete')}
                            onClick={() =>
                              setRules((current) =>
                                current.filter((item) => item.id !== rule.id)
                              )
                            }
                          >
                            <Trash2 />
                          </Button>
                        </div>
                      )}
                    </TableCell>
                  </TableRow>
                )
              })}
            </RuleTable>
          </TitledCard>
        </TabsContent>

        <TabsContent value='exemptions' className='grid gap-4 pt-4'>
          <TitledCard
            title={t('Exempt channels')}
            action={
              <Button onClick={() => void saveExemptions()} disabled={loading}>
                <Save data-icon='inline-start' />
                {t('Save exemptions')}
              </Button>
            }
          >
            <div className='mb-4 flex gap-2'>
              <Input
                value={channelKeyword}
                onChange={(event) => setChannelKeyword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    void searchChannels()
                  }
                }}
                placeholder={t('Search channels by name or ID')}
              />
              <Button variant='outline' onClick={() => void searchChannels()}>
                {t('Search')}
              </Button>
            </div>
            {channelResults.length ? (
              <div className='mb-4 flex max-h-40 flex-wrap gap-2 overflow-y-auto rounded-lg border p-2'>
                {channelResults
                  .filter((channel) => !exemptIds.includes(channel.id))
                  .map((channel) => (
                    <Button
                      key={channel.id}
                      variant='secondary'
                      size='sm'
                      onClick={() =>
                        setExemptIds((current) => [
                          ...new Set([...current, channel.id]),
                        ])
                      }
                    >
                      <Plus data-icon='inline-start' />#{channel.id}{' '}
                      {channel.name}
                    </Button>
                  ))}
              </div>
            ) : null}
            <RuleTable
              headers={[t('Channel ID'), t('Channel name'), t('Actions')]}
              empty={exemptIds.length === 0}
              emptyText={t('No data')}
            >
              {exemptIds.map((id) => (
                <TableRow key={id}>
                  <TableCell className='font-medium'>#{id}</TableCell>
                  <TableCell>{channelNames[id] || '-'}</TableCell>
                  <TableCell>
                    <Button
                      variant='ghost'
                      size='icon-sm'
                      aria-label={t('Remove')}
                      title={t('Remove')}
                      onClick={() =>
                        setExemptIds((current) =>
                          current.filter((channelId) => channelId !== id)
                        )
                      }
                    >
                      <Trash2 />
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </RuleTable>
          </TitledCard>
        </TabsContent>

        <TabsContent value='settings' className='grid gap-4 pt-4'>
          <TitledCard title={t('Global circuit breaker settings')}>
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
              <label className='flex items-center gap-2 text-sm'>
                <Switch
                  checked={config.ChannelBreakerEnabled}
                  onCheckedChange={(checked) =>
                    setConfig((current) => ({
                      ...current,
                      ChannelBreakerEnabled: checked,
                    }))
                  }
                />
                {t('Enable circuit breaker routing')}
              </label>
              {(
                [
                  ['ChannelBreakerFailureLimit', 'Failure limit'],
                  ['ChannelBreakerCooldownSeconds', 'Cooldown seconds'],
                  ['ChannelBreakerProbeCount', 'Probe requests'],
                  [
                    'ChannelBreakerProbeSuccessCount',
                    'Required probe successes',
                  ],
                ] as const
              ).map(([key, label]) => (
                <EditorField key={key} label={t(label)}>
                  <Input
                    type='number'
                    min={1}
                    value={config[key]}
                    onChange={(event) =>
                      setConfig((current) => ({
                        ...current,
                        [key]: event.target.value,
                      }))
                    }
                  />
                </EditorField>
              ))}
              <div className='sm:col-span-2 lg:col-span-3'>
                <EditorField label={t('Failure status codes')}>
                  <Input
                    value={config.ChannelBreakerFailureStatusCodes}
                    onChange={(event) =>
                      setConfig((current) => ({
                        ...current,
                        ChannelBreakerFailureStatusCodes: event.target.value,
                      }))
                    }
                  />
                </EditorField>
              </div>
              <EditorField label={t('Excluded path prefixes')}>
                <Textarea
                  rows={5}
                  value={config.ChannelBreakerExcludePaths}
                  onChange={(event) =>
                    setConfig((current) => ({
                      ...current,
                      ChannelBreakerExcludePaths: event.target.value,
                    }))
                  }
                />
              </EditorField>
              <div className='sm:col-span-2'>
                <EditorField label={t('Failure keywords')}>
                  <Textarea
                    rows={5}
                    value={config.AutomaticDisableKeywords}
                    onChange={(event) =>
                      setConfig((current) => ({
                        ...current,
                        AutomaticDisableKeywords: event.target.value,
                      }))
                    }
                  />
                </EditorField>
              </div>
            </div>
          </TitledCard>

          <TitledCard title={t('Bark alerts')}>
            <div className='grid gap-4 sm:grid-cols-2 lg:grid-cols-3'>
              {(
                [
                  ['monitor_setting.bark_alert_enabled', 'Enable Bark alerts'],
                  [
                    'monitor_setting.channel_breaker_alert_enabled',
                    'Circuit breaker alerts',
                  ],
                  [
                    'monitor_setting.low_balance_alert_enabled',
                    'Low balance alerts',
                  ],
                  [
                    'monitor_setting.channel_disable_alert_enabled',
                    'Channel disable alerts',
                  ],
                ] as const
              ).map(([key, label]) => (
                <label key={key} className='flex items-center gap-2 text-sm'>
                  <Switch
                    checked={config[key]}
                    onCheckedChange={(checked) =>
                      setConfig((current) => ({ ...current, [key]: checked }))
                    }
                  />
                  {t(label)}
                </label>
              ))}
              <div className='sm:col-span-2 lg:col-span-3'>
                <EditorField label={t('Bark API URL')}>
                  <Input
                    value={config['monitor_setting.bark_alert_url']}
                    onChange={(event) =>
                      setConfig((current) => ({
                        ...current,
                        'monitor_setting.bark_alert_url': event.target.value,
                      }))
                    }
                    placeholder='https://bark.example.com/device-key/'
                  />
                </EditorField>
              </div>
              {(
                [
                  ['monitor_setting.bark_alert_volume', 'Alert volume'],
                  [
                    'monitor_setting.low_balance_threshold_cny',
                    'Low balance threshold (CNY)',
                  ],
                  [
                    'monitor_setting.channel_disable_alert_cooldown_second',
                    'Disable alert cooldown seconds',
                  ],
                ] as const
              ).map(([key, label]) => (
                <EditorField key={key} label={t(label)}>
                  <Input
                    type='number'
                    min={0}
                    value={config[key]}
                    onChange={(event) =>
                      setConfig((current) => ({
                        ...current,
                        [key]: Number(event.target.value) || 0,
                      }))
                    }
                  />
                </EditorField>
              ))}
              {(
                [
                  [
                    'monitor_setting.channel_breaker_alert_sound',
                    'Circuit breaker alert sound',
                  ],
                  [
                    'monitor_setting.channel_disable_alert_sound',
                    'Channel disable alert sound',
                  ],
                  [
                    'monitor_setting.low_balance_alert_sound',
                    'Low balance alert sound',
                  ],
                ] as const
              ).map(([key, label]) => (
                <EditorField key={key} label={t(label)}>
                  <NativeSelect
                    className='w-full'
                    value={config[key]}
                    onChange={(event) =>
                      setConfig((current) => ({
                        ...current,
                        [key]: event.target.value,
                      }))
                    }
                  >
                    {BARK_SOUNDS.map((sound) => (
                      <NativeSelectOption key={sound} value={sound}>
                        {sound}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </EditorField>
              ))}
            </div>
          </TitledCard>

          <TitledCard title={t('Retest disabled channels')}>
            <div className='grid gap-4 sm:grid-cols-2'>
              <label className='flex items-center gap-2 text-sm'>
                <Switch
                  checked={
                    config['monitor_setting.retest_disabled_channel_enabled']
                  }
                  onCheckedChange={(checked) =>
                    setConfig((current) => ({
                      ...current,
                      'monitor_setting.retest_disabled_channel_enabled':
                        checked,
                    }))
                  }
                />
                {t('Retest disabled channels')}
              </label>
              <EditorField label={t('Retest interval seconds')}>
                <Input
                  type='number'
                  min={5}
                  value={
                    config['monitor_setting.retest_disabled_channel_seconds']
                  }
                  onChange={(event) =>
                    setConfig((current) => ({
                      ...current,
                      'monitor_setting.retest_disabled_channel_seconds':
                        Number(event.target.value) || 5,
                    }))
                  }
                />
              </EditorField>
            </div>
          </TitledCard>

          <div className='flex justify-end'>
            <Button onClick={() => void saveConfig()} disabled={loading}>
              <Save data-icon='inline-start' />
              {loading ? t('Saving...') : t('Save global settings')}
            </Button>
          </div>
        </TabsContent>
      </Tabs>

      <ChannelBreakerRuleDialog
        open={ruleDialogOpen}
        onOpenChange={setRuleDialogOpen}
        rule={editingRule}
        onSave={saveEditedRule}
      />
    </div>
  )
}
