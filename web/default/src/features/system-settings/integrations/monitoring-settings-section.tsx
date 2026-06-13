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
import { useMemo, useRef } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { safeNumberFieldProps } from '../utils/numeric-field'

const numericString = z.string().refine((value) => {
  const trimmed = value.trim()
  if (!trimmed) return true
  return !Number.isNaN(Number(trimmed)) && Number(trimmed) >= 0
}, 'Enter a non-negative number or leave empty')

const monitoringSchema = z
  .object({
    ChannelDisableThreshold: numericString,
    QuotaRemindThreshold: numericString,
    AutomaticDisableChannelEnabled: z.boolean(),
    AutomaticEnableChannelEnabled: z.boolean(),
    ChannelBreakerEnabled: z.boolean(),
    AutomaticDisableKeywords: z.string(),
    AutomaticDisableStatusCodes: z.string(),
    AutomaticRetryStatusCodes: z.string(),
    ChannelBreakerFailureLimit: numericString,
    ChannelBreakerCooldownSeconds: numericString,
    ChannelBreakerProbeCount: numericString,
    ChannelBreakerProbeSuccessCount: numericString,
    ChannelBreakerExcludePaths: z.string(),
    monitor_setting: z.object({
      auto_test_channel_enabled: z.boolean(),
      auto_test_channel_minutes: z.coerce
        .number()
        .int()
        .min(1, 'Interval must be at least 1 minute'),
      channel_disable_alert_enabled: z.boolean(),
      channel_disable_alert_cooldown_second: z.coerce
        .number()
        .int()
        .min(0, 'Cooldown must be zero or greater'),
      retest_disabled_channel_enabled: z.boolean(),
      retest_disabled_channel_seconds: z.coerce
        .number()
        .int()
        .min(5, 'Interval must be at least 5 seconds'),
    }),
  })
  .superRefine((values, ctx) => {
    const disableParsed = parseHttpStatusCodeRules(
      values.AutomaticDisableStatusCodes
    )
    if (!disableParsed.ok) {
      ctx.addIssue({
        code: 'custom',
        path: ['AutomaticDisableStatusCodes'],
        message: `Invalid status code rules: ${disableParsed.invalidTokens.join(
          ', '
        )}`,
      })
    }

    const retryParsed = parseHttpStatusCodeRules(
      values.AutomaticRetryStatusCodes
    )
    if (!retryParsed.ok) {
      ctx.addIssue({
        code: 'custom',
        path: ['AutomaticRetryStatusCodes'],
        message: `Invalid status code rules: ${retryParsed.invalidTokens.join(
          ', '
        )}`,
      })
    }
  })

type MonitoringFormValues = z.output<typeof monitoringSchema>
type MonitoringFormInput = z.input<typeof monitoringSchema>

type MonitoringSettingsSectionProps = {
  defaultValues: {
    ChannelDisableThreshold: string
    QuotaRemindThreshold: string
    AutomaticDisableChannelEnabled: boolean
    AutomaticEnableChannelEnabled: boolean
    ChannelBreakerEnabled: boolean
    AutomaticDisableKeywords: string
    AutomaticDisableStatusCodes: string
    AutomaticRetryStatusCodes: string
    ChannelBreakerFailureLimit: string
    ChannelBreakerCooldownSeconds: string
    ChannelBreakerProbeCount: string
    ChannelBreakerProbeSuccessCount: string
    ChannelBreakerExcludePaths: string
    'monitor_setting.auto_test_channel_enabled': boolean
    'monitor_setting.auto_test_channel_minutes': number
    'monitor_setting.channel_disable_alert_enabled': boolean
    'monitor_setting.channel_disable_alert_cooldown_second': number
    'monitor_setting.retest_disabled_channel_enabled': boolean
    'monitor_setting.retest_disabled_channel_seconds': number
  }
}

function normalizeLineEndings(value: string) {
  return value.replace(/\r\n/g, '\n')
}

