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
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { TableCell, TableRow } from '@/components/ui/table'
import { TitledCard } from '@/components/ui/titled-card'

import { useUpdateOption } from '../hooks/use-update-option'
import { EditorField, RuleTable } from './rule-editor-ui'

type BillingMode = 'per_second' | 'per_call'
type VideoBillingRule = { model: string; mode: BillingMode }

function parseRules(raw: string): VideoBillingRule[] {
  try {
    const parsed: unknown = JSON.parse(raw || '{}')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return []
    }
    return Object.entries(parsed)
      .filter(
        (entry): entry is [string, BillingMode] =>
          entry[1] === 'per_second' || entry[1] === 'per_call'
      )
      .map(([model, mode]) => ({ model, mode }))
      .sort((a, b) => a.model.localeCompare(b.model))
  } catch {
    return []
  }
}

function serializeRules(rules: VideoBillingRule[]) {
  return JSON.stringify(
    Object.fromEntries(rules.map((rule) => [rule.model, rule.mode])),
    null,
    2
  )
}

function wildcardMatches(pattern: string, value: string) {
  const escaped = pattern
    .split('*')
    .map((part) => part.replaceAll(/[.*+?^${}()|[\]\\]/g, '\\$&'))
    .join('.*')
  return new RegExp(`^${escaped}$`).test(value)
}

export function VideoBillingModeSection(props: { defaultValue: string }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [rules, setRules] = useState(() => parseRules(props.defaultValue))
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingModel, setEditingModel] = useState('')
  const [model, setModel] = useState('')
  const [mode, setMode] = useState<BillingMode>('per_second')
  const [previewModel, setPreviewModel] = useState('')

  useEffect(
    () => setRules(parseRules(props.defaultValue)),
    [props.defaultValue]
  )

  const previewMatch = useMemo(() => {
    const value = previewModel.trim()
    if (!value) return null
    return (
      rules.find((rule) => rule.model === value) ??
      rules
        .filter((rule) => wildcardMatches(rule.model, value))
        .sort(
          (a, b) =>
            b.model.replaceAll('*', '').length -
            a.model.replaceAll('*', '').length
        )[0] ??
      null
    )
  }, [previewModel, rules])

  const openEditor = (rule?: VideoBillingRule) => {
    setEditingModel(rule?.model ?? '')
    setModel(rule?.model ?? '')
    setMode(rule?.mode ?? 'per_second')
    setDialogOpen(true)
  }

  const upsertRule = () => {
    const normalized = model.trim()
    if (!normalized) {
      toast.error(t('Enter a model match rule'))
      return
    }
    setRules((current) =>
      [
        ...current.filter(
          (rule) => rule.model !== editingModel && rule.model !== normalized
        ),
        { model: normalized, mode },
      ].sort((a, b) => a.model.localeCompare(b.model))
    )
    setDialogOpen(false)
  }

  const save = () =>
    updateOption.mutate({
      key: 'VideoBillingMode',
      value: serializeRules(rules),
    })

  return (
    <div className='grid gap-4'>
      <TitledCard
        title={t('Video Billing Mode')}
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
      >
        <EditorField label={t('Rule match preview')}>
          <Input
            value={previewModel}
            onChange={(event) => setPreviewModel(event.target.value)}
            placeholder='seedance-2.0-480p'
          />
        </EditorField>
        {previewModel.trim() ? (
          <div className='mt-3 flex flex-wrap gap-2'>
            {previewMatch ? (
              <>
                <Badge variant='outline'>{previewMatch.model}</Badge>
                <Badge>
                  {previewMatch.mode === 'per_call'
                    ? t('Per call')
                    : t('Per second')}
                </Badge>
              </>
            ) : (
              <Badge variant='secondary'>{t('Default: per second')}</Badge>
            )}
          </div>
        ) : null}
      </TitledCard>

      <RuleTable
        headers={[t('Model match'), t('Billing mode'), t('Actions')]}
        empty={rules.length === 0}
        emptyText={t('No data')}
      >
        {rules.map((rule) => (
          <TableRow key={rule.model}>
            <TableCell className='font-mono text-xs'>{rule.model}</TableCell>
            <TableCell>
              <Badge variant='secondary'>
                {rule.mode === 'per_call' ? t('Per call') : t('Per second')}
              </Badge>
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
                      current.filter((item) => item.model !== rule.model)
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
        title={editingModel ? t('Edit Rule') : t('Add Rule')}
        footer={
          <>
            <Button variant='outline' onClick={() => setDialogOpen(false)}>
              {t('Cancel')}
            </Button>
            <Button onClick={upsertRule}>{t('Save')}</Button>
          </>
        }
      >
        <div className='grid gap-4'>
          <EditorField label={t('Model match')}>
            <Input
              value={model}
              onChange={(event) => setModel(event.target.value)}
              placeholder='seedance-*'
            />
          </EditorField>
          <EditorField label={t('Billing mode')}>
            <NativeSelect
              className='w-full'
              value={mode}
              onChange={(event) => setMode(event.target.value as BillingMode)}
            >
              <NativeSelectOption value='per_second'>
                {t('Per second')}
              </NativeSelectOption>
              <NativeSelectOption value='per_call'>
                {t('Per call')}
              </NativeSelectOption>
            </NativeSelect>
          </EditorField>
        </div>
      </Dialog>
    </div>
  )
}
