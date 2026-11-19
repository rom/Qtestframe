import { describe, expect, it } from 'vitest'
import { deviceIcon, parseUserAgent } from './useragent'

const UA = {
  chromeWin: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36',
  edge: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.2903.86',
  firefoxLinux: 'Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0',
  firefoxUbuntu: 'Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0',
  safariMac: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Safari/605.1.15',
  oldMac: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_9_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/49.0 Safari/537.36',
  bigSur: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 11_2) AppleWebKit/537.36 Chrome/90.0 Safari/537.36',
  iphone: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1',
  ipad: 'Mozilla/5.0 (iPad; CPU OS 16_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0 Mobile/15E148 Safari/604.1',
  android: 'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Mobile Safari/537.36',
  androidTab: 'Mozilla/5.0 (Linux; Android 13; SM-X700) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/23.0 Chrome/115.0 Safari/537.36',
  opera: 'Mozilla/5.0 (Windows NT 6.1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36 OPR/106.0',
  ie: 'Mozilla/5.0 (Windows NT 6.3; Trident/7.0; rv:11.0) like Gecko',
  ie10: 'Mozilla/5.0 (compatible; MSIE 10.0; Windows NT 6.2; Trident/6.0)',
  headless: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/131.0.0.0 Safari/537.36',
  chromeos: 'Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36',
  fedora: 'Mozilla/5.0 (X11; Fedora; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0',
}

describe('parseUserAgent', () => {
  it('parses desktop browsers', () => {
    expect(parseUserAgent(UA.chromeWin)).toEqual({ browser: 'Chrome', version: '131', os: 'Windows 10/11', device: 'desktop', label: 'Chrome 131 on Windows 10/11' })
    expect(parseUserAgent(UA.edge).label).toBe('Edge 131 on Windows 10/11')
    expect(parseUserAgent(UA.firefoxLinux).label).toBe('Firefox 133 on Linux')
    expect(parseUserAgent(UA.firefoxUbuntu).os).toBe('Ubuntu')
    expect(parseUserAgent(UA.fedora).os).toBe('Fedora')
    expect(parseUserAgent(UA.safariMac).label).toBe('Safari 18 on macOS 10.15')
    expect(parseUserAgent(UA.oldMac).os).toBe('macOS 10.9')
    expect(parseUserAgent(UA.bigSur).os).toBe('macOS')
    expect(parseUserAgent(UA.opera).label).toBe('Opera 106 on Windows 7')
    expect(parseUserAgent(UA.ie).label).toBe('Internet Explorer 11 on Windows 8.1')
    expect(parseUserAgent(UA.ie10).label).toBe('Internet Explorer 10 on Windows 8')
    expect(parseUserAgent(UA.headless).browser).toBe('Headless Chrome')
    expect(parseUserAgent(UA.chromeos).os).toBe('ChromeOS')
  })
  it('parses mobile devices', () => {
    expect(parseUserAgent(UA.iphone)).toMatchObject({ browser: 'Safari', os: 'iOS 17.5', device: 'mobile' })
    expect(parseUserAgent(UA.ipad)).toMatchObject({ browser: 'Chrome', os: 'iPadOS 16.0', device: 'tablet' })
    expect(parseUserAgent(UA.android)).toMatchObject({ os: 'Android 14', device: 'mobile' })
    expect(parseUserAgent(UA.androidTab)).toMatchObject({ browser: 'Samsung Internet', device: 'tablet' })
  })
  it('parses CLI clients and bots', () => {
    expect(parseUserAgent('curl/8.5.0')).toEqual({ browser: 'curl', version: '8.5.0', os: '', device: 'cli', label: 'curl 8.5.0' })
    expect(parseUserAgent('Go-http-client/1.1').label).toBe('Go HTTP client 1.1')
    expect(parseUserAgent('python-requests/2.32.3').device).toBe('cli')
    expect(parseUserAgent('Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)')).toMatchObject({ device: 'bot', label: 'Googlebot' })
    expect(parseUserAgent('some crawler')).toMatchObject({ device: 'bot', label: 'Bot' })
  })
  it('handles unknown input', () => {
    expect(parseUserAgent('')).toMatchObject({ device: 'unknown', label: 'Unknown device' })
    expect(parseUserAgent(null).label).toBe('Unknown device')
    expect(parseUserAgent('MyTool')).toMatchObject({ device: 'unknown', label: 'MyTool' })
    expect(parseUserAgent('x'.repeat(80)).label).toHaveLength(60)
    expect(parseUserAgent('Something (Windows; whatever)')).toMatchObject({ os: 'Windows', device: 'desktop', label: 'Windows' })
    expect(parseUserAgent('Thing (Windows NT 5.1)').os).toBe('Windows')
    expect(parseUserAgent('Foo (Android) bar').os).toBe('Android')
    expect(parseUserAgent('Foo (Macintosh)').os).toBe('macOS')
  })
  it('maps device icons', () => {
    for (const k of ['mobile', 'tablet', 'cli', 'bot', 'desktop', 'unknown'] as const) expect(deviceIcon(k)).toMatch(/^pi pi-/)
  })
})
