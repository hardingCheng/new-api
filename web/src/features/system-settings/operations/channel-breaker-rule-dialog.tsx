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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import { EditorField } from '../billing/rule-editor-ui'
import {
  DEFAULT_BREAKER_RULE,
  normalizeBreakerRule,
  type BreakerRule,
  type BreakerScope,
} from './channel-breaker-types'

export function ChannelBreakerRuleDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  rule: BreakerRule | null
  onSave: (rule: BreakerRule) => void
}) {
  const { t } = useTranslation()
  const [form, setForm] = useState<BreakerRule>(DEFAULT_BREAKER_RULE)

  useEffect(() => {
    if (!props.open) return
    setForm(normalizeBreakerRule(props.rule ?? {}))
  }, [props.open, props.rule])

  const save = () => {
    const next = normalizeBreakerRule(form)
    if (!next.name.trim()) {
      toast.error(t('Rule name is required'))
      return
    }
    if (next.scope !== 'global' && next.targets.length === 0) {
      toast.error(t('Non-global rules require at least one match target'))
      return
    }
    props.onSave(next)
    props.onOpenChange(false)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.rule ? t('Edit Rule') : t('Add Rule')}
      contentClassName='sm:max-w-4xl'
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={save}>{t('Save')}</Button>
        </>
      }
    >
      <div className='grid gap-4 sm:grid-cols-2'>
        <EditorField label={t('Rule name')}>
          <Input
            value={form.name}
            onChange={(event) =>
              setForm((current) => ({ ...current, name: event.target.value }))
            }
          />
        </EditorField>
        <EditorField label={t('Scope')}>
          <NativeSelect
            className='w-full'
            value={form.scope}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                scope: event.target.value as BreakerScope,
              }))
            }
          >
            <NativeSelectOption value='global'>
              {t('Global')}
            </NativeSelectOption>
            <NativeSelectOption value='group'>{t('Group')}</NativeSelectOption>
            <NativeSelectOption value='model'>{t('Model')}</NativeSelectOption>
            <NativeSelectOption value='channel'>
              {t('Channel')}
            </NativeSelectOption>
          </NativeSelect>
        </EditorField>
        {form.scope !== 'global' ? (
          <div className='sm:col-span-2'>
            <EditorField label={t('Match targets')}>
              <Textarea
                rows={3}
                value={form.targets.join('\n')}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    targets: event.target.value
                      .split(/[\n,，]/)
                      .map((item) => item.trim())
                      .filter(Boolean),
                  }))
                }
              />
            </EditorField>
          </div>
        ) : null}
        {(
          [
            ['failure_limit', 'Failure limit'],
            ['cooldown_seconds', 'Cooldown seconds'],
            ['probe_count', 'Probe requests'],
            ['probe_success_count', 'Required probe successes'],
          ] as const
        ).map(([key, label]) => (
          <EditorField key={key} label={t(label)}>
            <Input
              type='number'
              min={1}
              value={form[key]}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  [key]: Number(event.target.value) || 1,
                }))
              }
            />
          </EditorField>
        ))}
        <div className='sm:col-span-2'>
          <EditorField label={t('Failure status codes')}>
            <Input
              value={form.failure_status_codes}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  failure_status_codes: event.target.value,
                }))
              }
              placeholder='429,500-599'
            />
          </EditorField>
        </div>
        <EditorField label={t('Failure keywords')}>
          <Textarea
            rows={5}
            value={form.failure_keywords}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                failure_keywords: event.target.value,
              }))
            }
          />
        </EditorField>
        <EditorField label={t('Excluded path prefixes')}>
          <Textarea
            rows={5}
            value={form.exclude_paths}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                exclude_paths: event.target.value,
              }))
            }
          />
        </EditorField>
        <div className='grid gap-3 sm:col-span-2 sm:grid-cols-3'>
          {(
            [
              ['enabled', 'Enabled'],
              ['disable_breaker', 'Bypass circuit breaker'],
              ['ignore_client_error_4xx', 'Ignore 4xx client errors'],
              ['only_key_breaker', 'Break only the failing key'],
              ['instant_disable_enabled', 'Disable channel immediately'],
            ] as const
          ).map(([key, label]) => (
            <label key={key} className='flex items-center gap-2 text-sm'>
              <Switch
                checked={form[key]}
                onCheckedChange={(checked) =>
                  setForm((current) => ({ ...current, [key]: checked }))
                }
              />
              {t(label)}
            </label>
          ))}
        </div>
        {form.instant_disable_enabled ? (
          <>
            <EditorField label={t('Immediate-disable status codes')}>
              <Input
                value={form.instant_disable_status_codes}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    instant_disable_status_codes: event.target.value,
                  }))
                }
                placeholder='403'
              />
            </EditorField>
            <EditorField label={t('Immediate-disable keywords')}>
              <Textarea
                rows={4}
                value={form.instant_disable_keywords}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    instant_disable_keywords: event.target.value,
                  }))
                }
              />
            </EditorField>
          </>
        ) : null}
      </div>
    </Dialog>
  )
}
