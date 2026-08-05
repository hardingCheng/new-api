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
import {
  Add01Icon,
  Cancel01Icon,
  Delete02Icon,
  PencilEdit02Icon,
  Route01Icon,
  Search01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { TitledCard } from '@/components/ui/titled-card'
import { getChannel, searchChannels } from '@/features/channels/api'
import type { Channel } from '@/features/channels/types'

import {
  UserSearchPicker,
  type UserChoice,
} from '../billing/user-search-picker'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  parseUserChannelRouting,
  serializeUserChannelRouting,
  type UserChannelRoutingRule,
} from './user-channel-routing-config'

type RoutingFallback = 'strict' | 'default'

type SelectedChannelBadgesProps = {
  channelIDs: number[]
  channelsByID: Record<number, Channel | null>
  onRemove: (channelID: number) => void
}

export function SelectedChannelBadges(props: SelectedChannelBadgesProps) {
  const { t } = useTranslation()

  return (
    <div className='flex flex-wrap gap-1'>
      {props.channelIDs.map((id) => (
        <Badge key={id} variant='secondary' className='h-7 pr-1'>
          #{id} {props.channelsByID[id]?.name ?? ''}
          <Button
            type='button'
            variant='ghost'
            size='icon-xs'
            aria-label={t('Remove {{value}}', {
              value: props.channelsByID[id]?.name || `#${id}`,
            })}
            title={t('Remove')}
            onClick={() => props.onRemove(id)}
          >
            <HugeiconsIcon icon={Cancel01Icon} />
          </Button>
        </Badge>
      ))}
    </div>
  )
}

function createDraftRule(): UserChannelRoutingRule {
  return {
    id: '',
    name: '',
    user_id: 0,
    username: '',
    user_group: '',
    group_pattern: '',
    model_pattern: '*',
    channel_ids: [],
    fallback: 'strict',
    disabled: false,
  }
}

