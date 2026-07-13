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
import { zodResolver } from '@hookform/resolvers/zod'
import { Plus, Trash2 } from 'lucide-react'
import { useEffect } from 'react'
import { useFieldArray, useForm, type Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { Button } from '@/components/design-system/button'
import { Input } from '@/components/design-system/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/design-system/select'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const ANNOUNCEMENT_TYPES = [
  'default',
  'ongoing',
  'success',
  'warning',
  'error',
] as const

const ANNOUNCEMENT_TYPE_LABELS: Record<
  (typeof ANNOUNCEMENT_TYPES)[number],
  string
> = {
  default: 'Default',
  ongoing: 'Ongoing',
  success: 'Success',
  warning: 'Warning',
  error: 'Error',
}

const announcementSchema = z.object({
  content: z.string().min(1),
  type: z.enum(ANNOUNCEMENT_TYPES),
  publishDate: z.string().optional(),
})

const stationSchema = z.object({
  domain: z.string().trim().min(1),
  group: z.string(),
  githubClientId: z.string(),
  githubClientSecret: z.string(),
  systemName: z.string(),
  logo: z.string(),
  footer: z.string(),
  homePageContent: z.string(),
  notice: z.string(),
  about: z.string(),
  announcements: z.array(announcementSchema),
})

const stationsFormSchema = z.object({
  stations: z.array(stationSchema),
})

type StationsFormValues = z.infer<typeof stationsFormSchema>
type StationValues = StationsFormValues['stations'][number]

type RawStationAnnouncement = {
  content?: string
  type?: string
  publishDate?: string
}

type RawStationConfig = {
  group?: string
  oauth?: Record<string, { client_id?: string; client_secret?: string }>
  brand?: {
    system_name?: string
    logo?: string
    footer?: string
    home_page_content?: string
    notice?: string
    about?: string
    announcements?: RawStationAnnouncement[]
  }
}

function emptyStation(): StationValues {
  return {
    domain: '',
    group: '',
    githubClientId: '',
    githubClientSecret: '',
    systemName: '',
    logo: '',
    footer: '',
    homePageContent: '',
    notice: '',
    about: '',
    announcements: [],
  }
}

function isAnnouncementType(
  value: string | undefined
): value is (typeof ANNOUNCEMENT_TYPES)[number] {
  return (ANNOUNCEMENT_TYPES as readonly string[]).includes(value ?? '')
}

function parseStations(raw: string): StationValues[] {
  let parsed: Record<string, RawStationConfig>
  try {
    parsed = JSON.parse(raw || '{}') as Record<string, RawStationConfig>
  } catch {
    return []
  }
  if (!parsed || typeof parsed !== 'object') {
    return []
  }
  return Object.entries(parsed).map(([domain, config]) => ({
    domain,
    group: config?.group ?? '',
    githubClientId: config?.oauth?.github?.client_id ?? '',
    githubClientSecret: config?.oauth?.github?.client_secret ?? '',
    systemName: config?.brand?.system_name ?? '',
    logo: config?.brand?.logo ?? '',
    footer: config?.brand?.footer ?? '',
    homePageContent: config?.brand?.home_page_content ?? '',
    notice: config?.brand?.notice ?? '',
    about: config?.brand?.about ?? '',
    announcements: (config?.brand?.announcements ?? []).map((item) => ({
      content: item?.content ?? '',
      type: isAnnouncementType(item?.type) ? item.type : 'default',
      publishDate: item?.publishDate,
    })),
  }))
}

function serializeStations(stations: StationValues[]): string {
  const result: Record<string, RawStationConfig> = {}
  for (const station of stations) {
    const domain = station.domain.trim().toLowerCase()
    const config: RawStationConfig = {}
    if (station.group.trim()) {
      config.group = station.group.trim()
    }
    if (station.githubClientId.trim() || station.githubClientSecret.trim()) {
      config.oauth = {
        github: {
          client_id: station.githubClientId.trim(),
          client_secret: station.githubClientSecret.trim(),
        },
      }
    }
    const brand: NonNullable<RawStationConfig['brand']> = {}
    if (station.systemName.trim()) brand.system_name = station.systemName.trim()
    if (station.logo.trim()) brand.logo = station.logo.trim()
    if (station.footer.trim()) brand.footer = station.footer
    if (station.homePageContent.trim()) {
      brand.home_page_content = station.homePageContent
    }
    if (station.notice.trim()) brand.notice = station.notice
    if (station.about.trim()) brand.about = station.about
    const announcements = station.announcements
      .filter((item) => item.content.trim())
      .map((item) => ({
        content: item.content,
        type: item.type,
        publishDate: item.publishDate || new Date().toISOString(),
      }))
    if (announcements.length > 0) {
      brand.announcements = announcements
    }
    if (Object.keys(brand).length > 0) {
      config.brand = brand
    }
    result[domain] = config
  }
  return JSON.stringify(result)
}

type StationAnnouncementsProps = {
  control: Control<StationsFormValues>
  stationIndex: number
}

function StationAnnouncements({
  control,
  stationIndex,
}: StationAnnouncementsProps) {
  const { t } = useTranslation()
  const { fields, append, remove } = useFieldArray({
    control,
    name: `stations.${stationIndex}.announcements`,
  })

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between'>
        <span className='text-sm font-medium'>
          {t('Console announcements')}
        </span>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() =>
            append({ content: '', type: 'default', publishDate: '' })
          }
        >
          <Plus className='mr-1 h-4 w-4' />
          {t('Add announcement')}
        </Button>
      </div>
      {fields.map((field, index) => (
        <div key={field.id} className='flex items-start gap-2'>
          <FormField
            control={control}
            name={`stations.${stationIndex}.announcements.${index}.content`}
            render={({ field: contentField }) => (
              <FormItem className='flex-1'>
                <FormControl>
                  <Input
                    placeholder={t('Announcement content')}
                    {...contentField}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={control}
            name={`stations.${stationIndex}.announcements.${index}.type`}
            render={({ field: typeField }) => (
              <FormItem>
                <Select
                  value={typeField.value}
                  onValueChange={typeField.onChange}
                >
                  <FormControl>
                    <SelectTrigger className='w-28'>
                      <SelectValue placeholder={t('Type')} />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectGroup>
                      {ANNOUNCEMENT_TYPES.map((type) => (
                        <SelectItem key={type} value={type}>
                          {t(ANNOUNCEMENT_TYPE_LABELS[type])}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </FormItem>
            )}
          />
          <Button
            type='button'
            variant='ghost'
            size='icon'
            onClick={() => remove(index)}
          >
            <Trash2 className='h-4 w-4' />
          </Button>
        </div>
      ))}
    </div>
  )
}

type StationCardProps = {
  control: Control<StationsFormValues>
  index: number
  domainLabel: string
  onRemove: () => void
}

function StationCard({
  control,
  index,
  domainLabel,
  onRemove,
}: StationCardProps) {
  const { t } = useTranslation()

  const textField = (
    name:
      | `stations.${number}.footer`
      | `stations.${number}.homePageContent`
      | `stations.${number}.notice`
      | `stations.${number}.about`,
    label: string,
    rows: number
  ) => (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Textarea rows={rows} {...field} />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )

  return (
    <Card>
      <CardHeader className='flex flex-row items-center justify-between space-y-0'>
        <CardTitle className='text-base'>
          {domainLabel || t('New station')}
        </CardTitle>
        <Button type='button' variant='ghost' size='sm' onClick={onRemove}>
          <Trash2 className='mr-1 h-4 w-4' />
          {t('Remove station')}
        </Button>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='grid grid-cols-1 gap-4 md:grid-cols-2'>
          <FormField
            control={control}
            name={`stations.${index}.domain`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Domain')}</FormLabel>
                <FormControl>
                  <Input placeholder='example.com' {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={control}
            name={`stations.${index}.group`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Register group')}</FormLabel>
                <FormControl>
                  <Input {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={control}
            name={`stations.${index}.systemName`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Site name')}</FormLabel>
                <FormControl>
                  <Input {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={control}
            name={`stations.${index}.logo`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Logo URL')}</FormLabel>
                <FormControl>
                  <Input placeholder='https://' {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={control}
            name={`stations.${index}.githubClientId`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('GitHub Client ID')}</FormLabel>
                <FormControl>
                  <Input {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={control}
            name={`stations.${index}.githubClientSecret`}
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('GitHub Client Secret')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>
        {textField(`stations.${index}.footer`, t('Footer'), 2)}
        {textField(
          `stations.${index}.homePageContent`,
          t('Homepage content'),
          5
        )}
        {textField(`stations.${index}.notice`, t('System Notice'), 3)}
        {textField(`stations.${index}.about`, t('About page'), 3)}
        <StationAnnouncements control={control} stationIndex={index} />
      </CardContent>
    </Card>
  )
}

type StationsSectionProps = {
  defaultValue: string
}

export function StationsSection({ defaultValue }: StationsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<StationsFormValues>({
    resolver: zodResolver(stationsFormSchema),
    defaultValues: {
      stations: parseStations(defaultValue),
    },
  })

  useEffect(() => {
    form.reset({ stations: parseStations(defaultValue) })
  }, [defaultValue, form])

  const { fields, append, remove } = useFieldArray({
    control: form.control,
    name: 'stations',
  })
  const watchedStations = form.watch('stations')

  const onSubmit = async (values: StationsFormValues) => {
    const domains = values.stations.map((s) => s.domain.trim().toLowerCase())
    if (new Set(domains).size !== domains.length) {
      toast.error(t('Duplicate domain'))
      return
    }
    const serialized = serializeStations(values.stations)
    if (serialized === defaultValue) {
      return
    }
    await updateOption.mutateAsync({
      key: 'StationConfigs',
      value: serialized,
    })
  }

  return (
    <SettingsSection title={t('Station Management')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Each station is keyed by its domain. Empty fields fall back to the global settings.'
        )}
      </p>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save stations'
          />
          <div className='space-y-4'>
            {fields.length === 0 && (
              <p className='text-muted-foreground text-sm'>
                {t('No stations configured yet')}
              </p>
            )}
            {fields.map((field, index) => (
              <StationCard
                key={field.id}
                control={form.control}
                index={index}
                domainLabel={watchedStations?.[index]?.domain ?? ''}
                onRemove={() => remove(index)}
              />
            ))}
            <Button
              type='button'
              variant='outline'
              onClick={() => append(emptyStation())}
            >
              <Plus className='mr-1 h-4 w-4' />
              {t('Add station')}
            </Button>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
