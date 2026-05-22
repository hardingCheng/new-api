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
import { RotateCcw, Save } from 'lucide-react'
import * as z from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const DURATION_BILLING_MODEL_PATTERNS_KEY =
  'task_billing_setting.duration_billing_model_patterns'
const DURATION_BILLING_EXCLUDE_MODEL_PATTERNS_KEY =
  'task_billing_setting.duration_billing_exclude_model_patterns'
const REFERENCE_VIDEO_BILLING_MODEL_PATTERNS_KEY =
  'task_billing_setting.reference_video_billing_model_patterns'

const recommendedDurationBillingPatterns = [
  'sora-2*',
  'seedance-*',
  'doubao-seedance-*',
]

const recommendedDurationBillingExcludePatterns = [
  'grok-imagine-video',
  'grok-imagine-1.0-video',
]

const recommendedReferenceVideoBillingPatterns = [
  'seedance-*',
  'doubao-seedance-*',
]

const taskBillingSchema = z.object({
  durationBillingModelPatterns: z.string(),
  durationBillingExcludeModelPatterns: z.string(),
  referenceVideoBillingModelPatterns: z.string(),
})

type TaskBillingFormValues = z.infer<typeof taskBillingSchema>

type TaskBillingSettingsDefaults = {
  [DURATION_BILLING_MODEL_PATTERNS_KEY]: string
  [DURATION_BILLING_EXCLUDE_MODEL_PATTERNS_KEY]: string
  [REFERENCE_VIDEO_BILLING_MODEL_PATTERNS_KEY]: string
}

type Props = {
  defaultValues: TaskBillingSettingsDefaults
}

function parsePatternList(raw: string, fallback: string[]) {
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) {
      return fallback
    }
    return parsed
      .filter((item): item is string => typeof item === 'string')
      .map((item) => item.trim())
      .filter(Boolean)
  } catch {
    return fallback
  }
}

function patternListToText(raw: string, fallback: string[]) {
  return parsePatternList(raw, fallback).join('\n')
}

function patternTextToJson(value: string) {
  const patterns = value
    .split(/\r?\n|,/)
    .map((item) => item.trim())
    .filter(Boolean)
  return JSON.stringify(patterns)
}

function buildFormDefaults(defaultValues: TaskBillingSettingsDefaults) {
  return {
    durationBillingModelPatterns: patternListToText(
      defaultValues[DURATION_BILLING_MODEL_PATTERNS_KEY],
      recommendedDurationBillingPatterns
    ),
    durationBillingExcludeModelPatterns: patternListToText(
      defaultValues[DURATION_BILLING_EXCLUDE_MODEL_PATTERNS_KEY],
      recommendedDurationBillingExcludePatterns
    ),
    referenceVideoBillingModelPatterns: patternListToText(
      defaultValues[REFERENCE_VIDEO_BILLING_MODEL_PATTERNS_KEY],
      recommendedReferenceVideoBillingPatterns
    ),
  }
}

export function TaskBillingSettingsCard({ defaultValues }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const formDefaults = buildFormDefaults(defaultValues)

  const form = useForm<TaskBillingFormValues>({
    resolver: zodResolver(taskBillingSchema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  const fillRecommendedDefaults = () => {
    form.setValue(
      'durationBillingModelPatterns',
      recommendedDurationBillingPatterns.join('\n'),
      { shouldDirty: true }
    )
    form.setValue(
      'durationBillingExcludeModelPatterns',
      recommendedDurationBillingExcludePatterns.join('\n'),
      { shouldDirty: true }
    )
    form.setValue(
      'referenceVideoBillingModelPatterns',
      recommendedReferenceVideoBillingPatterns.join('\n'),
      { shouldDirty: true }
    )
  }

  const onSubmit = async (values: TaskBillingFormValues) => {
    const nextValues = {
      [DURATION_BILLING_MODEL_PATTERNS_KEY]: patternTextToJson(
        values.durationBillingModelPatterns
      ),
      [DURATION_BILLING_EXCLUDE_MODEL_PATTERNS_KEY]: patternTextToJson(
        values.durationBillingExcludeModelPatterns
      ),
      [REFERENCE_VIDEO_BILLING_MODEL_PATTERNS_KEY]: patternTextToJson(
        values.referenceVideoBillingModelPatterns
      ),
    }
    const currentValues = {
      [DURATION_BILLING_MODEL_PATTERNS_KEY]: patternTextToJson(
        formDefaults.durationBillingModelPatterns
      ),
      [DURATION_BILLING_EXCLUDE_MODEL_PATTERNS_KEY]: patternTextToJson(
        formDefaults.durationBillingExcludeModelPatterns
      ),
      [REFERENCE_VIDEO_BILLING_MODEL_PATTERNS_KEY]: patternTextToJson(
        formDefaults.referenceVideoBillingModelPatterns
      ),
    }
    const updates = Object.entries(nextValues).filter(
      ([key, value]) => value !== currentValues[key as keyof typeof nextValues]
    )

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value })
    }
  }

  return (
    <SettingsSection
      title={t('Task Billing')}
      description={t('Configure per-second billing rules for video task models')}
    >
      <Form {...form}>
        <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-6'>
          <FormField
            control={form.control}
            name='durationBillingModelPatterns'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Per-second billing models')}</FormLabel>
                <FormControl>
                  <Textarea rows={6} {...field} />
                </FormControl>
                <FormDescription>
                  {t('One model pattern per line. Use * as a wildcard.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='durationBillingExcludeModelPatterns'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Per-second billing exclusions')}</FormLabel>
                <FormControl>
                  <Textarea rows={5} {...field} />
                </FormControl>
                <FormDescription>
                  {t('Excluded patterns take priority over billing patterns.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='referenceVideoBillingModelPatterns'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Reference video billing models')}</FormLabel>
                <FormControl>
                  <Textarea rows={5} {...field} />
                </FormControl>
                <FormDescription>
                  {t(
                    'Models listed here add detected reference video seconds to the generated video duration.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={fillRecommendedDefaults}
            >
              <RotateCcw className='h-4 w-4' />
              {t('Use recommended defaults')}
            </Button>
            <Button type='submit' disabled={updateOption.isPending}>
              <Save className='h-4 w-4' />
              {updateOption.isPending ? t('Saving...') : t('Save Changes')}
            </Button>
          </div>
        </form>
      </Form>
    </SettingsSection>
  )
}