export function UserChannelRoutingSection(props: { defaultValue: string }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [rules, setRules] = useState(() =>
    parseUserChannelRouting(props.defaultValue)
  )
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingRuleID, setEditingRuleID] = useState('')
  const [selectedUsers, setSelectedUsers] = useState<UserChoice[]>([])
  const [draft, setDraft] = useState<UserChannelRoutingRule>(createDraftRule)
  const [deleteTarget, setDeleteTarget] =
    useState<UserChannelRoutingRule | null>(null)
  const [channelKeyword, setChannelKeyword] = useState('')
  const [channelResults, setChannelResults] = useState<Channel[]>([])
  const [channelsByID, setChannelsByID] = useState<
    Record<number, Channel | null>
  >({})
  const [channelLoading, setChannelLoading] = useState(false)

  useEffect(
    () => setRules(parseUserChannelRouting(props.defaultValue)),
    [props.defaultValue]
  )

  useEffect(() => {
    const missingIDs = [
      ...new Set(rules.flatMap((rule) => rule.channel_ids)),
    ].filter((id) => channelsByID[id] === undefined)
    if (missingIDs.length === 0) return

    let active = true
    void Promise.allSettled(missingIDs.map((id) => getChannel(id))).then(
      (responses) => {
        if (!active) return
        const loadedEntries = responses.map((response, index) => {
          if (
            response.status === 'fulfilled' &&
            response.value.success &&
            response.value.data
          ) {
            return [missingIDs[index], response.value.data] as const
          }
          return [missingIDs[index], null] as const
        })
        setChannelsByID((current) => ({
          ...current,
          ...Object.fromEntries(loadedEntries),
        }))
      }
    )
    return () => {
      active = false
    }
  }, [channelsByID, rules])

  const openCreateDialog = () => {
    setEditingRuleID('')
    setSelectedUsers([])
    setDraft(createDraftRule())
    setChannelKeyword('')
    setChannelResults([])
    setDialogOpen(true)
  }

  const openEditDialog = (rule: UserChannelRoutingRule) => {
    setEditingRuleID(rule.id)
    setSelectedUsers([
      { id: rule.user_id, username: rule.username, group: rule.user_group },
    ])
    setDraft({ ...rule, channel_ids: [...rule.channel_ids] })
    setChannelKeyword('')
    setChannelResults([])
    setDialogOpen(true)
  }

  const findChannels = async () => {
    setChannelLoading(true)
    try {
      const exactGroup = draft.group_pattern.includes('*')
        ? ''
        : draft.group_pattern
      const exactModel = draft.model_pattern.includes('*')
        ? ''
        : draft.model_pattern
      const response = await searchChannels({
        keyword: channelKeyword,
        group: exactGroup,
        model: exactModel,
        p: 1,
        page_size: 50,
      })
      if (!response.success) {
        toast.error(response.message || t('Failed to load channels'))
        return
      }
      const items = response.data?.items ?? []
      setChannelResults(items)
      setChannelsByID((current) => ({
        ...current,
        ...Object.fromEntries(items.map((channel) => [channel.id, channel])),
      }))
    } catch {
      toast.error(t('Failed to load channels'))
    } finally {
      setChannelLoading(false)
    }
  }

  const toggleChannel = (channel: Channel) => {
    setDraft((current) => {
      const selected = current.channel_ids.includes(channel.id)
      return {
        ...current,
        channel_ids: selected
          ? current.channel_ids.filter((id) => id !== channel.id)
          : [...current.channel_ids, channel.id],
      }
    })
  }

  const saveRule = () => {
    const user = selectedUsers[0]
    const groupPattern = draft.group_pattern.trim()
    const modelPattern = draft.model_pattern.trim() || '*'
    if (!user) {
      toast.error(t('Select one user'))
      return
    }
    if (!groupPattern) {
      toast.error(t('Using group is required'))
      return
    }
    if (groupPattern !== '*' && groupPattern.includes('*')) {
      toast.error(t('Using group must be an exact group or *'))
      return
    }
    if (draft.channel_ids.length === 0) {
      toast.error(t('Select at least one channel'))
      return
    }
    const duplicateScope = rules.some(
      (rule) =>
        rule.id !== editingRuleID &&
        rule.user_id === user.id &&
        rule.group_pattern === groupPattern &&
        rule.model_pattern === modelPattern
    )
    if (duplicateScope) {
      toast.error(t('This user already has a routing rule for this scope'))
      return
    }

    const id =
      editingRuleID ||
      `user-${user.id}-${groupPattern.replaceAll(/[^a-zA-Z0-9_-]/g, '-')}-${Date.now()}`
    const nextRule: UserChannelRoutingRule = {
      id,
      name:
        draft.name.trim() ||
        `${user.username || user.id} / ${groupPattern} / ${modelPattern}`,
      user_id: user.id,
      username: user.username,
      user_group: user.group,
      group_pattern: groupPattern,
      model_pattern: modelPattern,
      channel_ids: [...new Set(draft.channel_ids)].sort((a, b) => a - b),
      fallback: draft.fallback,
      disabled: draft.disabled,
    }
    const nextRules = [
      ...rules.filter((rule) => rule.id !== editingRuleID),
      nextRule,
    ].sort((left, right) =>
      `${left.user_id}-${left.group_pattern}-${left.model_pattern}`.localeCompare(
        `${right.user_id}-${right.group_pattern}-${right.model_pattern}`
      )
    )

    updateOption.mutate(
      {
        key: 'UserChannelRouting',
        value: serializeUserChannelRouting(nextRules),
      },
      {
        onSuccess: (data) => {
          if (!data.success) return
          setRules(nextRules)
          setDialogOpen(false)
        },
      }
    )
  }

  const deleteRule = () => {
    if (!deleteTarget) return
    const nextRules = rules.filter((rule) => rule.id !== deleteTarget.id)
    updateOption.mutate(
      {
        key: 'UserChannelRouting',
        value: serializeUserChannelRouting(nextRules),
      },
      {
        onSuccess: (data) => {
          if (!data.success) return
          setRules(nextRules)
          setDeleteTarget(null)
        },
      }
    )
  }

  return (
    <TitledCard
      title={t('User Channel Routing')}
      description={t(
        "Limit each user to selected channels within a using group, then apply the channels' existing priorities and weights."
      )}
      icon={<HugeiconsIcon icon={Route01Icon} />}
      action={
        <Button type='button' onClick={openCreateDialog}>
          <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
          {t('Add routing rule')}
        </Button>
      }
    >
      {rules.length === 0 ? (
        <Empty className='border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={Route01Icon} />
            </EmptyMedia>
            <EmptyTitle>{t('No user channel routing rules')}</EmptyTitle>
            <EmptyDescription>
              {t(
                'Users without a rule continue to use every eligible channel in the group.'
              )}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className='overflow-x-auto rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('User')}</TableHead>
                <TableHead>{t('Scope')}</TableHead>
                <TableHead>{t('Allowed channels')}</TableHead>
                <TableHead>{t('Failure policy')}</TableHead>
                <TableHead>{t('Status')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.map((rule) => (
                <TableRow key={rule.id}>
                  <TableCell>
                    <div className='font-medium'>{rule.username || '-'}</div>
                    <div className='text-muted-foreground text-xs'>
                      ID {rule.user_id} / {rule.user_group || '-'}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className='font-medium'>{rule.group_pattern}</div>
                    <div className='text-muted-foreground text-xs'>
                      {rule.model_pattern}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className='flex max-w-96 flex-wrap gap-1'>
                      {rule.channel_ids.map((id) => (
                        <Badge key={id} variant='outline'>
                          #{id} {channelsByID[id]?.name ?? ''}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>
                    {rule.fallback === 'strict'
                      ? t('Strict failure')
                      : t('Use group defaults')}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={rule.disabled ? 'destructive' : 'secondary'}
                    >
                      {rule.disabled ? t('Disabled') : t('Enabled')}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <div className='flex justify-end gap-1'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Edit')}
                        title={t('Edit')}
                        onClick={() => openEditDialog(rule)}
                      >
                        <HugeiconsIcon icon={PencilEdit02Icon} />
                      </Button>
                      <Button
                        type='button'
                        variant='ghost'
                        size='icon-sm'
                        aria-label={t('Delete')}
                        title={t('Delete')}
                        onClick={() => setDeleteTarget(rule)}
                      >
                        <HugeiconsIcon icon={Delete02Icon} />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <Dialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={editingRuleID ? t('Edit routing rule') : t('Add routing rule')}
        description={t(
          'The selected channels must already support the using group and requested model.'
        )}
        contentClassName='sm:max-w-4xl'
        contentHeight='min(76vh, 48rem)'
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              disabled={updateOption.isPending}
              onClick={() => setDialogOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              disabled={updateOption.isPending}
              onClick={saveRule}
            >
              {updateOption.isPending ? (
                <Spinner data-icon='inline-start' />
              ) : null}
              {updateOption.isPending ? t('Saving...') : t('Save Settings')}
            </Button>
          </>
        }
      >
        <FieldGroup>
          <Field>
            <FieldLabel>{t('User')}</FieldLabel>
            <UserSearchPicker
              value={selectedUsers}
              onChange={setSelectedUsers}
            />
            <FieldDescription>
              {t('This rule is matched by exact user ID.')}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor='routing-rule-name'>
              {t('Rule name')}
            </FieldLabel>
            <Input
              id='routing-rule-name'
              value={draft.name}
              placeholder={t('Generated automatically when left empty')}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  name: event.target.value,
                }))
              }
            />
          </Field>

          <div className='grid gap-4 sm:grid-cols-2'>
            <Field>
              <FieldLabel htmlFor='routing-using-group'>
                {t('Using group')}
              </FieldLabel>
              <Input
                id='routing-using-group'
                value={draft.group_pattern}
                placeholder='sd2'
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    group_pattern: event.target.value,
                  }))
                }
              />
            </Field>
            <Field>
              <FieldLabel htmlFor='routing-model-pattern'>
                {t('Model pattern')}
              </FieldLabel>
              <Input
                id='routing-model-pattern'
                value={draft.model_pattern}
                placeholder='*'
                onChange={(event) =>
                  setDraft((current) => ({
                    ...current,
                    model_pattern: event.target.value,
                  }))
                }
              />
              <FieldDescription>
                {t('Use * for every model in the using group.')}
              </FieldDescription>
            </Field>
          </div>

          <Field>
            <FieldLabel>{t('Allowed channels')}</FieldLabel>
            <div className='flex gap-2'>
              <Input
                value={channelKeyword}
                placeholder={t('Search channels')}
                onChange={(event) => setChannelKeyword(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    void findChannels()
                  }
                }}
              />
              <Button
                type='button'
                variant='outline'
                size='icon'
                aria-label={t('Search')}
                title={t('Search')}
                disabled={channelLoading}
                onClick={() => void findChannels()}
              >
                {channelLoading ? (
                  <Spinner />
                ) : (
                  <HugeiconsIcon icon={Search01Icon} />
                )}
              </Button>
            </div>
            {draft.channel_ids.length > 0 ? (
              <SelectedChannelBadges
                channelIDs={draft.channel_ids}
                channelsByID={channelsByID}
                onRemove={(id) =>
                  setDraft((current) => ({
                    ...current,
                    channel_ids: current.channel_ids.filter(
                      (channelID) => channelID !== id
                    ),
                  }))
                }
              />
            ) : null}
            {channelResults.length > 0 ? (
              <div className='max-h-64 overflow-y-auto rounded-lg border'>
                {channelResults.map((channel) => {
                  const selected = draft.channel_ids.includes(channel.id)
                  return (
                    <label
                      key={channel.id}
                      className='hover:bg-muted flex cursor-pointer items-start gap-3 border-b px-3 py-2 last:border-b-0'
                    >
                      <Checkbox
                        checked={selected}
                        onCheckedChange={() => toggleChannel(channel)}
                        aria-label={t('Select channel {{channel}}', {
                          channel: channel.name,
                        })}
                      />
                      <span className='min-w-0 flex-1'>
                        <span className='block truncate text-sm font-medium'>
                          #{channel.id} {channel.name}
                        </span>
                        <span className='text-muted-foreground block text-xs'>
                          {channel.group} / {t('Priority')}:{' '}
                          {channel.priority ?? 0} / {t('Weight')}:{' '}
                          {channel.weight ?? 0}
                        </span>
                      </span>
                    </label>
                  )
                })}
              </div>
            ) : null}
            <FieldDescription>
              {t(
                'Only these channels participate in the existing priority and weight selection.'
              )}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor='routing-fallback'>
              {t('Failure policy')}
            </FieldLabel>
            <NativeSelect
              id='routing-fallback'
              value={draft.fallback}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  fallback: event.target.value as RoutingFallback,
                }))
              }
            >
              <NativeSelectOption value='strict'>
                {t('Strict failure')}
              </NativeSelectOption>
              <NativeSelectOption value='default'>
                {t('Use group defaults')}
              </NativeSelectOption>
            </NativeSelect>
            <FieldDescription>
              {draft.fallback === 'strict'
                ? t('Return 503 when every selected channel is unavailable.')
                : t(
                    'Use every eligible channel in the group when the selected channels are unavailable.'
                  )}
            </FieldDescription>
          </Field>

          <Field orientation='horizontal'>
            <FieldContent>
              <FieldTitle>{t('Enabled')}</FieldTitle>
              <FieldDescription>
                {t('Disable the routing rule without deleting it.')}
              </FieldDescription>
            </FieldContent>
            <Switch
              checked={!draft.disabled}
              onCheckedChange={(checked) =>
                setDraft((current) => ({
                  ...current,
                  disabled: !checked,
                }))
              }
              aria-label={t('Enabled')}
            />
          </Field>
        </FieldGroup>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        title={t('Delete routing rule')}
        desc={t('This will remove routing rule {{rule}}.', {
          rule: deleteTarget?.name || '-',
        })}
        confirmText={t('Delete')}
        destructive
        isLoading={updateOption.isPending}
        handleConfirm={deleteRule}
      />
    </TitledCard>
  )
}
