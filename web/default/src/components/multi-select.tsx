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
import * as React from 'react'
import { Command as CommandPrimitive } from 'cmdk'
import { Check, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Command, CommandGroup, CommandItem } from '@/components/ui/command'

export type Option = {
  label: string
  value: string
}

interface MultiSelectProps {
  options: Option[]
  selected: string[]
  onChange: (values: string[]) => void
  placeholder?: string
  className?: string
  dropdownClassName?: string
  maxVisible?: number
  keepSelectedOptionsVisible?: boolean
  createOption?: (inputValue: string) => Option | null
  formatSelectedLabel?: (value: string, option?: Option) => string
}

export function MultiSelect({
  options,
  selected,
  onChange,
  placeholder,
  className,
  dropdownClassName,
  maxVisible,
  keepSelectedOptionsVisible,
  createOption,
  formatSelectedLabel,
}: MultiSelectProps) {
  const { t } = useTranslation()
  const resolvedPlaceholder = placeholder ?? t('Select items...')
  const inputRef = React.useRef<HTMLInputElement>(null)
  const [open, setOpen] = React.useState(false)
  const [inputValue, setInputValue] = React.useState('')

  const handleUnselect = (value: string) => {
    onChange(selected.filter((s) => s !== value))
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    const input = inputRef.current
    if (input) {
      if (e.key === 'Delete' || e.key === 'Backspace') {
        if (input.value === '' && selected.length > 0) {
          onChange(selected.slice(0, -1))
        }
      }
      if (e.key === 'Escape') {
        input.blur()
      }
    }
  }

  const selectedOptions = React.useMemo(() => {
    if (!keepSelectedOptionsVisible) return []
    const existingValues = new Set(options.map((option) => option.value))
    return selected
      .filter((value) => !existingValues.has(value))
      .map((value) => ({
        value,
        label: formatSelectedLabel ? formatSelectedLabel(value) : value,
      }))
  }, [formatSelectedLabel, keepSelectedOptionsVisible, options, selected])
  const selectableOptions = keepSelectedOptionsVisible
    ? [...options, ...selectedOptions]
    : options.filter((option) => !selected.includes(option.value))
  const trimmedInputValue = inputValue.trim()
  const createdOption =
    trimmedInputValue.length > 0 && createOption
      ? createOption(trimmedInputValue)
      : null
  const showCreatedOption =
    createdOption != null &&
    !selected.includes(createdOption.value) &&
    !options.some(
      (option) =>
        option.value === createdOption.value ||
        option.label.toLowerCase() === createdOption.label.toLowerCase()
    )
  const visibleSelected =
    maxVisible && maxVisible > 0 ? selected.slice(0, maxVisible) : selected
  const hiddenSelectedCount = selected.length - visibleSelected.length

  return (
    <Command
      onKeyDown={handleKeyDown}
      className={`overflow-visible bg-transparent ${className || ''}`}
    >
      <div className='group border-input ring-offset-background focus-within:ring-ring flex h-9 items-center overflow-hidden rounded-md border px-2 text-sm focus-within:ring-2 focus-within:ring-offset-2'>
        <div className='flex min-w-0 flex-1 items-center gap-1 overflow-hidden'>
          {visibleSelected.map((value) => {
            const option = options.find((o) => o.value === value)
            const label = formatSelectedLabel
              ? formatSelectedLabel(value, option)
              : option?.label || value
            return (
              <Badge
                key={value}
                variant='secondary'
                className='max-w-[8rem] px-1.5'
              >
                <span className='truncate'>{label}</span>
                <Button
                  variant='ghost'
                  size='icon-sm'
                  aria-label='Remove'
                  className='ml-1 size-auto p-0'
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      handleUnselect(value)
                    }
                  }}
                  onMouseDown={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                  }}
                  onClick={() => handleUnselect(value)}
                >
                  <X
                    className='text-muted-foreground hover:text-foreground h-3 w-3'
                    aria-hidden='true'
                  />
                </Button>
              </Badge>
            )
          })}
          {hiddenSelectedCount > 0 && (
            <Badge variant='outline'>+{hiddenSelectedCount}</Badge>
          )}
          <CommandPrimitive.Input
            ref={inputRef}
            value={inputValue}
            onValueChange={setInputValue}
            onBlur={() => setOpen(false)}
            onFocus={() => setOpen(true)}
            placeholder={selected.length === 0 ? resolvedPlaceholder : ''}
            className='placeholder:text-muted-foreground min-w-[3rem] flex-1 bg-transparent outline-none'
          />
        </div>
      </div>
      <div className='relative'>
        {open && (selectableOptions.length > 0 || showCreatedOption) ? (
          <div
            className={cn(
              'bg-popover text-popover-foreground animate-in absolute top-0 z-50 w-full rounded-md border shadow-md outline-none',
              dropdownClassName
            )}
          >
            <CommandGroup className='h-full max-h-60 overflow-auto'>
              {showCreatedOption && (
                <CommandItem
                  key={createdOption.value}
                  value={createdOption.label}
                  onMouseDown={(e) => {
                    e.preventDefault()
                    e.stopPropagation()
                  }}
                  onSelect={() => {
                    setInputValue('')
                    onChange([...selected, createdOption.value])
                  }}
                  className='cursor-pointer'
                >
                  {createdOption.label}
                </CommandItem>
              )}
              {selectableOptions.map((option) => {
                const isSelected = selected.includes(option.value)
                return (
                  <CommandItem
                    key={option.value}
                    value={option.label}
                    onMouseDown={(e) => {
                      e.preventDefault()
                      e.stopPropagation()
                    }}
                    onSelect={() => {
                      setInputValue('')
                      onChange(
                        isSelected
                          ? selected.filter((value) => value !== option.value)
                          : [...selected, option.value]
                      )
                    }}
                    className='cursor-pointer'
                  >
                    {keepSelectedOptionsVisible && (
                      <Check
                        className={cn(
                          'mr-2 h-4 w-4',
                          isSelected ? 'opacity-100' : 'opacity-0'
                        )}
                      />
                    )}
                    {option.label}
                  </CommandItem>
                )
              })}
            </CommandGroup>
          </div>
        ) : null}
      </div>
    </Command>
  )
}
