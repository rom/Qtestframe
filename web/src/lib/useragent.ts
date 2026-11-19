// Minimal user-agent parsing for the active sessions list.

export type DeviceKind = 'desktop' | 'mobile' | 'tablet' | 'bot' | 'cli' | 'unknown'

export interface UserAgentInfo {
  browser: string
  version: string
  os: string
  device: DeviceKind
  /** e.g. "Firefox 128 on Windows 10/11" */
  label: string
}

const CLI: [RegExp, string][] = [
  [/^curl\/([\d.]+)/i, 'curl'],
  [/^Wget\/([\d.]+)/i, 'Wget'],
  [/^python-requests\/([\d.]+)/i, 'Python requests'],
  [/^python-urllib\/([\d.]+)/i, 'Python urllib'],
  [/^Go-http-client\/([\d.]+)/i, 'Go HTTP client'],
  [/^PostmanRuntime\/([\d.]+)/i, 'Postman'],
  [/^insomnia\/([\d.]+)/i, 'Insomnia'],
  [/^node-fetch\/([\d.]+)/i, 'node-fetch'],
  [/^axios\/([\d.]+)/i, 'axios'],
  [/^okhttp\/([\d.]+)/i, 'OkHttp'],
]

// Order matters: Edge and Opera also claim Chrome, Chrome also claims Safari.
const BROWSERS: [RegExp, string][] = [
  [/Edg(?:e|A|iOS)?\/([\d.]+)/, 'Edge'],
  [/(?:OPR|Opera)\/([\d.]+)/, 'Opera'],
  [/SamsungBrowser\/([\d.]+)/, 'Samsung Internet'],
  [/Vivaldi\/([\d.]+)/, 'Vivaldi'],
  [/YaBrowser\/([\d.]+)/, 'Yandex'],
  [/HeadlessChrome\/([\d.]+)/, 'Headless Chrome'],
  [/(?:Chrome|CriOS)\/([\d.]+)/, 'Chrome'],
  [/(?:Firefox|FxiOS)\/([\d.]+)/, 'Firefox'],
  [/Version\/([\d.]+).*Safari\//, 'Safari'],
  [/MSIE ([\d.]+)/, 'Internet Explorer'],
  [/Trident\/.*rv:([\d.]+)/, 'Internet Explorer'],
]

function windowsName(nt: string): string {
  switch (nt) {
    case '10.0':
      return 'Windows 10/11'
    case '6.3':
      return 'Windows 8.1'
    case '6.2':
      return 'Windows 8'
    case '6.1':
      return 'Windows 7'
    default:
      return 'Windows'
  }
}

function detectOS(ua: string): string {
  let m: RegExpMatchArray | null
  if ((m = ua.match(/Windows NT ([\d.]+)/))) return windowsName(m[1])
  if (/Windows/.test(ua)) return 'Windows'
  if ((m = ua.match(/(?:iPhone|CPU) OS (\d+)[_.]?(\d+)?/)) && /iPhone|iPad|iPod/.test(ua)) {
    return `${/iPad/.test(ua) ? 'iPadOS' : 'iOS'} ${m[1]}${m[2] ? '.' + m[2] : ''}`
  }
  if ((m = ua.match(/Android (\d+(?:\.\d+)?)/))) return `Android ${m[1]}`
  if (/Android/.test(ua)) return 'Android'
  if (/CrOS/.test(ua)) return 'ChromeOS'
  if ((m = ua.match(/Mac OS X (\d+)[_.](\d+)/))) return Number(m[1]) >= 11 ? 'macOS' : `macOS ${m[1]}.${m[2]}`
  if (/Macintosh|Mac OS/.test(ua)) return 'macOS'
  if (/Ubuntu/.test(ua)) return 'Ubuntu'
  if (/Fedora/.test(ua)) return 'Fedora'
  if (/Linux|X11/.test(ua)) return 'Linux'
  return ''
}

function major(v: string): string {
  return v.split('.')[0] ?? ''
}

/** Parse a User-Agent header into browser, OS and a readable label. */
export function parseUserAgent(ua: string | null | undefined): UserAgentInfo {
  const s = (ua ?? '').trim()
  if (!s) return { browser: '', version: '', os: '', device: 'unknown', label: 'Unknown device' }
  for (const [re, name] of CLI) {
    const m = s.match(re)
    if (m) return { browser: name, version: m[1], os: '', device: 'cli', label: `${name} ${m[1]}` }
  }
  if (/bot|crawler|spider|slurp/i.test(s)) {
    const name = (s.match(/([\w-]*bot[\w-]*)/i)?.[1] ?? 'Bot').replace(/^\w/, (c) => c.toUpperCase())
    return { browser: name, version: '', os: '', device: 'bot', label: name }
  }
  let browser = ''
  let version = ''
  for (const [re, name] of BROWSERS) {
    const m = s.match(re)
    if (m) {
      browser = name
      version = major(m[1])
      break
    }
  }
  const os = detectOS(s)
  const device: DeviceKind = /iPad|Tablet/.test(s) || (/Android/.test(s) && !/Mobile/.test(s))
    ? 'tablet'
    : /Mobi|iPhone|iPod|Android/.test(s)
      ? 'mobile'
      : browser || os
        ? 'desktop'
        : 'unknown'
  const b = browser ? `${browser}${version ? ' ' + version : ''}` : ''
  const label = b && os ? `${b} on ${os}` : b || os || (s.length > 60 ? s.slice(0, 59) + '…' : s)
  return { browser, version, os, device, label }
}

/** PrimeIcons class for a device kind. */
export function deviceIcon(kind: DeviceKind): string {
  switch (kind) {
    case 'mobile':
      return 'pi pi-mobile'
    case 'tablet':
      return 'pi pi-tablet'
    case 'cli':
      return 'pi pi-code'
    case 'bot':
      return 'pi pi-android'
    case 'desktop':
      return 'pi pi-desktop'
    default:
      return 'pi pi-question-circle'
  }
}
