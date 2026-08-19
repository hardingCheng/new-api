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
  AlibabaCloud,
  Anthropic,
  Aws,
  Azure,
  Cloudflare,
  GoogleCloud,
  HuggingFace,
  Meta,
  Mistral,
  Nvidia,
  OpenAI,
  Snowflake,
  TencentCloud,
  Vercel,
} from '@lobehub/icons'
import { Link } from '@tanstack/react-router'
import {
  ArrowRight,
  BookOpen,
  Code2,
  Image,
  MessageSquare,
  Mic2,
  Terminal,
  Video,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

const MODEL_FAMILIES = [
  {
    title: 'Text and reasoning',
    body: 'Move between general chat, deep reasoning, and coding workloads without rebuilding the application.',
    icon: MessageSquare,
    className: 'md:col-span-7 md:row-span-2',
    labels: ['OpenAI', 'Claude', 'Gemini', 'DeepSeek'],
    tone: 'prism-model-panel--primary',
  },
  {
    title: 'Image generation',
    body: 'Connect image generation and editing models through the same account.',
    icon: Image,
    className: 'md:col-span-5',
    labels: ['Generate', 'Edit'],
    tone: 'prism-model-panel--image',
  },
  {
    title: 'Video generation',
    body: 'Explore video models and compare available routes before integrating.',
    icon: Video,
    className: 'md:col-span-5',
    labels: ['Text to video', 'Image to video'],
    tone: 'prism-model-panel--video',
  },
] as const

export function PrismModelShowcase() {
  const { t } = useTranslation()

  return (
    <section className='px-4 py-20 sm:px-6 sm:py-28'>
      <div className='mx-auto max-w-7xl'>
        <div className='prism-reveal max-w-3xl'>
          <h2 className='text-4xl leading-tight font-semibold tracking-[-0.045em] sm:text-5xl'>
            {t('Pick the right model for every job.')}
          </h2>
          <p className='text-muted-foreground mt-5 max-w-2xl text-base leading-7'>
            {t(
              'Browse model families by workload, then compare the available route and price before you call.'
            )}
          </p>
        </div>

        <div className='mt-12 grid grid-cols-1 gap-3 md:grid-cols-12 md:grid-rows-2'>
          {MODEL_FAMILIES.map((family) => {
            const Icon = family.icon
            return (
              <article
                key={family.title}
                className={`prism-model-panel prism-reveal ${family.tone} ${family.className}`}
              >
                <div className='flex items-start justify-between gap-6'>
                  <Icon
                    aria-hidden='true'
                    className='size-5'
                    strokeWidth={1.6}
                  />
                  <ArrowRight
                    aria-hidden='true'
                    className='size-4 text-white/40'
                  />
                </div>
                <div className='mt-auto pt-14'>
                  <h3 className='text-2xl font-semibold tracking-[-0.03em]'>
                    {t(family.title)}
                  </h3>
                  <p className='text-muted-foreground mt-3 max-w-xl text-sm leading-6'>
                    {t(family.body)}
                  </p>
                  <div className='mt-6 flex flex-wrap gap-2'>
                    {family.labels.map((label) => (
                      <span
                        key={label}
                        className='border border-white/10 bg-black/15 px-3 py-1.5 font-mono text-[11px] text-white/68'
                      >
                        {t(label)}
                      </span>
                    ))}
                  </div>
                </div>
              </article>
            )
          })}

          <article className='prism-model-panel prism-model-panel--audio prism-reveal md:col-span-12'>
            <div className='flex flex-col justify-between gap-10 md:flex-row md:items-end'>
              <div>
                <Mic2 aria-hidden='true' className='size-5' strokeWidth={1.6} />
                <h3 className='mt-10 text-2xl font-semibold tracking-[-0.03em]'>
                  {t('Speech and realtime')}
                </h3>
                <p className='text-muted-foreground mt-3 max-w-xl text-sm leading-6'>
                  {t(
                    'Use speech, transcription, and realtime model routes when they are available on the station.'
                  )}
                </p>
              </div>
              <Button
                variant='outline'
                className='h-7 w-fit gap-1 border-[var(--prism-line-strong)] bg-transparent text-[0.8rem] hover:bg-white/5 sm:h-8 sm:gap-1.5 sm:text-sm'
                render={<Link to='/pricing' />}
              >
                {t('Browse models and pricing')}
                <ArrowRight aria-hidden='true' className='size-4' />
              </Button>
            </div>
          </article>
        </div>
      </div>
    </section>
  )
}

const ONBOARDING_STEPS = [
  {
    title: 'Create an account',
    body: 'Open the console and keep balance, projects, and usage in one place.',
  },
  {
    title: 'Create a project key',
    body: 'Choose the models and limits that this application can use.',
  },
  {
    title: 'Replace the Base URL',
    body: 'Keep the SDK and request format already used by your application.',
  },
  {
    title: 'Send the first request',
    body: 'Review the request record, route, and cost from the console.',
  },
] as const

export function PrismOnboarding() {
  const { t } = useTranslation()

  return (
    <section className='border-y border-white/10 bg-[#0e1110] px-4 py-20 sm:px-6 sm:py-28'>
      <div className='mx-auto max-w-7xl'>
        <div className='prism-reveal max-w-3xl'>
          <h2 className='text-4xl leading-tight font-semibold tracking-[-0.045em] sm:text-5xl'>
            {t('From account to first request.')}
          </h2>
          <p className='text-muted-foreground mt-5 max-w-2xl text-base leading-7'>
            {t(
              'The integration path stays short, even when the model behind the endpoint changes.'
            )}
          </p>
        </div>

        <ol className='prism-step-line mt-14 grid grid-cols-1 gap-8 md:grid-cols-4 md:gap-0'>
          {ONBOARDING_STEPS.map((step, index) => (
            <li key={step.title} className='prism-step prism-reveal'>
              <span className='prism-step-marker font-mono text-xs'>
                {String(index + 1).padStart(2, '0')}
              </span>
              <h3 className='mt-7 text-lg font-semibold'>{t(step.title)}</h3>
              <p className='text-muted-foreground mt-3 max-w-xs text-sm leading-6'>
                {t(step.body)}
              </p>
            </li>
          ))}
        </ol>
      </div>
    </section>
  )
}

const CLIENTS = [
  { name: 'OpenAI SDK', icon: Code2, slug: 'quick-start' },
  { name: 'Claude Code', icon: Terminal, slug: 'claude-code' },
  { name: 'Codex', icon: Terminal, slug: 'codex' },
  { name: 'Cherry Studio', icon: MessageSquare, slug: 'quick-start' },
] as const

export function PrismEcosystem(props: { docsUrl: string }) {
  const { t } = useTranslation()

  return (
    <section className='px-4 py-20 sm:px-6 sm:py-28'>
      <div className='mx-auto grid max-w-7xl grid-cols-1 gap-12 lg:grid-cols-[0.8fr_1.2fr] lg:gap-20'>
        <div className='prism-reveal lg:sticky lg:top-28 lg:self-start'>
          <h2 className='text-4xl leading-tight font-semibold tracking-[-0.045em] sm:text-5xl'>
            {t('Use the tools already in your workflow.')}
          </h2>
          <p className='text-muted-foreground mt-5 max-w-xl text-base leading-7'>
            {t(
              'Connect SDKs, coding agents, and desktop clients with one station address and one project key.'
            )}
          </p>
          <Button
            variant='outline'
            className='mt-7 h-7 gap-1 border-[var(--prism-line-strong)] bg-transparent text-[0.8rem] hover:bg-white/5 sm:h-8 sm:gap-1.5 sm:text-sm'
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

        <div className='border-t border-white/10'>
          {CLIENTS.map((client) => {
            const Icon = client.icon
            return (
              <Link
                key={client.name}
                to='/tutorials/$slug'
                params={{ slug: client.slug }}
                className='prism-client-row prism-reveal grid grid-cols-[auto_1fr_auto] items-center gap-5 border-b border-white/10 py-6 transition-colors hover:bg-white/[0.03] sm:py-7'
              >
                <Icon
                  aria-hidden='true'
                  className='size-5 text-white/60'
                  strokeWidth={1.6}
                />
                <div>
                  <h3 className='font-semibold'>{client.name}</h3>
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {t('Works with compatible station endpoints')}
                  </p>
                </div>
                <ArrowRight
                  aria-hidden='true'
                  className='size-4 text-white/35 transition-transform group-hover:translate-x-0.5'
                />
              </Link>
            )
          })}
        </div>
      </div>
    </section>
  )
}

const ECOSYSTEM_ROWS = [
  [
    { name: 'Alibaba Cloud', icon: AlibabaCloud },
    { name: 'Amazon Web Services', icon: Aws },
    { name: 'Tencent Cloud', icon: TencentCloud },
    { name: 'Google Cloud', icon: GoogleCloud },
    { name: 'NVIDIA', icon: Nvidia },
    { name: 'Microsoft Azure', icon: Azure },
    { name: 'Cloudflare', icon: Cloudflare },
  ],
  [
    { name: 'Snowflake', icon: Snowflake },
    { name: 'Vercel', icon: Vercel },
    { name: 'OpenAI', icon: OpenAI },
    { name: 'Anthropic', icon: Anthropic },
    { name: 'Meta', icon: Meta },
    { name: 'Mistral AI', icon: Mistral },
    { name: 'Hugging Face', icon: HuggingFace },
  ],
] as const

function EcosystemLogoGroup(props: {
  items: (typeof ECOSYSTEM_ROWS)[number]
  hidden?: boolean
}) {
  return (
    <div
      className='prism-ecosystem-group'
      aria-hidden={props.hidden || undefined}
    >
      {props.items.map((item) => {
        const Icon = item.icon
        return (
          <div key={item.name} className='prism-ecosystem-logo'>
            <Icon aria-hidden='true' className='size-7 shrink-0' />
            <span>{item.name}</span>
          </div>
        )
      })}
    </div>
  )
}

export function PrismEcosystemMarquee() {
  const { t } = useTranslation()

  return (
    <section
      aria-label={t('AI and cloud ecosystem')}
      className='overflow-hidden border-y border-white/10 bg-[#0e1110] py-20 sm:py-28'
    >
      <div className='mx-auto max-w-7xl px-4 text-center sm:px-6'>
        <div className='prism-reveal mx-auto max-w-3xl'>
          <h2 className='text-4xl leading-tight font-semibold tracking-[-0.045em] sm:text-5xl'>
            {t('Connected to the AI and cloud ecosystem.')}
          </h2>
          <p className='text-muted-foreground mx-auto mt-5 max-w-2xl text-base leading-7'>
            {t(
              'Model providers, cloud platforms, and developer infrastructure meet behind one station endpoint.'
            )}
          </p>
        </div>
      </div>

      <div className='prism-ecosystem-marquee mt-12 space-y-3'>
        {ECOSYSTEM_ROWS.map((row, index) => (
          <div
            key={row[0].name}
            className={`prism-ecosystem-track ${index === 1 ? 'prism-ecosystem-track--reverse' : ''}`}
          >
            <EcosystemLogoGroup items={row} />
            <EcosystemLogoGroup items={row} hidden />
          </div>
        ))}
      </div>
    </section>
  )
}
