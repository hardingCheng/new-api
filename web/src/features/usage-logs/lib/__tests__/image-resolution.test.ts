import assert from 'node:assert/strict'
import test from 'node:test'

import { formatImageResolution, getImageResolution } from '../image-resolution'

test('image edit error log displays structured request resolution', () => {
  const raw = getImageResolution({
    content: 'status_code=400, size is required',
    other: JSON.stringify({
      request_path: '/v1/images/edits',
      image_size: '1536x2736',
    }),
  })

  assert.equal(formatImageResolution(raw), '4K (1536x2736)')
})

test('legacy image log displays resolution from content', () => {
  const raw = getImageResolution({
    content: '大小 3312x2480, 品质 standard, 生成数量 1',
    other: '{}',
  })

  assert.equal(formatImageResolution(raw), '4K (3312x2480)')
})