type NormalizedMonitoringValues = {
  ChannelDisableThreshold: string
  QuotaRemindThreshold: string
  AutomaticDisableChannelEnabled: boolean
  AutomaticEnableChannelEnabled: boolean
  ChannelBreakerEnabled: boolean
  AutomaticDisableKeywords: string
  AutomaticDisableStatusCodes: string
  AutomaticRetryStatusCodes: string
  ChannelBreakerFailureLimit: string
  ChannelBreakerCooldownSeconds: string
  ChannelBreakerProbeCount: string
  ChannelBreakerProbeSuccessCount: string
  ChannelBreakerExcludePaths: string
  'monitor_setting.auto_test_channel_enabled': boolean
  'monitor_setting.auto_test_channel_minutes': number
  'monitor_setting.channel_disable_alert_enabled': boolean
  'monitor_setting.channel_disable_alert_cooldown_second': number
  'monitor_setting.retest_disabled_channel_enabled': boolean
  'monitor_setting.retest_disabled_channel_seconds': number
}

const buildFormDefaults = (
  defaults: MonitoringSettingsSectionProps['defaultValues']
): MonitoringFormInput => ({
  ChannelDisableThreshold: defaults.ChannelDisableThreshold ?? '',
  QuotaRemindThreshold: defaults.QuotaRemindThreshold ?? '',
  AutomaticDisableChannelEnabled: defaults.AutomaticDisableChannelEnabled,
  AutomaticEnableChannelEnabled: defaults.AutomaticEnableChannelEnabled,
  ChannelBreakerEnabled: defaults.ChannelBreakerEnabled,
  AutomaticDisableKeywords: normalizeLineEndings(
    defaults.AutomaticDisableKeywords ?? ''
  ),
  AutomaticDisableStatusCodes: defaults.AutomaticDisableStatusCodes ?? '',
  AutomaticRetryStatusCodes: defaults.AutomaticRetryStatusCodes ?? '',
  ChannelBreakerFailureLimit: defaults.ChannelBreakerFailureLimit ?? '',
  ChannelBreakerCooldownSeconds: defaults.ChannelBreakerCooldownSeconds ?? '',
  ChannelBreakerProbeCount: defaults.ChannelBreakerProbeCount ?? '',
  ChannelBreakerProbeSuccessCount:
    defaults.ChannelBreakerProbeSuccessCount ?? '',
  ChannelBreakerExcludePaths: normalizeLineEndings(
    defaults.ChannelBreakerExcludePaths ?? ''
  ),
  monitor_setting: {
    auto_test_channel_enabled:
      defaults['monitor_setting.auto_test_channel_enabled'],
    auto_test_channel_minutes:
      defaults['monitor_setting.auto_test_channel_minutes'],
    channel_disable_alert_enabled:
      defaults['monitor_setting.channel_disable_alert_enabled'],
    channel_disable_alert_cooldown_second:
      defaults['monitor_setting.channel_disable_alert_cooldown_second'],
    retest_disabled_channel_enabled:
      defaults['monitor_setting.retest_disabled_channel_enabled'],
    retest_disabled_channel_seconds:
      defaults['monitor_setting.retest_disabled_channel_seconds'],
  },
})

const normalizeDefaults = (
  defaults: MonitoringSettingsSectionProps['defaultValues']
): NormalizedMonitoringValues => ({
  ChannelDisableThreshold: (defaults.ChannelDisableThreshold ?? '').trim(),
  QuotaRemindThreshold: (defaults.QuotaRemindThreshold ?? '').trim(),
  AutomaticDisableChannelEnabled: defaults.AutomaticDisableChannelEnabled,
  AutomaticEnableChannelEnabled: defaults.AutomaticEnableChannelEnabled,
  ChannelBreakerEnabled: defaults.ChannelBreakerEnabled,
  AutomaticDisableKeywords: normalizeLineEndings(
    defaults.AutomaticDisableKeywords ?? ''
  ),
  AutomaticDisableStatusCodes: parseHttpStatusCodeRules(
    defaults.AutomaticDisableStatusCodes ?? ''
  ).normalized,
  AutomaticRetryStatusCodes: parseHttpStatusCodeRules(
    defaults.AutomaticRetryStatusCodes ?? ''
  ).normalized,
  ChannelBreakerFailureLimit: (
    defaults.ChannelBreakerFailureLimit ?? ''
  ).trim(),
  ChannelBreakerCooldownSeconds: (
    defaults.ChannelBreakerCooldownSeconds ?? ''
  ).trim(),
  ChannelBreakerProbeCount: (defaults.ChannelBreakerProbeCount ?? '').trim(),
  ChannelBreakerProbeSuccessCount: (
    defaults.ChannelBreakerProbeSuccessCount ?? ''
  ).trim(),
  ChannelBreakerExcludePaths: normalizeLineEndings(
    defaults.ChannelBreakerExcludePaths ?? ''
  ),
  'monitor_setting.auto_test_channel_enabled':
    defaults['monitor_setting.auto_test_channel_enabled'],
  'monitor_setting.auto_test_channel_minutes':
    defaults['monitor_setting.auto_test_channel_minutes'],
  'monitor_setting.channel_disable_alert_enabled':
    defaults['monitor_setting.channel_disable_alert_enabled'],
  'monitor_setting.channel_disable_alert_cooldown_second':
    defaults['monitor_setting.channel_disable_alert_cooldown_second'],
  'monitor_setting.retest_disabled_channel_enabled':
    defaults['monitor_setting.retest_disabled_channel_enabled'],
  'monitor_setting.retest_disabled_channel_seconds':
    defaults['monitor_setting.retest_disabled_channel_seconds'],
})

