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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

const CODE_SAMPLES = {
  OpenAI: `from openai import OpenAI

client = OpenAI(
  api_key="sk-your-key",
  base_url="https://api.example.com/v1"
)

response = client.responses.create(
  model="gpt-4.1-mini",
  input="Hello"
)

print(response.output_text)`,
  Claude: `import anthropic

client = anthropic.Anthropic(
  api_key="sk-your-key",
  base_url="https://api.example.com"
)

message = client.messages.create(
  model="claude-sonnet",
  max_tokens=1024,
  messages=[{"role": "user", "content": "Hello"}]
)`,
  Gemini: `import requests

response = requests.post(
  "https://api.example.com/v1beta/models/gemini:generateContent",
  headers={"x-goog-api-key": "sk-your-key"},
  json={
    "contents": [{
      "parts": [{"text": "Hello"}]
    }]
  }
)`,
} as const

type CodeProtocol = keyof typeof CODE_SAMPLES

const PROTOCOL_ENDPOINTS: Record<CodeProtocol, string> = {
  OpenAI: 'POST /v1/responses',
  Claude: 'POST /v1/messages',
  Gemini: 'POST /v1beta/models/...:generateContent',
}

export function PrismCodeSample() {
  const { t } = useTranslation()
  const [protocol, setProtocol] = useState<CodeProtocol>('OpenAI')
  const stationOrigin = window.location.origin
  const activeSample = CODE_SAMPLES[protocol].replaceAll(
    'https://api.example.com',
    stationOrigin
  )

  return (
    <section id='integration' className='px-4 py-20 sm:px-6 sm:py-28'>
      <div className='mx-auto max-w-7xl'>
        <div className='max-w-3xl'>
          <h2 className='text-4xl leading-tight font-semibold tracking-[-0.045em] sm:text-5xl'>
            {t('Change one address, start calling.')}
          </h2>
          <p className='text-muted-foreground mt-5 max-w-2xl text-base leading-7'>
            {t(
              'Use the protocol you already know and keep the rest of your application unchanged.'
            )}
          </p>
        </div>

        <div className='mt-14 grid grid-cols-1 gap-10 lg:grid-cols-[0.72fr_1.28fr] lg:gap-16'>
          <div className='prism-reveal lg:pt-8'>
            <span className='text-muted-foreground font-mono text-xs'>
              {t('What changes')}
            </span>
            <h3 className='mt-4 text-2xl font-semibold tracking-[-0.03em]'>
              {t('Replace the Base URL')}
            </h3>
            <div className='mt-6 overflow-x-auto border-l-2 border-[var(--prism-accent)] bg-white/[0.025] px-5 py-4 font-mono text-xs leading-6 text-[#c9cec5]'>
              base_url=
              <span className='text-[#9eb0ff]'>
                &quot;{stationOrigin}/v1&quot;
              </span>
            </div>

            <div className='mt-10 border-t border-white/10 pt-8'>
              <span className='text-muted-foreground font-mono text-xs'>
                {t('What stays')}
              </span>
              <p className='mt-4 max-w-sm text-base leading-7'>
                {t('Keep your existing SDK and request structure.')}
              </p>
            </div>
          </div>

          <div className='prism-reveal min-w-0 overflow-hidden rounded-xl border border-white/12 bg-[#111411]'>
            <div
              role='tablist'
              aria-label={t('API protocol')}
              className='flex gap-6 overflow-x-auto border-b border-white/10 px-5'
            >
              {(Object.keys(CODE_SAMPLES) as CodeProtocol[]).map((item) => (
                <button
                  key={item}
                  type='button'
                  role='tab'
                  aria-selected={protocol === item}
                  onClick={() => setProtocol(item)}
                  className={`relative py-4 font-mono text-xs font-medium transition-colors ${
                    protocol === item
                      ? 'text-foreground after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:bg-[var(--prism-accent)]'
                      : 'text-muted-foreground hover:text-foreground'
                  }`}
                >
                  {item}
                </button>
              ))}
            </div>
            <pre className='min-h-80 overflow-x-auto p-5 font-mono text-xs leading-7 text-[#c9cec5] sm:p-7'>
              <code>{activeSample}</code>
            </pre>
            <div className='grid grid-cols-1 border-t border-white/10 sm:grid-cols-3'>
              <div className='p-4 sm:border-r sm:border-white/10'>
                <span className='text-muted-foreground block text-[11px]'>
                  {t('Request')}
                </span>
                <span className='mt-1 block truncate font-mono text-xs text-[#c9cec5]'>
                  {PROTOCOL_ENDPOINTS[protocol]}
                </span>
              </div>
              <div className='border-t border-white/10 p-4 sm:border-t-0 sm:border-r'>
                <span className='text-muted-foreground block text-[11px]'>
                  {t('Routing')}
                </span>
                <span className='mt-1 block text-xs'>
                  {t('Model route selected')}
                </span>
              </div>
              <div className='border-t border-white/10 p-4 sm:border-t-0'>
                <span className='text-muted-foreground block text-[11px]'>
                  {t('Console')}
                </span>
                <span className='mt-1 block text-xs'>
                  {t('Usage record created')}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
