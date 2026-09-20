import { describe, expect, it } from 'vitest'

import { shortUrlOf } from '../constants'

// #30：管理台曾经自己拼短链地址，并兜底到一个硬编码的第三方域名，
// 于是每一次「复制短链 / 访问短链」复制到的都是别人的站。地址现在只允许来自后端。
describe('shortUrlOf', () => {
  it('原样回显后端下发的地址', () => {
    expect(shortUrlOf({ short_url: 'https://s.link/ab12cd' })).toBe('https://s.link/ab12cd')
  })

  it('后端没给地址时返回空串，绝不自己造一个绝对地址', () => {
    expect(shortUrlOf({})).toBe('')
    expect(shortUrlOf({ short_url: null })).toBe('')
    expect(shortUrlOf({ short_url: undefined })).not.toMatch(/:\/\//)
  })
})
