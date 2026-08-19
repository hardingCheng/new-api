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
import type { Row } from '@tanstack/react-table'
import {
  Trash2,
  Edit,
  Power,
  PowerOff,
  ArrowRightLeft,
  KeyRound,
  Loader2,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'

import { updateApiKeyStatus } from '../api'
import { API_KEY_STATUS, ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import { apiKeySchema } from '../types'
import { useApiKeys } from './api-keys-provider'

type DataTableRowActionsProps<TData> = {
  row: Row<TData>
}

export function DataTableRowActions<TData>({
  row,
}: DataTableRowActionsProps<TData>) {
  const { t } = useTranslation()
  const apiKey = apiKeySchema.parse(row.original)
  const {
    setOpen,
    setCurrentRow,
    triggerRefresh,
    setResolvedKey,
    resolveRealKey,
  } = useApiKeys()
  const isEnabled = apiKey.status === API_KEY_STATUS.ENABLED
  const [isTogglingStatus, setIsTogglingStatus] = useState(false)

  const toggleLabel = isEnabled ? t('Disable') : t('Enable')

  const handleToggleStatus = async (
    e?: React.MouseEvent<HTMLButtonElement>
  ) => {
    e?.stopPropagation()
    const newStatus = isEnabled
      ? API_KEY_STATUS.DISABLED
      : API_KEY_STATUS.ENABLED

    setIsTogglingStatus(true)
    try {
      const result = await updateApiKeyStatus(apiKey.id, newStatus)
      if (result.success) {
        const message = isEnabled
          ? t(SUCCESS_MESSAGES.API_KEY_DISABLED)
          : t(SUCCESS_MESSAGES.API_KEY_ENABLED)
        toast.success(message)
        triggerRefresh()
      } else {
        toast.error(result.message || t(ERROR_MESSAGES.STATUS_UPDATE_FAILED))
      }
    } catch {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsTogglingStatus(false)
    }
  }

  let statusIcon = <Power className='size-3.5' />
  if (isTogglingStatus) {
    statusIcon = <Loader2 className='size-3.5 animate-spin' />
  } else if (isEnabled) {
    statusIcon = <PowerOff className='size-3.5' />
  }

  return (
    <div className='flex items-center gap-1 whitespace-nowrap'>
      <Button
        variant='ghost'
        size='sm'
        className='h-7 px-2 text-xs'
        onClick={async () => {
          const realKey = await resolveRealKey(apiKey.id)
          if (!realKey) return
          setResolvedKey(realKey)
          setCurrentRow(apiKey)
          setOpen('use-key')
        }}
      >
        <KeyRound className='size-3.5' data-icon='inline-start' />
        {t('Use API Key')}
      </Button>
      <Button
        variant='ghost'
        size='sm'
        className='h-7 px-2 text-xs'
        onClick={async () => {
          const realKey = await resolveRealKey(apiKey.id)
          if (!realKey) return
          setResolvedKey(realKey)
          setCurrentRow(apiKey)
          setOpen('cc-switch')
        }}
      >
        <ArrowRightLeft className='size-3.5' data-icon='inline-start' />
        CCS
      </Button>
      <Button
        variant='ghost'
        size='sm'
        className='h-7 px-2 text-xs'
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('update')
        }}
      >
        <Edit className='size-3.5' data-icon='inline-start' />
        {t('Edit')}
      </Button>
      <Button
        variant='ghost'
        size='sm'
        className={
          isEnabled
            ? 'text-destructive hover:text-destructive h-7 px-2 text-xs'
            : 'h-7 px-2 text-xs text-emerald-600 hover:text-emerald-600 dark:text-emerald-400 dark:hover:text-emerald-400'
        }
        onClick={handleToggleStatus}
        disabled={isTogglingStatus}
      >
        {statusIcon}
        {toggleLabel}
      </Button>
      <Button
        variant='ghost'
        size='sm'
        className='text-destructive hover:text-destructive h-7 px-2 text-xs'
        onClick={() => {
          setCurrentRow(apiKey)
          setOpen('delete')
        }}
      >
        <Trash2 className='size-3.5' data-icon='inline-start' />
        {t('Delete')}
      </Button>
    </div>
  )
}
