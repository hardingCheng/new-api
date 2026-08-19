/*
巴西站信任区块:可靠性机制三卡 + 为巴西开发者三卡。
全部主张对应真实机制(重试兜底/多渠道冗余/透明计费/葡语全链路/USDT/美元计价),
不写不可验证的空话(SLA 百分比、客户数一律不写)。
*/
import {
  Banknote,
  Coins,
  Languages,
  Layers,
  ReceiptText,
  RefreshCcw,
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

  const brazil = [
    {
      icon: Languages,
      title: t('100% in Portuguese'),
      body: t('Interface and API error messages in Portuguese — end to end.'),
    },
    {
      icon: Coins,
      title: t('Pay with USDT'),
      body: t('No international credit card, no IOF, credited in minutes.'),
    },
    {
      icon: Banknote,
      title: t('Dollar pricing, no hidden markup'),
      body: t(
        'You pay in dollars at the listed rate. What you see on the pricing page is what you pay.'
      ),
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
            {t('Built for Brazilian developers')}
          </div>
          <div className='mt-5 grid gap-4 md:grid-cols-3'>
            {brazil.map((c) => (
              <TrustCard key={c.title} {...c} />
            ))}
          </div>
        </div>
      </div>
    </section>
  )
}
