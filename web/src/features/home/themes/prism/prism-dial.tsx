/*
巴西站首页 hero 模型转盘(参考稿方案 D 本地化,verde 绿青系)。
替换原静态占位图:真实挂牌模型名轮播聚光,中心盘同步显示当前模型。
*/
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useSystemConfig } from '@/hooks/use-system-config'

// 挂牌代表模型(与站内 pricing 一致的真实名字,纯展示无请求)
const DIAL_MODELS = [
  'claude-fable-5',
  'gpt-5.6-sol',
  'gemini-3-pro-preview',
  'claude-opus-5',
  'gpt-5.6',
  'gemini-3.6-flash',
  'claude-sonnet-4-6',
  'gpt-5.5',
]

const ROTATION_SECONDS = 24

export function PrismDial() {
  const { t } = useTranslation()
  const { systemName } = useSystemConfig()
  const n = DIAL_MODELS.length
  const [active, setActive] = useState(0)

  useEffect(() => {
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
    const step = (ROTATION_SECONDS / n) * 1000
    const timer = window.setInterval(() => {
      setActive((i) => (i + 1) % n)
    }, step)
    return () => window.clearInterval(timer)
  }, [n])

  return (
    <div className='prism-dial' aria-hidden='true'>
      <div className='prism-dial-ring' />
      <div className='prism-dial-ring inner' />
      <div className='prism-dial-arc' />
      {DIAL_MODELS.map((name, i) => (
        <div
          key={name}
          className='prism-d-hold'
          style={
            {
              '--a': `${(i * 360) / n}deg`,
              '--t': `${ROTATION_SECONDS}s`,
            } as React.CSSProperties
          }
        >
          <div
            className='prism-d-sat'
            style={{ '--a': `${(i * 360) / n}deg` } as React.CSSProperties}
          >
            <div
              className='prism-d-tile'
              style={{ '--i': i, '--n': n } as React.CSSProperties}
            >
              {name}
            </div>
          </div>
        </div>
      ))}
      <div className='prism-dial-core'>
        <div className='prism-dial-disc'>
          <small className='uppercase'>{systemName}</small>
          <b>{DIAL_MODELS[active]}</b>
          <small>{t('One account, more models')}</small>
        </div>
      </div>
    </div>
  )
}
