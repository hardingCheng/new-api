/*
「使用密钥」弹窗(参照特征:按客户端生成已填真实 key 的接入配置)。
模板与教程中心(features/tutorials)保持同一口径,站点地址自动取当前 origin。
*/
import { Check, Copy } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { useSystemConfig } from '@/hooks/use-system-config'

type UseKeyDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  tokenKey: string | null
}

function CodeBlock({ label, code }: { label: string; code: string }) {
  const { copyToClipboard } = useCopyToClipboard()
  const [copied, setCopied] = useState(false)
  return (
    <div className='overflow-hidden rounded-lg border'>
      <div className='bg-muted/40 flex items-center justify-between border-b px-3 py-1.5'>
        <span className='text-muted-foreground font-mono text-[11px]'>{label}</span>
        <Button
          variant='ghost'
          size='icon-sm'
          onClick={async () => {
            const ok = await copyToClipboard(code)
            if (ok) {
              setCopied(true)
              window.setTimeout(() => setCopied(false), 1500)
            }
          }}
        >
          {copied ? <Check className='size-3.5' /> : <Copy className='size-3.5' />}
        </Button>
      </div>
      <pre className='overflow-x-auto p-3 font-mono text-xs leading-6'>{code}</pre>
    </div>
  )
}

export function UseKeyDialog({ open, onOpenChange, tokenKey }: UseKeyDialogProps) {
  const { t } = useTranslation()
  const { systemName } = useSystemConfig()
  const origin = window.location.origin
  const key = tokenKey ?? 'sk-...'

  const clients = [
    {
      value: 'claude-code',
      label: 'Claude Code',
      blocks: [
        {
          label: 'macOS / Linux / WSL',
          code: `export ANTHROPIC_BASE_URL="${origin}"\nexport ANTHROPIC_AUTH_TOKEN="${key}"\nclaude`,
        },
        {
          label: 'Windows PowerShell',
          code: `$env:ANTHROPIC_BASE_URL="${origin}"\n$env:ANTHROPIC_AUTH_TOKEN="${key}"\nclaude`,
        },
      ],
    },
    {
      value: 'codex',
      label: 'Codex CLI',
      blocks: [
        {
          label: '~/.codex/config.toml',
          code: `model_provider = "station"\nmodel = "gpt-5.6"\nmodel_reasoning_effort = "high"\n\n[model_providers.station]\nname = "${systemName}"\nbase_url = "${origin}/v1"\nrequires_openai_auth = true\nwire_api = "responses"`,
        },
        {
          label: 'macOS / Linux',
          code: `printf '%s' '${key}' | codex login --with-api-key`,
        },
        {
          label: 'Windows PowerShell',
          code: `"${key}" | codex login --with-api-key`,
        },
      ],
    },
    {
      value: 'gemini-cli',
      label: 'Gemini CLI',
      blocks: [
        {
          label: 'macOS / Linux / WSL',
          code: `export GOOGLE_GEMINI_BASE_URL="${origin}"\nexport GEMINI_API_KEY="${key}"\ngemini`,
        },
        {
          label: 'Windows PowerShell',
          code: `$env:GOOGLE_GEMINI_BASE_URL="${origin}"\n$env:GEMINI_API_KEY="${key}"\ngemini`,
        },
      ],
    },
  ]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='w-[calc(100vw-1rem)] max-w-[calc(100vw-1rem)] gap-0 p-0 sm:w-[min(92vw,44rem)] sm:max-w-[min(92vw,44rem)]'>
        <DialogHeader className='border-b px-5 py-4'>
          <DialogTitle>{t('Use API Key')}</DialogTitle>
          <DialogDescription>
            {t(
              'Use an API key created on this station. Do not use your station account password here.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='max-h-[70vh] overflow-y-auto px-5 py-4'>
          <Tabs defaultValue='claude-code'>
            <TabsList className='mb-3'>
              {clients.map((c) => (
                <TabsTrigger key={c.value} value={c.value}>
                  {c.label}
                </TabsTrigger>
              ))}
            </TabsList>
            {clients.map((c) => (
              <TabsContent key={c.value} value={c.value} className='flex flex-col gap-3'>
                {c.blocks.map((b) => (
                  <CodeBlock key={b.label} label={b.label} code={b.code} />
                ))}
              </TabsContent>
            ))}
          </Tabs>
        </div>
      </DialogContent>
    </Dialog>
  )
}