const normalizeFormValues = (
  values: MonitoringFormValues
): NormalizedMonitoringValues => ({
  ChannelDisableThreshold: values.ChannelDisableThreshold.trim(),
  QuotaRemindThreshold: values.QuotaRemindThreshold.trim(),
  AutomaticDisableChannelEnabled: values.AutomaticDisableChannelEnabled,
  AutomaticEnableChannelEnabled: values.AutomaticEnableChannelEnabled,
  ChannelBreakerEnabled: values.ChannelBreakerEnabled,
  AutomaticDisableKeywords: normalizeLineEndings(
    values.AutomaticDisableKeywords
  ),
  AutomaticDisableStatusCodes: parseHttpStatusCodeRules(
    values.AutomaticDisableStatusCodes
  ).normalized,
  AutomaticRetryStatusCodes: parseHttpStatusCodeRules(
    values.AutomaticRetryStatusCodes
  ).normalized,
  ChannelBreakerFailureLimit: values.ChannelBreakerFailureLimit.trim(),
  ChannelBreakerCooldownSeconds: values.ChannelBreakerCooldownSeconds.trim(),
  ChannelBreakerProbeCount: values.ChannelBreakerProbeCount.trim(),
  ChannelBreakerProbeSuccessCount:
    values.ChannelBreakerProbeSuccessCount.trim(),
  ChannelBreakerExcludePaths: normalizeLineEndings(
    values.ChannelBreakerExcludePaths
  ),
  'monitor_setting.auto_test_channel_enabled':
    values.monitor_setting.auto_test_channel_enabled,
  'monitor_setting.auto_test_channel_minutes':
    values.monitor_setting.auto_test_channel_minutes,
  'monitor_setting.channel_disable_alert_enabled':
    values.monitor_setting.channel_disable_alert_enabled,
  'monitor_setting.channel_disable_alert_cooldown_second':
    values.monitor_setting.channel_disable_alert_cooldown_second,
  'monitor_setting.retest_disabled_channel_enabled':
    values.monitor_setting.retest_disabled_channel_enabled,
  'monitor_setting.retest_disabled_channel_seconds':
    values.monitor_setting.retest_disabled_channel_seconds,
})

