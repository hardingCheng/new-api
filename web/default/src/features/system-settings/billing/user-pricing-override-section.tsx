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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { TableCell, TableRow } from '@/components/ui/table'
import { TitledCard } from '@/components/ui/titled-card'
import { api } from '@/lib/api'

import { useUpdateOption } from '../hooks/use-update-option'
import { EditorField, RuleTable } from './rule-editor-ui'
import { UserSearchPicker, type UserChoice } from './user-search-picker'

type PriceRuleType =
  | 'ratio'
  | 'model_price'
  | 'model_ratio'
  | 'video_ref_factor'
  | 'video_ref_price'
  | 'video_ref_flat'
  | 'video_ref_cap'

type PricingScenario =
  | 'all_discount'
  | 'group_discount'
  | 'model_fixed_price'
  | 'model_ratio'
  | 'video_reference'

type PricingRule = {
  id: string
  user_id: number
  username: string
  user_group: string
  group_pattern: string
  model_pattern: string
  type: PriceRuleType
  value: number
  apply_group_ratio: boolean
  disabled: boolean
}

type ModelPriceInfo = {
  exists?: boolean
  use_price?: boolean
  price?: number
  ratio?: number
}

type PricingContext = {
  user: UserChoice & { current_group_ratio?: number }
  groups?: Array<{
    name: string
    desc?: string
    ratio?: number
    models?: string[]
  }>
  models?: string[]
  model_prices?: Record<string, ModelPriceInfo>
}

const VIDEO_TYPES = new Set<PriceRuleType>([
  'video_ref_factor',
  'video_ref_price',
  'video_ref_flat',
  'video_ref_cap',
])

const EMPTY_RULE: PricingRule = {
  id: '',
  user_id: 0,
  username: '',
  user_group: '',
  group_pattern: '',
  model_pattern: '',
  type: 'ratio',
  value: 1,
  apply_group_ratio: false,
  disabled: false,
}

