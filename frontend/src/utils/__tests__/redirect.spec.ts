import { describe, expect, it } from 'vitest'

import { resolveSiteRedirect } from '../redirect'

// 站点根：会员 SPA 挂在 /short/member/ 下时的部署根（子目录场景，#48）。
const root = 'https://links.example.com/short/'
const fallback = '/'

describe('resolveSiteRedirect', () => {
  it('放行根路径写法', () => {
    expect(resolveSiteRedirect('/member/', root, fallback)).toBe('/member/')
    expect(resolveSiteRedirect('/#batch', root, fallback)).toBe('/#batch')
  })

  it('把相对值按站点根解析，子目录部署不会掉回域名根', () => {
    expect(resolveSiteRedirect('./', root, fallback)).toBe(root)
    expect(resolveSiteRedirect('./#batch', root, fallback)).toBe(`${root}#batch`)
  })

  // 登录回跳是个天然的开放重定向入口：三种"看起来像站内"的写法必须全部拒绝。
  it('拒绝协议相对与外链', () => {
    expect(resolveSiteRedirect('//evil.com/x', root, fallback)).toBe(fallback)
    expect(resolveSiteRedirect('https://evil.com/x', root, fallback)).toBe(fallback)
    expect(resolveSiteRedirect('javascript:alert(1)', root, fallback)).toBe(fallback)
  })

  it('拒绝解析后跑出站点根的相对写法', () => {
    expect(resolveSiteRedirect('../../../etc', root, fallback)).toBe(fallback)
  })

  it('空值与非法类型回落到会员中心', () => {
    expect(resolveSiteRedirect('', root, fallback)).toBe(fallback)
    expect(resolveSiteRedirect(undefined, root, fallback)).toBe(fallback)
    expect(resolveSiteRedirect(['/', '/'], root, fallback)).toBe(fallback)
  })
})
