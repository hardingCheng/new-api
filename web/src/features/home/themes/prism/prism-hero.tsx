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
import { Link } from '@tanstack/react-router'
import { ArrowRight, BookOpen } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PrismDial } from './prism-dial'

import { Button } from '@/components/ui/button'

type PrismHeroProps = {
  isAuthenticated: boolean
  docsUrl: string
}

export function PrismHero(props: PrismHeroProps) {
  const { t } = useTranslation()
  const primaryHref = props.isAuthenticated ? '/dashboard' : '/sign-up'
  const primaryLabel = props.isAuthenticated
    ? t('Go to Dashboard')
    : t('Get API Key')

  return (
    <section className='relative overflow-hidden px-4 pt-24 pb-16 sm:px-6 sm:pt-28 sm:pb-20 lg:pt-32 lg:pb-24'>
      {/* 方案 C:底部升起巨环(纯装饰背景层) */}
      <div className='prism-rise-layer' aria-hidden='true'>
        <div className='prism-rise' />
        <div className='prism-rise-glow' />
      </div>
      <div className='relative z-10 mx-auto grid max-w-7xl grid-cols-1 items-center gap-10 lg:min-h-[680px] lg:grid-cols-[0.95fr_1.05fr] lg:gap-12'>
        <div className='min-w-0'>
          <h1 className='max-w-2xl text-[clamp(2.85rem,5vw,4.5rem)] leading-[1.01] font-semibold tracking-[-0.065em]'>
            {t('One endpoint,')}
            <br />
            <span className='text-[var(--prism-accent)]'>
              {t('more models.')}
            </span>
          </h1>
          <p className='text-muted-foreground mt-6 max-w-xl text-base leading-7 sm:text-lg'>
            {t(
              'Keep the SDK you know. Change the Base URL to connect text, image, and video models.'
            )}
          </p>
          <div className='mt-8 flex flex-wrap gap-3'>
            <Button
              size='xl'
              className='group bg-[var(--prism-accent)] whitespace-nowrap text-[var(--prism-accent-ink)] [a]:hover:bg-[color-mix(in_srgb,var(--prism-accent)_88%,white)]'
              render={<Link to={primaryHref} />}
            >
              {primaryLabel}
              <ArrowRight
                aria-hidden='true'
                className='size-4 transition-transform group-hover:translate-x-0.5'
              />
            </Button>
            <Button
              size='xl'
              variant='outline'
              className='group border-[var(--prism-line-strong)] bg-transparent whitespace-nowrap hover:bg-white/5'
              render={
                props.docsUrl.startsWith('http') ? (
                  <a
                    href={props.docsUrl}
                    target='_blank'
                    rel='noopener noreferrer'
                  />
                ) : (
                  <Link to={props.docsUrl} />
                )
              }
            >
              <BookOpen aria-hidden='true' className='size-4' />
              {t('View integration guide')}
            </Button>
          </div>
        </div>

        <PrismDial />
      </div>
    </section>
  )
}
