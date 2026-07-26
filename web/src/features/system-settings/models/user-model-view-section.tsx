import {
  Add01Icon,
  AiVideoIcon,
  Copy01Icon,
  Delete02Icon,
  PencilEdit02Icon,
  UserSettings01Icon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
  FieldLegend,
  FieldSet,
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

import {
  UserSearchPicker,
  type UserChoice,
} from '../billing/user-search-picker'
import { useUpdateOption } from '../hooks/use-update-option'

type ReferenceVideoPolicy = 'allowed' | 'forbidden'

type UserModelAlias = {
  public_model: string
  target_model: string
  reference_video: ReferenceVideoPolicy
  editor_key: string
}

type UserModelViewRule = {
  user_id: number
  username: string
  user_group: string
  disabled: boolean
  aliases: UserModelAlias[]
}

type StoredUserModelAlias = Omit<UserModelAlias, 'editor_key'>

let aliasEditorKey = 0

function createEditableAlias(alias: StoredUserModelAlias): UserModelAlias {
  aliasEditorKey += 1
  return {
    ...alias,
    editor_key: `user-model-alias-${aliasEditorKey}`,
  }
}

const seedanceTemplate: StoredUserModelAlias[] = [
  {
    public_model: '521ai-2.0-sp-1080p',
    target_model: 'seedance-2.0-1080p',
    reference_video: 'allowed',
  },
  {
    public_model: '521ai-2.0-sp-480p',
    target_model: 'seedance-2.0-480p',
    reference_video: 'allowed',
  },
  {
    public_model: '521ai-2.0-sp-720p',
    target_model: 'seedance-2.0-720p',
    reference_video: 'allowed',
  },
  {
    public_model: '521ai-2.0-sp-QD-480p',
    target_model: 'seedance-2.0-fast-480p',
    reference_video: 'allowed',
  },
  {
    public_model: '521ai-2.0-sp-QD-720p',
    target_model: 'seedance-2.0-fast-720p',
    reference_video: 'allowed',
  },
  {
    public_model: '521ai-2.0-1080p',
    target_model: 'seedance-2.0-1080p',
    reference_video: 'forbidden',
  },
  {
    public_model: '521ai-2.0-480p',
    target_model: 'seedance-2.0-480p',
    reference_video: 'forbidden',
  },
  {
    public_model: '521ai-2.0-720p',
    target_model: 'seedance-2.0-720p',
    reference_video: 'forbidden',
  },
  {
    public_model: '521ai-2.0-QD-480p',
    target_model: 'seedance-2.0-fast-480p',
    reference_video: 'forbidden',
  },
  {
    public_model: '521ai-2.0-QD-720p',
    target_model: 'seedance-2.0-fast-720p',
    reference_video: 'forbidden',
  },
]

function cloneSeedanceTemplate() {
  return seedanceTemplate.map(createEditableAlias)
}

function parseUserModelView(rawValue: string): UserModelViewRule[] {
  try {
    const parsed = JSON.parse(rawValue || '{"rules":[]}') as {
      rules?: UserModelViewRule[]
    }
    if (!Array.isArray(parsed.rules)) return []
    return parsed.rules.map((rule) => ({
      user_id: Number(rule.user_id),
      username: String(rule.username ?? ''),
      user_group: String(rule.user_group ?? ''),
      disabled: Boolean(rule.disabled),
      aliases: Array.isArray(rule.aliases)
        ? rule.aliases.map((alias) =>
            createEditableAlias({
              public_model: String(alias.public_model ?? ''),
              target_model: String(alias.target_model ?? ''),
              reference_video:
                alias.reference_video === 'forbidden' ? 'forbidden' : 'allowed',
            })
          )
        : [],
    }))
  } catch {
    return []
  }
}

function serializeUserModelView(rules: UserModelViewRule[]) {
  return JSON.stringify({
    rules: rules.map((rule) => ({
      user_id: rule.user_id,
      username: rule.username,
      user_group: rule.user_group,
      disabled: rule.disabled,
      aliases: rule.aliases.map((alias) => ({
        public_model: alias.public_model,
        target_model: alias.target_model,
        reference_video: alias.reference_video,
      })),
    })),
  })
}

function createDraftRule(): UserModelViewRule {
  return {
    user_id: 0,
    username: '',
    user_group: '',
    disabled: false,
    aliases: cloneSeedanceTemplate(),
  }
}

export function UserModelViewSection(props: { defaultValue: string }) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [rules, setRules] = useState(() =>
    parseUserModelView(props.defaultValue)
  )
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingUserId, setEditingUserId] = useState(0)
  const [selectedUsers, setSelectedUsers] = useState<UserChoice[]>([])
  const [draft, setDraft] = useState<UserModelViewRule>(createDraftRule)
  const [deleteTarget, setDeleteTarget] = useState<UserModelViewRule | null>(
    null
  )

  useEffect(
    () => setRules(parseUserModelView(props.defaultValue)),
    [props.defaultValue]
  )

  const openCreateDialog = () => {
    setEditingUserId(0)
    setSelectedUsers([])
    setDraft(createDraftRule())
    setDialogOpen(true)
  }

  const openEditDialog = (rule: UserModelViewRule) => {
    setEditingUserId(rule.user_id)
    setSelectedUsers([
      {
        id: rule.user_id,
        username: rule.username,
        group: rule.user_group,
      },
    ])
    setDraft({
      ...rule,
      aliases: rule.aliases.map((alias) => ({ ...alias })),
    })
    setDialogOpen(true)
  }

  const updateAlias = (index: number, patch: Partial<UserModelAlias>) => {
    setDraft((current) => ({
      ...current,
      aliases: current.aliases.map((alias, aliasIndex) =>
        aliasIndex === index ? { ...alias, ...patch } : alias
      ),
    }))
  }

  const saveRule = () => {
    const user = selectedUsers[0]
    if (!user) {
      toast.error(t('Select one user'))
      return
    }
    if (
      rules.some(
        (rule) => rule.user_id === user.id && rule.user_id !== editingUserId
      )
    ) {
      toast.error(t('This user already has a model view'))
      return
    }
    if (draft.aliases.length === 0) {
      toast.error(t('Add at least one model alias'))
      return
    }

    const aliases = draft.aliases.map((alias) => ({
      ...alias,
      public_model: alias.public_model.trim(),
      target_model: alias.target_model.trim(),
    }))
    if (aliases.some((alias) => !alias.public_model || !alias.target_model)) {
      toast.error(t('Public and target model names are required'))
      return
    }
    if (aliases.some((alias) => alias.public_model === alias.target_model)) {
      toast.error(t('Public and target model names must differ'))
      return
    }
    if (
      new Set(aliases.map((alias) => alias.public_model)).size !==
      aliases.length
    ) {
      toast.error(t('Public model names must be unique'))
      return
    }

    const nextRule: UserModelViewRule = {
      user_id: user.id,
      username: user.username,
      user_group: user.group,
      disabled: draft.disabled,
      aliases,
    }
    const nextRules = [
      ...rules.filter((rule) => rule.user_id !== editingUserId),
      nextRule,
    ].sort((left, right) => left.user_id - right.user_id)

    updateOption.mutate(
      {
        key: 'UserModelView',
        value: serializeUserModelView(nextRules),
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
    const nextRules = rules.filter(
      (rule) => rule.user_id !== deleteTarget.user_id
    )
    updateOption.mutate(
      {
        key: 'UserModelView',
        value: serializeUserModelView(nextRules),
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
      title={t('User Model Views')}
      description={t(
        'Replace selected video models with user-specific aliases without changing channels or groups. Routing and pricing continue to use the target models.'
      )}
      icon={<HugeiconsIcon icon={UserSettings01Icon} strokeWidth={2} />}
      action={
        <Button type='button' onClick={openCreateDialog}>
          <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
          {t('Add user model view')}
        </Button>
      }
    >
      {rules.length === 0 ? (
        <Empty className='border'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <HugeiconsIcon icon={AiVideoIcon} strokeWidth={2} />
            </EmptyMedia>
            <EmptyTitle>{t('No user model views configured')}</EmptyTitle>
            <EmptyDescription>
              {t(
                'Add a user to replace visible model names only for that account.'
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
                <TableHead>{t('Status')}</TableHead>
                <TableHead>{t('Model aliases')}</TableHead>
                <TableHead className='text-right'>{t('Actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.map((rule) => (
                <TableRow key={rule.user_id}>
                  <TableCell>
                    <div className='font-medium'>{rule.username || '-'}</div>
                    <div className='text-muted-foreground text-xs'>
                      ID {rule.user_id} / {rule.user_group || '-'}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={rule.disabled ? 'destructive' : 'secondary'}
                    >
                      {rule.disabled ? t('Disabled') : t('Enabled')}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    {t('{{count}} aliases', { count: rule.aliases.length })}
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
        title={
          editingUserId ? t('Edit user model view') : t('Add user model view')
        }
        description={t(
          "Aliases appear in this user's model plaza and model lists. Target models remain callable but are hidden from this user."
        )}
        contentClassName='sm:max-w-5xl'
        contentHeight='min(70vh, 44rem)'
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
              {t('This configuration is matched by exact user ID.')}
            </FieldDescription>
          </Field>

          <Field orientation='horizontal'>
            <FieldContent>
              <FieldTitle>{t('Enabled')}</FieldTitle>
              <FieldDescription>
                {t(
                  'Disable the view temporarily without deleting its aliases.'
                )}
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

          <FieldSet>
            <FieldLegend variant='label'>{t('Model aliases')}</FieldLegend>
            <div className='flex flex-wrap justify-end gap-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() =>
                  setDraft((current) => ({
                    ...current,
                    aliases: cloneSeedanceTemplate(),
                  }))
                }
              >
                <HugeiconsIcon icon={Copy01Icon} data-icon='inline-start' />
                {t('Load 521AI Seedance template')}
              </Button>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() =>
                  setDraft((current) => ({
                    ...current,
                    aliases: [
                      ...current.aliases,
                      createEditableAlias({
                        public_model: '',
                        target_model: '',
                        reference_video: 'forbidden',
                      }),
                    ],
                  }))
                }
              >
                <HugeiconsIcon icon={Add01Icon} data-icon='inline-start' />
                {t('Add model alias')}
              </Button>
            </div>

            <div className='overflow-x-auto rounded-lg border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className='min-w-56'>
                      {t('Public model')}
                    </TableHead>
                    <TableHead className='min-w-56'>
                      {t('Target model')}
                    </TableHead>
                    <TableHead className='min-w-44'>
                      {t('Reference video')}
                    </TableHead>
                    <TableHead className='w-14 text-right'>
                      {t('Actions')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {draft.aliases.map((alias, index) => (
                    <TableRow key={alias.editor_key}>
                      <TableCell>
                        <Input
                          value={alias.public_model}
                          aria-label={t('Public model')}
                          placeholder='521ai-2.0-720p'
                          onChange={(event) =>
                            updateAlias(index, {
                              public_model: event.target.value,
                            })
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <Input
                          value={alias.target_model}
                          aria-label={t('Target model')}
                          placeholder='seedance-2.0-720p'
                          onChange={(event) =>
                            updateAlias(index, {
                              target_model: event.target.value,
                            })
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <NativeSelect
                          className='w-full'
                          value={alias.reference_video}
                          aria-label={t('Reference video')}
                          onChange={(event) =>
                            updateAlias(index, {
                              reference_video: event.target
                                .value as ReferenceVideoPolicy,
                            })
                          }
                        >
                          <NativeSelectOption value='allowed'>
                            {t('Allowed')}
                          </NativeSelectOption>
                          <NativeSelectOption value='forbidden'>
                            {t('Forbidden')}
                          </NativeSelectOption>
                        </NativeSelect>
                      </TableCell>
                      <TableCell className='text-right'>
                        <Button
                          type='button'
                          variant='ghost'
                          size='icon-sm'
                          aria-label={t('Delete')}
                          title={t('Delete')}
                          onClick={() =>
                            setDraft((current) => ({
                              ...current,
                              aliases: current.aliases.filter(
                                (_, aliasIndex) => aliasIndex !== index
                              ),
                            }))
                          }
                        >
                          <HugeiconsIcon icon={Delete02Icon} />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </FieldSet>
        </FieldGroup>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
        title={t('Delete user model view')}
        desc={t('This will remove all model aliases for user {{user}}.', {
          user: deleteTarget
            ? `${deleteTarget.user_id} / ${deleteTarget.username || '-'}`
            : '-',
        })}
        confirmText={t('Delete')}
        destructive
        isLoading={updateOption.isPending}
        handleConfirm={deleteRule}
      />
    </TitledCard>
  )
}
