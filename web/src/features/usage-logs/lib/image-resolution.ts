import type { UsageLog } from '../data/schema'
import { parseLogOther } from './format'

type ImageResolutionLog = Pick<UsageLog, 'content' | 'other'>

export function getImageResolution(log: ImageResolutionLog): string {
  const other = parseLogOther(log.other)
  if (other?.image_size) return String(other.image_size)
  const match = /大小\s*([^\s，,；;]+)/.exec(log.content || '')
  return match?.[1] ?? ''
}

export function formatImageResolution(raw: string): string {
  const value = raw.trim()
  if (!value) return ''
  if (/^\d+\s*k$/i.test(value)) {
    return value.toUpperCase().replaceAll(/\s+/g, '')
  }
  const match = /^(\d+)\s*[x×]\s*(\d+)$/i.exec(value)
  if (!match) return value
  const width = Number(match[1])
  const height = Number(match[2])
  const maxDimension = Math.max(width, height)
  let tier = '8K'
  if (maxDimension <= 1024) tier = '1K'
  else if (maxDimension <= 2048) tier = '2K'
  else if (maxDimension <= 4096) tier = '4K'
  return `${tier} (${width}x${height})`
}