function normalizeRule(raw: Partial<PricingRule>, index = 0): PricingRule {
  return {
    ...EMPTY_RULE,
    ...raw,
    id:
      raw.id ||
      `user-${Number(raw.user_id) || 0}-${index}-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    user_id: Number(raw.user_id) || 0,
    value: Number(raw.value) || 0,
    apply_group_ratio: Boolean(raw.apply_group_ratio),
    disabled: Boolean(raw.disabled),
  }
}

function parseRules(raw: string): PricingRule[] {
  try {
    const parsed: unknown = JSON.parse(raw || '{"rules":[]}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return []
    }
    const rules = (parsed as { rules?: unknown }).rules
    return Array.isArray(rules)
      ? rules.map((rule, index) =>
          normalizeRule(rule as Partial<PricingRule>, index)
        )
      : []
  } catch {
    return []
  }
}

function serializeRules(rules: PricingRule[]) {
  return JSON.stringify(
    {
      rules: rules
        .filter((rule) => rule.user_id > 0 && rule.type)
        .map(({ id: _, ...rule }) => ({
          ...rule,
          username: rule.username.trim(),
          user_group: rule.user_group.trim(),
          group_pattern: rule.group_pattern.trim(),
          model_pattern: rule.model_pattern.trim(),
        })),
    },
    null,
    2
  )
}

function inferScenario(rule: PricingRule): PricingScenario {
  if (rule.type === 'model_price') return 'model_fixed_price'
  if (rule.type === 'model_ratio') return 'model_ratio'
  if (VIDEO_TYPES.has(rule.type)) return 'video_reference'
  return rule.group_pattern ? 'group_discount' : 'all_discount'
}

function ruleTypeForScenario(
  scenario: PricingScenario,
  current: PriceRuleType
): PriceRuleType {
  if (scenario === 'model_fixed_price') return 'model_price'
  if (scenario === 'model_ratio') return 'model_ratio'
  if (scenario === 'video_reference') {
    return VIDEO_TYPES.has(current) ? current : 'video_ref_factor'
  }
  return 'ratio'
}

function isModelScoped(scenario: PricingScenario) {
  return (
    scenario === 'model_fixed_price' ||
    scenario === 'model_ratio' ||
    scenario === 'video_reference'
  )
}

function selectedOrPattern(selected: string[], pattern: string) {
  if (selected.length) return selected
  return pattern ? [pattern] : []
}

function typeLabel(type: PriceRuleType, t: (key: string) => string) {
  const labels: Record<PriceRuleType, string> = {
    ratio: 'Overall ratio',
    model_price: 'Fixed model price',
    model_ratio: 'Model ratio',
    video_ref_factor: 'Reference-second ratio',
    video_ref_price: 'Reference-second price',
    video_ref_flat: 'Reference-video flat price',
    video_ref_cap: 'Reference-second cap',
  }
  return t(labels[type])
}

function formatNumber(value: unknown) {
  const number = Number(value)
  return Number.isFinite(number) ? Number(number.toFixed(6)).toString() : '-'
}

export function UserPricingOverrideSection(props: { defaultValue: string }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [rules, setRules] = useState(() => parseRules(props.defaultValue))
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingId, setEditingId] = useState('')
  const [scenario, setScenario] = useState<PricingScenario>('group_discount')
  const [form, setForm] = useState<PricingRule>(EMPTY_RULE)
  const [selectedUsers, setSelectedUsers] = useState<UserChoice[]>([])
  const [selectedModels, setSelectedModels] = useState<string[]>([])
  const [contexts, setContexts] = useState<PricingContext[]>([])
  const [contextLoading, setContextLoading] = useState(false)

  useEffect(
    () => setRules(parseRules(props.defaultValue)),
    [props.defaultValue]
  )

  const groupOptions = useMemo(() => {
    const map = new Map<string, NonNullable<PricingContext['groups']>[number]>()
    contexts
      .flatMap((context) => context.groups ?? [])
      .forEach((group) => {
        if (!map.has(group.name)) map.set(group.name, group)
      })
    return [...map.values()].sort((a, b) => a.name.localeCompare(b.name))
  }, [contexts])

  const modelOptions = useMemo(() => {
    const values = form.group_pattern
      ? contexts.flatMap(
          (context) =>
            context.groups?.find((group) => group.name === form.group_pattern)
              ?.models ?? []
        )
      : contexts.flatMap((context) => context.models ?? [])
    return [...new Set(values)].sort((a, b) => a.localeCompare(b))
  }, [contexts, form.group_pattern])

  const comparisonRows = useMemo(() => {
    if (contexts.length === 0) return []
    let models = ['']
    if (isModelScoped(scenario)) {
      models = selectedOrPattern(selectedModels, form.model_pattern)
    }
    return contexts.flatMap((context) => {
      const groupRatio = form.group_pattern
        ? context.groups?.find((group) => group.name === form.group_pattern)
            ?.ratio
        : context.user.current_group_ratio
      return models.map((model) => {
        const info = model ? context.model_prices?.[model] : undefined
        const base = info?.use_price ? info.price : info?.ratio
        const original = model
          ? Number(base) * Number(groupRatio)
          : Number(groupRatio)
        let overridden = original
        if (form.type === 'ratio') overridden = form.value
        if (form.type === 'model_price' || form.type === 'model_ratio') {
          overridden = form.value * Number(groupRatio)
        }
        return {
          key: `${context.user.id}-${model || 'all'}`,
          user: `${context.user.id} / ${context.user.username}`,
          model: model || t('All Models'),
          original: formatNumber(original),
          overridden: formatNumber(overridden),
        }
      })
    })
  }, [contexts, form, scenario, selectedModels, t])

  const loadContexts = async (users: UserChoice[]) => {
    setSelectedUsers(users)
    setContexts([])
    if (!users.length) return
    setContextLoading(true)
    try {
      const values = await Promise.all(
        users.map(async (user) => {
          const response = await api.get<{
            success: boolean
            message?: string
            data?: PricingContext
          }>(`/api/user/${user.id}/pricing_context`)
          if (!response.data.success || !response.data.data) {
            throw new Error(response.data.message || t('Failed to load'))
          }
          return response.data.data
        })
      )
      setContexts(values)
      const first = values[0]
      setForm((current) => ({
        ...current,
        user_id: first.user.id,
        username: first.user.username,
        user_group: first.user.group,
      }))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Failed to load'))
    } finally {
      setContextLoading(false)
    }
  }

  const openEditor = (rule?: PricingRule) => {
    const next = rule ? normalizeRule(rule) : normalizeRule({})
    const nextScenario = rule ? inferScenario(next) : 'group_discount'
    setEditingId(rule?.id ?? '')
    setScenario(nextScenario)
    setForm(next)
    setSelectedModels(next.model_pattern ? [next.model_pattern] : [])
    const users = next.user_id
      ? [
          {
            id: next.user_id,
            username: next.username,
            group: next.user_group,
          },
        ]
      : []
    setSelectedUsers(users)
    setContexts([])
    setDialogOpen(true)
    if (users.length) void loadContexts(users)
  }

  const saveRule = () => {
    if (!selectedUsers.length) {
      toast.error(t('Select at least one user'))
      return
    }
    let targetModels = ['']
    if (isModelScoped(scenario)) {
      targetModels = selectedOrPattern(
        selectedModels,
        form.model_pattern.trim()
      )
    }
    if (isModelScoped(scenario) && !targetModels.length) {
      toast.error(t('Select a model or enter a wildcard'))
      return
    }

    const additions = selectedUsers.flatMap((user) =>
      targetModels.map((model, index) =>
        normalizeRule({
          ...form,
          id:
            editingId && selectedUsers.length === 1 && targetModels.length === 1
              ? editingId
              : `rule-${Date.now()}-${user.id}-${index}`,
          user_id: user.id,
          username: user.username,
          user_group: user.group,
          group_pattern:
            scenario === 'all_discount' ? '' : form.group_pattern.trim(),
          model_pattern: model,
          type: ruleTypeForScenario(scenario, form.type),
        })
      )
    )
    setRules((current) => [
      ...current.filter((rule) => rule.id !== editingId),
      ...additions,
    ])
    setDialogOpen(false)
  }

  const save = () =>
    updateOption.mutate({
      key: 'UserPricingOverride',
      value: serializeRules(rules),
    })

  return (
    <div className='grid gap-4'>
      <TitledCard
        title={t('User Pricing Overrides')}
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
          t('User'),
          t('Group scope'),
          t('Model scope'),
          t('Pricing action'),
          t('Value'),
          t('Actions'),
        ]}
        empty={rules.length === 0}
        emptyText={t('No data')}
      >
        {rules.map((rule) => (
          <TableRow key={rule.id}>
            <TableCell>
              <Badge variant={rule.disabled ? 'destructive' : 'secondary'}>
                {rule.disabled ? t('Disabled') : t('Enabled')}
              </Badge>
            </TableCell>
            <TableCell className='whitespace-nowrap'>
              {rule.user_id} / {rule.username || '-'} / {rule.user_group || '-'}
            </TableCell>
            <TableCell>{rule.group_pattern || t('All Groups')}</TableCell>
            <TableCell className='font-mono text-xs'>
              {rule.model_pattern || t('All Models')}
            </TableCell>
            <TableCell>{typeLabel(rule.type, t)}</TableCell>
            <TableCell>{rule.value}</TableCell>
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
        contentClassName='sm:max-w-5xl'
        footer={
          <>
            <Button variant='outline' onClick={() => setDialogOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button onClick={saveRule}>{t('Save')}</Button>
          </>
        }
      >
        <div className='grid gap-4'>
          <EditorField label={t('Pricing scenario')}>
            <NativeSelect
              className='w-full'
              value={scenario}
              onChange={(event) => {
                const value = event.target.value as PricingScenario
                setScenario(value)
                setSelectedModels([])
                setForm((current) => ({
                  ...current,
                  type: ruleTypeForScenario(value, current.type),
                  group_pattern:
                    value === 'all_discount' ? '' : current.group_pattern,
                  model_pattern: '',
                }))
              }}
            >
              <NativeSelectOption value='group_discount'>
                {t('Discount one group for users')}
              </NativeSelectOption>
              <NativeSelectOption value='model_fixed_price'>
                {t('Set a fixed model price for users')}
              </NativeSelectOption>
              <NativeSelectOption value='all_discount'>
                {t('Discount all usage for users')}
              </NativeSelectOption>
              <NativeSelectOption value='model_ratio'>
                {t('Set a model ratio for users')}
              </NativeSelectOption>
              <NativeSelectOption value='video_reference'>
                {t('Price reference-video seconds separately')}
              </NativeSelectOption>
            </NativeSelect>
          </EditorField>

          <EditorField label={t('Users')}>
            <UserSearchPicker
              multiple
              value={selectedUsers}
              onChange={(users) => void loadContexts(users)}
            />
          </EditorField>

          {scenario !== 'all_discount' ? (
            <EditorField label={t('Group scope')}>
              <NativeSelect
                className='w-full'
                value={form.group_pattern}
                onChange={(event) => {
                  setSelectedModels([])
                  setForm((current) => ({
                    ...current,
                    group_pattern: event.target.value,
                    model_pattern: '',
                  }))
                }}
              >
                <NativeSelectOption value=''>
                  {t('All Groups')}
                </NativeSelectOption>
                {groupOptions.map((group) => (
                  <NativeSelectOption key={group.name} value={group.name}>
                    {group.name} / {group.desc || '-'} /{' '}
                    {group.models?.length || 0}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </EditorField>
          ) : null}

          {isModelScoped(scenario) ? (
            <EditorField label={t('Models')}>
              <div className='max-h-40 overflow-y-auto rounded-lg border p-2'>
                {modelOptions.length ? (
                  <div className='grid gap-2 sm:grid-cols-2'>
                    {modelOptions.map((model) => (
                      <label
                        key={model}
                        className='flex items-center gap-2 text-sm'
                      >
                        <Checkbox
                          checked={selectedModels.includes(model)}
                          onCheckedChange={(checked) => {
                            const next = checked
                              ? [...new Set([...selectedModels, model])]
                              : selectedModels.filter((item) => item !== model)
                            setSelectedModels(next)
                            setForm((current) => ({
                              ...current,
                              model_pattern: next[0] || '',
                            }))
                          }}
                        />
                        <span className='truncate font-mono text-xs'>
                          {model}
                        </span>
                      </label>
                    ))}
                  </div>
                ) : (
                  <p className='text-muted-foreground text-sm'>
                    {contextLoading
                      ? t('Loading pricing context...')
                      : t('Select users to load available models')}
                  </p>
                )}
              </div>
              <Input
                value={
                  selectedModels.length === 1 &&
                  !modelOptions.includes(selectedModels[0])
                    ? selectedModels[0]
                    : ''
                }
                onChange={(event) => {
                  const value = event.target.value
                  setSelectedModels(value ? [value] : [])
                  setForm((current) => ({ ...current, model_pattern: value }))
                }}
                placeholder='seedance-*'
              />
            </EditorField>
          ) : null}

          {scenario === 'video_reference' ? (
            <EditorField label={t('Reference pricing mode')}>
              <NativeSelect
                className='w-full'
                value={form.type}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    type: event.target.value as PriceRuleType,
                  }))
                }
              >
                <NativeSelectOption value='video_ref_factor'>
                  {t('Reference-second discount or ratio')}
                </NativeSelectOption>
                <NativeSelectOption value='video_ref_price'>
                  {t('Fixed price per reference second')}
                </NativeSelectOption>
                <NativeSelectOption value='video_ref_flat'>
                  {t('Fixed total price for the reference video')}
                </NativeSelectOption>
                <NativeSelectOption value='video_ref_cap'>
                  {t('Billable reference-second cap')}
                </NativeSelectOption>
              </NativeSelect>
            </EditorField>
          ) : null}

          <EditorField label={typeLabel(form.type, t)}>
            <Input
              type='number'
              min={0}
              step={0.1}
              value={form.value}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  value: Number(event.target.value) || 0,
                }))
              }
            />
          </EditorField>

          {form.type === 'video_ref_price' || form.type === 'video_ref_flat' ? (
            <label className='flex items-center gap-2 text-sm'>
              <Switch
                checked={form.apply_group_ratio}
                onCheckedChange={(checked) =>
                  setForm((current) => ({
                    ...current,
                    apply_group_ratio: checked,
                  }))
                }
              />
              {t('Apply the user group ratio to this fixed price')}
            </label>
          ) : null}

          <label className='flex items-center gap-2 text-sm'>
            <Switch
              checked={!form.disabled}
              onCheckedChange={(checked) =>
                setForm((current) => ({ ...current, disabled: !checked }))
              }
            />
            {t('Enabled')}
          </label>

          {comparisonRows.length ? (
            <div className='rounded-lg border p-3'>
              <h4 className='mb-2 text-sm font-medium'>
                {t('Price comparison before and after override')}
              </h4>
              <div className='overflow-x-auto'>
                <table className='w-full text-sm'>
                  <thead>
                    <tr className='border-b text-left'>
                      <th className='p-2'>{t('User')}</th>
                      <th className='p-2'>{t('Model')}</th>
                      <th className='p-2'>{t('Before')}</th>
                      <th className='p-2'>{t('After')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {comparisonRows.map((row) => (
                      <tr key={row.key} className='border-b last:border-0'>
                        <td className='p-2'>{row.user}</td>
                        <td className='p-2 font-mono text-xs'>{row.model}</td>
                        <td className='p-2'>{row.original}</td>
                        <td className='p-2 font-medium'>{row.overridden}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ) : null}

          <div className='bg-muted rounded-lg p-3 text-sm'>
            {t('This will create {{count}} pricing rules.', {
              count:
                selectedUsers.length *
                (isModelScoped(scenario)
                  ? Math.max(selectedModels.length, form.model_pattern ? 1 : 0)
                  : 1),
            })}
          </div>
        </div>
      </Dialog>
    </div>
  )
}