export function MonitoringSettingsSection({
  defaultValues,
}: MonitoringSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const baselineRef = useRef<NormalizedMonitoringValues>(
    normalizeDefaults(defaultValues)
  )

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<MonitoringFormInput, unknown, MonitoringFormValues>({
    resolver: zodResolver(monitoringSchema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  const autoDisableStatusCodes = form.watch('AutomaticDisableStatusCodes')
  const autoRetryStatusCodes = form.watch('AutomaticRetryStatusCodes')
  const autoDisableParsed = useMemo(
    () => parseHttpStatusCodeRules(autoDisableStatusCodes),
    [autoDisableStatusCodes]
  )
  const autoRetryParsed = useMemo(
    () => parseHttpStatusCodeRules(autoRetryStatusCodes),
    [autoRetryStatusCodes]
  )

  const onSubmit = async (values: MonitoringFormValues) => {
    const normalized = normalizeFormValues(values)
    const updates = (
      Object.keys(normalized) as Array<keyof NormalizedMonitoringValues>
    ).filter((key) => normalized[key] !== baselineRef.current[key])

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      const value = normalized[key]
      await updateOption.mutateAsync({
        key,
        value,
      })
    }

    baselineRef.current = normalized
  }

  return (
    <SettingsSection title={t('Monitoring & Alerts')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save monitoring rules'
          />
          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='monitor_setting.auto_test_channel_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Scheduled channel tests')}</FormLabel>
                    <FormDescription>
                      {t('Automatically probe all channels in the background')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='monitor_setting.auto_test_channel_minutes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Test interval (minutes)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('How frequently the system tests all channels')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='monitor_setting.retest_disabled_channel_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Re-test disabled channels')}</FormLabel>
                    <FormDescription>
                      {t('Frequently re-test auto-disabled and circuit-broken channels so they recover quickly')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='monitor_setting.retest_disabled_channel_seconds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Re-test interval (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={5}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('How frequently disabled channels are re-tested for recovery')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='monitor_setting.channel_disable_alert_enabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Channel disabled alert')}</FormLabel>
                    <FormDescription>
                      {t('Send a Bark critical alert when a channel is automatically disabled (e.g. out of balance)')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='monitor_setting.channel_disable_alert_cooldown_second'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Disabled alert cooldown (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      {...safeNumberFieldProps(field)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Minimum interval between disable alerts for the same channel')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='ChannelDisableThreshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Disable threshold (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Automatically disable channels exceeding this response time'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='QuotaRemindThreshold'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Quota reminder (tokens)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={0}
                      step={1}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Send email alerts when a user falls below this quota')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AutomaticDisableChannelEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Auto-disable channels')}</FormLabel>
                    <FormDescription>
                      {t('Change channel status to auto-disabled when automatic checks fail')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />

            <FormField
              control={form.control}
              name='ChannelBreakerEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Circuit breaker on failure')}</FormLabel>
                    <FormDescription>
                      {t('Temporarily skip unhealthy channels without changing their enabled status')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AutomaticEnableChannelEnabled'
              render={({ field }) => (
                <SettingsSwitchItem>
                  <SettingsSwitchContent>
                    <FormLabel>{t('Re-enable on success')}</FormLabel>
                    <FormDescription>
                      {t('Bring channels back online after successful checks')}
                    </FormDescription>
                  </SettingsSwitchContent>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </SettingsSwitchItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='ChannelBreakerFailureLimit'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Breaker failure limit')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Open the breaker after this many consecutive failures')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ChannelBreakerCooldownSeconds'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Breaker cooldown (seconds)')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Wait this long before allowing real probe requests')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='ChannelBreakerProbeCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Probe request count')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Maximum real requests allowed while half-open')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ChannelBreakerProbeSuccessCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Probe success requirement')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      step={1}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Close the breaker after this many probe successes')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='ChannelBreakerExcludePaths'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Breaker excluded paths')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={3}
                    placeholder={t('one path prefix per line')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t('Requests matching these path prefixes will not affect breaker state')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='AutomaticDisableKeywords'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Failure keywords')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={6}
                    placeholder={t('one keyword per line')}
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'If an upstream error contains any of these keywords, it is treated as a channel failure.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='grid gap-6 md:grid-cols-2'>
            <FormField
              control={form.control}
              name='AutomaticDisableStatusCodes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Channel failure status codes')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('e.g. 401, 403, 429, 500-599')}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Accepts comma-separated status codes and inclusive ranges.'
                    )}{' '}
                    {autoDisableParsed.ok &&
                      autoDisableParsed.normalized &&
                      autoDisableParsed.normalized !== field.value.trim() && (
                        <span className='text-muted-foreground'>
                          {t('Normalized:')} {autoDisableParsed.normalized}
                        </span>
                      )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='AutomaticRetryStatusCodes'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Auto-retry status codes')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('e.g. 401, 403, 429, 500-599')}
                      value={field.value}
                      onChange={(event) => field.onChange(event.target.value)}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Accepts comma-separated status codes and inclusive ranges.'
                    )}{' '}
                    {autoRetryParsed.ok &&
                      autoRetryParsed.normalized &&
                      autoRetryParsed.normalized !== field.value.trim() && (
                        <span className='text-muted-foreground'>
                          {t('Normalized:')} {autoRetryParsed.normalized}
                        </span>
                      )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
