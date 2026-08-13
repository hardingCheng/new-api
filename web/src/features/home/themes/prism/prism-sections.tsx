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
import { Anthropic, DeepSeek, Gemini, OpenAI } from '@lobehub/icons'
import { Link } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

const PROVIDERS = [OpenAI, Anthropic, Gemini, DeepSeek]

export function PrismProviderBand() {
  const { t } = useTranslation()

  return (
    <section
      aria-label={t('Supported model providers')}
      className='border-y border-white/10 px-4 sm:px-6'
    >
      <div className='mx-auto grid max-w-7xl grid-cols-4 items-center gap-6 py-8 sm:py-10'>
        {PROVIDERS.map((Provider) => (
          <Provider
            key={Provider.title}
            aria-label={Provider.title}
            className='prism-provider-logo mx-auto size-7 sm:size-9'
          />
        ))}
      </div>
    </section>
  )
}

export function PrismCapabilities() {
  const { t } = useTranslation()
  const capabilities = [
    {
      title: t('One account, more models'),
      body: t('Access text, image, and video models from one place.'),
    },
    {
      title: t('Clear usage records'),
      body: t('Review request usage and cost details in the same dashboard.'),
    },
    {
      title: t('Project-level keys'),
      body: t('Create separate credentials and limits for every application.'),
    },
    {
      title: t('Compatible integrations'),
      body: t(
        'Keep familiar SDKs while changing models as your project evolves.'
      ),
    },
  ]

  return (
    <section
      id='capabilities'
      className='border-y border-white/10 bg-[#111411]'
    >
      <div className='mx-auto grid max-w-7xl grid-cols-1 lg:grid-cols-[0.82fr_1.18fr]'>
        <div className='flex items-center px-4 py-16 sm:px-6 sm:py-24 lg:px-10'>
          <h2 className='max-w-lg text-4xl leading-tight font-semibold tracking-[-0.045em] sm:text-5xl'>
            {t('Simple to connect, clear to operate.')}
          </h2>
        </div>
        <div className='grid grid-cols-1 border-t border-white/10 sm:grid-cols-2 lg:border-t-0 lg:border-l'>
          {capabilities.map((item, index) => (
            <article
              key={item.title}
              className={`min-h-52 p-7 sm:p-8 ${
                index % 2 === 1 ? 'sm:border-l sm:border-white/10' : ''
              } ${index >= 2 ? 'border-t border-white/10' : ''}`}
            >
              <h3 className='text-lg font-semibold'>{item.title}</h3>
              <p className='text-muted-foreground mt-3 max-w-sm text-sm leading-6'>
                {item.body}
              </p>
            </article>
          ))}
        </div>
      </div>
    </section>
  )
}

export function PrismRouteDisclosure() {
  const { t } = useTranslation()

  return (
    <section className='px-4 py-20 sm:px-6 sm:py-28'>
      <div className='mx-auto max-w-7xl'>
        <div className='max-w-3xl'>
          <h2 className='text-4xl leading-tight font-semibold tracking-[-0.045em] sm:text-5xl'>
            {t('Choose the route that fits the workload.')}
          </h2>
          <p className='text-muted-foreground mt-5 max-w-2xl text-base leading-7'>
            {t(
              'Available routes include official direct access and compatible routes. Check the model list for the route type.'
            )}
          </p>
        </div>
        <div className='mt-12 grid grid-cols-1 gap-3 lg:grid-cols-[1.08fr_0.92fr]'>
          <div className='prism-route-map prism-reveal rounded-xl border border-white/10 bg-[#111411] p-6 sm:p-8'>
            <div className='prism-route-node'>
              <span>{t('Your application')}</span>
              <small>{t('Existing SDK')}</small>
            </div>
            <ArrowRight aria-hidden='true' className='size-4 text-white/35' />
            <div className='prism-route-node prism-route-node--accent'>
              <span>{t('Unified endpoint')}</span>
              <small>{t('One project key')}</small>
            </div>
            <ArrowRight aria-hidden='true' className='size-4 text-white/35' />
            <div className='grid gap-2'>
              <div className='prism-route-node prism-route-node--compact'>
                <span>{t('Official direct')}</span>
              </div>
              <div className='prism-route-node prism-route-node--compact'>
                <span>{t('Compatible route')}</span>
              </div>
            </div>
          </div>
          <div className='overflow-hidden rounded-xl border border-white/10 bg-[#111411]'>
            <article className='prism-reveal p-7 sm:p-8'>
              <h3 className='text-xl font-semibold'>{t('Official direct')}</h3>
              <p className='text-muted-foreground mt-3 text-sm leading-6'>
                {t(
                  'Routes backed by official API credentials or official cloud services.'
                )}
              </p>
            </article>
            <article className='prism-reveal border-t border-white/10 p-7 sm:p-8'>
              <h3 className='text-xl font-semibold'>{t('Compatible route')}</h3>
              <p className='text-muted-foreground mt-3 text-sm leading-6'>
                {t(
                  'Compatible protocol routes with their source clearly identified.'
                )}
              </p>
            </article>
          </div>
        </div>
        <Button
          variant='outline'
          className='mt-6 h-7 gap-1 border-[var(--prism-line-strong)] bg-transparent text-[0.8rem] hover:bg-white/5 sm:h-8 sm:gap-1.5 sm:text-sm'
          render={<Link to='/pricing' />}
        >
          {t('View models and route details')}
          <ArrowRight aria-hidden='true' className='size-4' />
        </Button>
      </div>
    </section>
  )
}

export function PrismFinalCTA(props: { isAuthenticated: boolean }) {
  const { t } = useTranslation()
  const href = props.isAuthenticated ? '/dashboard' : '/sign-up'
  const label = props.isAuthenticated ? t('Go to Dashboard') : t('Get Started')

  return (
    <section className='px-4 pb-24 sm:px-6 sm:pb-32'>
      <div className='mx-auto grid max-w-7xl grid-cols-1 items-end gap-8 border-t border-white/10 pt-16 lg:grid-cols-[1.25fr_0.75fr]'>
        <h2 className='max-w-4xl text-4xl leading-tight font-semibold tracking-[-0.05em] sm:text-6xl'>
          {t('Send your next model request from here.')}
        </h2>
        <div className='lg:justify-self-end'>
          <p className='text-muted-foreground mb-6 max-w-sm text-sm leading-6'>
            {t('Create an API key and send your first request in minutes.')}
          </p>
          <Button
            size='xl'
            className='group bg-[var(--prism-accent)] text-[var(--prism-accent-ink)] [a]:hover:bg-[color-mix(in_srgb,var(--prism-accent)_88%,white)]'
            render={<Link to={href} />}
          >
            {label}
            <ArrowRight
              aria-hidden='true'
              className='size-4 transition-transform group-hover:translate-x-0.5'
            />
          </Button>
        </div>
      </div>
    </section>
  )
}
