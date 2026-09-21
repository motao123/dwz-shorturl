/**
 * 把登录后回跳的 redirect 参数解析成站内目标，挡掉跳出本站的写法。
 *
 * 只接受两类值（#48）：
 *   - 根路径 `/member/`：本站绝对路径；
 *   - 相对路径 `./`、`./#batch`：静态首页在子目录部署里拼不出根路径，只能给相对值，
 *     按传入的站点根解析。
 * 其余一律回落到 fallback（会员中心首页），包括：
 *   - `//evil.com`（协议相对 URL，直接用会跳到外站）；
 *   - `https://evil.com`（绝对外链）；
 *   - `../..` 之类解析后跑出站点根的写法。
 */
export function resolveSiteRedirect(raw: unknown, siteRoot: string, fallback = '/'): string {
  if (typeof raw !== 'string' || raw === '') return fallback
  if (raw.startsWith('//')) return fallback
  if (raw.startsWith('/')) return raw
  let abs: string
  try {
    abs = new URL(raw, siteRoot).href
  } catch {
    return fallback
  }
  return abs.startsWith(siteRoot) ? abs : fallback
}
