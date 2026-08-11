import assert from 'node:assert/strict'
import test from 'node:test'

import { getTaskActionLabelKey } from '../mappers'

test('content-derived video generation mode takes precedence over legacy action', () => {
  const label = getTaskActionLabelKey('textGenerate', 'reference_image')

  assert.equal(label, 'Image to Video')
})

test('reference video mode has its own task type label', () => {
  const label = getTaskActionLabelKey('textGenerate', 'reference_video')

  assert.equal(label, 'Reference Video')
})

test('legacy action remains the fallback for historical tasks', () => {
  const label = getTaskActionLabelKey('textGenerate')

  assert.equal(label, 'Text to Video')
})

test('public camel-case actions map to their task type labels', () => {
  const cases = [
    ['textToVideo', 'Text to Video'],
    ['imageToVideo', 'Image to Video'],
    ['firstFrame', 'First Frame to Video'],
    ['firstAndLastFrames', 'First/Last Frame to Video'],
  ] as const

  for (const [action, label] of cases) {
    assert.equal(getTaskActionLabelKey(action), label)
  }
})
