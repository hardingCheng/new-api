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
import { Search, X } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { searchUsers } from '@/features/users/api'
import type { User } from '@/features/users/types'

export type UserChoice = Pick<User, 'id' | 'username' | 'group'>

export function UserSearchPicker(props: {
  value: UserChoice[]
  onChange: (users: UserChoice[]) => void
  multiple?: boolean
}) {
  const { t } = useTranslation()
  const [keyword, setKeyword] = useState('')
  const [results, setResults] = useState<UserChoice[]>([])
  const [loading, setLoading] = useState(false)

  const search = async () => {
    setLoading(true)
    try {
      const response = await searchUsers({
        keyword,
        p: 1,
        page_size: 20,
      })
      if (!response.success) {
        toast.error(response.message || t('Failed to search users'))
        return
      }
      setResults(
        (response.data?.items ?? []).map((user) => ({
          id: user.id,
          username: user.username,
          group: user.group,
        }))
      )
    } catch {
      toast.error(t('Failed to search users'))
    } finally {
      setLoading(false)
    }
  }

  const toggleUser = (user: UserChoice) => {
    if (!props.multiple) {
      props.onChange([user])
      return
    }
    const exists = props.value.some((item) => item.id === user.id)
    props.onChange(
      exists
        ? props.value.filter((item) => item.id !== user.id)
        : [...props.value, user]
    )
  }

  return (
    <div className='grid gap-2'>
      <div className='flex gap-2'>
        <Input
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              void search()
            }
          }}
          placeholder={t('Search users')}
        />
        <Button
          type='button'
          variant='outline'
          size='icon'
          aria-label={t('Search')}
          title={t('Search')}
          disabled={loading}
          onClick={() => void search()}
        >
          <Search />
        </Button>
      </div>
      {props.value.length > 0 ? (
        <div className='flex flex-wrap gap-1'>
          {props.value.map((user) => (
            <Badge key={user.id} variant='secondary' className='gap-1'>
              {user.id} / {user.username} / {user.group}
              <button
                type='button'
                aria-label={t('Remove')}
                onClick={() =>
                  props.onChange(
                    props.value.filter((item) => item.id !== user.id)
                  )
                }
              >
                <X className='size-3' />
              </button>
            </Badge>
          ))}
        </div>
      ) : null}
      {results.length > 0 ? (
        <div className='max-h-40 overflow-y-auto rounded-lg border p-1'>
          {results.map((user) => {
            const selected = props.value.some((item) => item.id === user.id)
            return (
              <button
                key={user.id}
                type='button'
                className='hover:bg-muted flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-sm'
                onClick={() => toggleUser(user)}
              >
                <span>
                  {user.id} / {user.username} / {user.group}
                </span>
                {selected ? <Badge>{t('selected')}</Badge> : null}
              </button>
            )
          })}
        </div>
      ) : null}
    </div>
  )
}
