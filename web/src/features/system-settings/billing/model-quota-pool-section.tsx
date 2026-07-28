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
import { Edit, Plus, Save, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { TableCell, TableRow } from '@/components/ui/table'
import { TitledCard } from '@/components/ui/titled-card'
import {
  formatQuota,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'

import { useUpdateOption } from '../hooks/use-update-option'
import { EditorField, RuleTable } from './rule-editor-ui'
import { UserSearchPicker, type UserChoice } from './user-search-picker'

type PoolScope = 'global' | 'user'
type PoolMetric = 'requests' | 'total_tokens' | 'quota'
type PoolPeriod = 'minute' | 'hour' | 'day' | 'week' | 'month'

type QuotaPoolRule = {
  id: string
  model: string
  scope: PoolScope
  metric: PoolMetric
  user_id: number
  username: string
  user_group: string
  period: PoolPeriod
  limit: number
  message: string
  disabled: boolean
}

const EMPTY_RULE: QuotaPoolRule = {
  id: '',
  model: '',
  scope: 'global',
  metric: 'requests',
  user_id: 0,
  username: '',
  user_group: '',
  period: 'day',
  limit: 500,
  message: '',
  disabled: false,
}

function normalizeRule(raw: Partial<QuotaPoolRule>): QuotaPoolRule {
  let metric: PoolMetric = 'requests'
  if (raw.metric === 'quota') metric = 'quota'
  if (
    raw.metric === 'total_tokens' ||
    raw.metric === ('prompt_tokens' as PoolMetric)
  ) {
    metric = 'total_tokens'
  }
  return {
    ...EMPTY_RULE,
    ...raw,
    id: raw.id || `rule-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    model: String(raw.model || '').trim(),
    scope: raw.scope === 'user' ? 'user' : 'global',
    metric,
    user_id: Number(raw.user_id) || 0,
    limit: Number(raw.limit) || 0,
    disabled: Boolean(raw.disabled),
  }
}

function parseRules(raw: string): QuotaPoolRule[] {
  try {
    const parsed: unknown = JSON.parse(raw || '{"rules":[]}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return []
    }
    const rules = (parsed as { rules?: unknown }).rules
    return Array.isArray(rules)
      ? rules.map((rule) => normalizeRule(rule as Partial<QuotaPoolRule>))
      : []
  } catch {
    return []
  }
}

function formatLimit(rule: QuotaPoolRule, t: (key: string) => string) {
  if (rule.metric === 'quota') return formatQuota(rule.limit)
  return `${rule.limit} ${rule.metric === 'requests' ? t('requests') : 'tokens'}`
}

export function ModelQuotaPoolSection(props: { defaultValue: string }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [rules, setRules] = useState(() => parseRules(props.defaultValue))
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingId, setEditingId] = useState('')
  const [form, setForm] = useState<QuotaPoolRule>(EMPTY_RULE)
  const [selectedUsers, setSelectedUsers] = useState<UserChoice[]>([])

  useEffect(
    () => setRules(parseRules(props.defaultValue)),
    [props.defaultValue]
  )

  const openEditor = (rule?: QuotaPoolRule) => {
    const next = rule
      ? normalizeRule(rule)
      : normalizeRule({ id: `rule-${Date.now()}` })
    setEditingId(rule?.id ?? '')
    setForm(next)
    setSelectedUsers(
      next.user_id
        ? [
            {
              id: next.user_id,
              username: next.username,
              group: next.user_group,
            },
          ]
        : []
    )
    setDialogOpen(true)
  }

  const saveRule = () => {
    const user = selectedUsers[0]
    const next = normalizeRule({
      ...form,
      user_id: form.scope === 'user' ? user?.id : 0,
      username: form.scope === 'user' ? user?.username : '',
      user_group: form.scope === 'user' ? user?.group : '',
    })
    if (!next.model) {
      toast.error(t('Enter a model match rule'))
      return
    }
    if (next.limit <= 0) {
      toast.error(t('Limit must be greater than zero'))
      return
    }
    if (next.scope === 'user' && !next.user_id) {
      toast.error(t('Select a user'))
      return
    }
    setRules((current) =>
      [...current.filter((rule) => rule.id !== editingId), next].sort((a, b) =>
        `${a.model}-${a.scope}-${a.user_id}`.localeCompare(
          `${b.model}-${b.scope}-${b.user_id}`
        )
      )
    )
    setDialogOpen(false)
  }

  const save = () =>
    updateOption.mutate({
      key: 'ModelQuotaPool',
      value: JSON.stringify({ rules }, null, 2),
    })

  return (
    <div className='grid gap-4'>
      <TitledCard
        title={t('Model Quota Pools')}
        action={
          <div className='flex flex-wrap gap-2'>
            <Button variant='outline' onClick={() => openEditor()}>
              <Plus data-icon='inline-start' />
              {t('Add Rule')}
            </Button>
            <Button onClick={save} disabled={updateOption.isPending}>
              <Save data-icon='inline-start' />
              {updateOption.isPending ? t('Saving...') : t('Save')}
            </Button>
          </div>
        }
      />

      <RuleTable
        headers={[
          t('Status'),
          t('Model match'),
          t('Pool scope'),
          t('User'),
          t('Metric'),
          t('Period'),
          t('Limit'),
          t('Actions'),
        ]}
        empty={rules.length === 0}
        emptyText={t('No data')}
      >
        {rules.map((rule) => (
          <TableRow key={rule.id}>
            <TableCell>
              <Switch
                size='sm'
                checked={!rule.disabled}
                aria-label={t('Enabled')}
                onCheckedChange={(checked) =>
                  setRules((current) =>
                    current.map((item) =>
                      item.id === rule.id
                        ? { ...item, disabled: !checked }
                        : item
                    )
                  )
                }
              />
            </TableCell>
            <TableCell className='font-mono text-xs'>{rule.model}</TableCell>
            <TableCell>
              <Badge variant='secondary'>
                {rule.scope === 'user' ? t('User pool') : t('Global pool')}
              </Badge>
            </TableCell>
            <TableCell className='whitespace-nowrap'>
              {rule.scope === 'user'
                ? `${rule.user_id} / ${rule.username || '-'} / ${rule.user_group || '-'}`
                : t('All users')}
            </TableCell>
            <TableCell>{t(rule.metric)}</TableCell>
            <TableCell>{t(rule.period)}</TableCell>
            <TableCell className='whitespace-nowrap'>
              {formatLimit(rule, t)}
            </TableCell>
            <TableCell>
              <div className='flex gap-1'>
                <Button
                  variant='ghost'
                  size='icon-sm'
                  aria-label={t('Edit')}
                  title={t('Edit')}
                  onClick={() => openEditor(rule)}
                >
                  <Edit />
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
            </TableCell>
          </TableRow>
        ))}
      </RuleTable>

      <Dialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={editingId ? t('Edit Rule') : t('Add Rule')}
        contentClassName='sm:max-w-3xl'
        footer={
          <>
            <Button variant='outline' onClick={() => setDialogOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button onClick={saveRule}>{t('Save')}</Button>
          </>
        }
      >
        <div className='grid gap-4 sm:grid-cols-2'>
          <div className='sm:col-span-2'>
            <EditorField label={t('Model match')}>
              <Input
                value={form.model}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    model: event.target.value,
                  }))
                }
                placeholder='prism-*'
              />
            </EditorField>
          </div>
          <EditorField label={t('Pool scope')}>
            <NativeSelect
              className='w-full'
              value={form.scope}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  scope: event.target.value as PoolScope,
                }))
              }
            >
              <NativeSelectOption value='global'>
                {t('Global pool')}
              </NativeSelectOption>
              <NativeSelectOption value='user'>
                {t('User pool')}
              </NativeSelectOption>
            </NativeSelect>
          </EditorField>
          <EditorField label={t('Metric')}>
            <NativeSelect
              className='w-full'
              value={form.metric}
              onChange={(event) =>
                setForm((current) => {
                  const metric = event.target.value as PoolMetric
                  let limit = current.limit
                  if (metric === 'quota' && current.metric !== 'quota') {
                    limit = parseQuotaFromDollars(current.limit)
                  } else if (metric !== 'quota' && current.metric === 'quota') {
                    limit = Math.max(
                      1,
                      Math.round(quotaUnitsToDollars(current.limit))
                    )
                  }
                  return { ...current, metric, limit }
                })
              }
            >
              <NativeSelectOption value='requests'>
                {t('Requests')}
              </NativeSelectOption>
              <NativeSelectOption value='total_tokens'>
                {t('Total tokens')}
              </NativeSelectOption>
              <NativeSelectOption value='quota'>
                {t('Spending')}
              </NativeSelectOption>
            </NativeSelect>
          </EditorField>
          {form.scope === 'user' ? (
            <div className='sm:col-span-2'>
              <EditorField label={t('User')}>
                <UserSearchPicker
                  value={selectedUsers}
                  onChange={setSelectedUsers}
                />
              </EditorField>
            </div>
          ) : null}
          <EditorField label={t('Period')}>
            <NativeSelect
              className='w-full'
              value={form.period}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  period: event.target.value as PoolPeriod,
                }))
              }
            >
              {(['minute', 'hour', 'day', 'week', 'month'] as const).map(
                (period) => (
                  <NativeSelectOption key={period} value={period}>
                    {t(period)}
                  </NativeSelectOption>
                )
              )}
            </NativeSelect>
          </EditorField>
          <EditorField
            label={form.metric === 'quota' ? t('Display amount') : t('Limit')}
          >
            <Input
              type='number'
              min={0}
              step={form.metric === 'quota' ? 0.01 : 1}
              value={
                form.metric === 'quota'
                  ? quotaUnitsToDollars(form.limit)
                  : form.limit
              }
              onChange={(event) => {
                const value = Number(event.target.value) || 0
                setForm((current) => ({
                  ...current,
                  limit:
                    current.metric === 'quota'
                      ? parseQuotaFromDollars(value)
                      : value,
                }))
              }}
            />
          </EditorField>
          <div className='sm:col-span-2'>
            <EditorField label={t('Message when exceeded')}>
              <Input
                value={form.message}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    message: event.target.value,
                  }))
                }
                placeholder={t(
                  'The model quota for this period has been reached'
                )}
              />
            </EditorField>
          </div>
          <label className='flex items-center gap-2 text-sm'>
            <Switch
              checked={!form.disabled}
              onCheckedChange={(checked) =>
                setForm((current) => ({ ...current, disabled: !checked }))
              }
            />
            {t('Enabled')}
          </label>
        </div>
      </Dialog>
    </div>
  )
}
