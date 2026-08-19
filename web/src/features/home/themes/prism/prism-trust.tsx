/*
巴西站信任区块:可靠性机制三卡 + 为巴西开发者三卡。
全部主张对应真实机制(重试兜底/多渠道冗余/透明计费/葡语全链路/USDT/美元计价),
不写不可验证的空话(SLA 百分比、客户数一律不写)。
*/
import {
  Layers,
  ReceiptText,
  RefreshCcw,
  Rocket,
  SquareTerminal,
  Wallet,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

const CARD =
  'flex flex-col rounded-xl border border-white/10 bg-white/[0.02] p-6'

function TrustCard({
  icon: Icon,
  title,
  body,
}: {
  icon: typeof RefreshCcw
  title: string
  body: string
}) {
  return (
    <article className={CARD}>
      <Icon aria-hidden='true' className='size-5 text-[var(--prism-accent)]' strokeWidth={1.6} />
      <h3 className='mt-4 text-lg font-semibold tracking-[-0.02em]'>{title}</h3>
      <p className='text-muted-foreground mt-2 text-sm leading-6'>{body}</p>
    </article>
  )
}

export function PrismTrust() {
  const { t } = useTranslation()

  const reliability = [
    {
      icon: RefreshCcw,
      title: t('Automatic failover'),
      body: t(
        'If one route fails, your request is resent through another — your user never sees the error.'
      ),
    },
    {
      icon: Layers,
      title: t('Multi-provider redundancy'),
      body: t('Every model is served by multiple independent routes.'),
    },
    {
      icon: ReceiptText,
      title: t('No billing surprises'),
      body: t(
        'Prices aligned to official rates, cache served at a discount, every token logged and auditable.'
      ),
    },
  ]

  const localCards = [
    {
      icon: Wallet,
      title: t('Top up online, credited instantly'),
      body: t('Alipay and WeChat Pay supported — quota arrives in seconds.'),
    },
    {
      icon: SquareTerminal,
      title: t('Coding CLIs ready in minutes'),
      body: t(
        'Claude Code, Codex and Gemini CLI with step-by-step guides and one-click key setup.'
      ),
    },
    {
      icon: Rocket,
      title: t('New models listed as they launch'),
      body: t('Frontier models go live here right after official release.'),
    },
  ]

  return (
    <section className='border-y border-white/10 px-4 py-16 sm:px-6 sm:py-20'>
      <div className='mx-auto flex max-w-7xl flex-col gap-12'>
        <div>
          <div className='text-muted-foreground text-[11px] font-medium tracking-[0.22em] uppercase'>
            {t('Built to stay up')}
          </div>
          <div className='mt-5 grid gap-4 md:grid-cols-3'>
            {reliability.map((c) => (
              <TrustCard key={c.title} {...c} />
            ))}
          </div>
        </div>
        <div>
          <div className='text-muted-foreground text-[11px] font-medium tracking-[0.22em] uppercase'>
            {t('Built for developers here')}
          </div>
          <div className='mt-5 grid gap-4 md:grid-cols-3'>
            {localCards.map((c) => (
              <TrustCard key={c.title} {...c} />
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
