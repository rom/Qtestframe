import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from './api'
import {
  assertionJSON,
  conditionalAvailable,
  createPasskey,
  creationOptions,
  fromBase64url,
  getPasskey,
  passkeyErrorKind,
  passkeySupport,
  registrationJSON,
  requestOptions,
  suggestName,
  toBase64url,
  type CreationOptionsJSON,
  type CredentialLike,
  type RequestOptionsJSON,
} from './passkeys'

const bytes = (...b: number[]) => new Uint8Array(b)
const buf = (...b: number[]) => bytes(...b).buffer

describe('base64url', () => {
  it('encodes without padding and with the URL alphabet', () => {
    expect(toBase64url(bytes())).toBe('')
    expect(toBase64url(bytes(0xfb, 0xff))).toBe('-_8')
    expect(toBase64url(buf(1, 2, 3, 4))).toBe('AQIDBA')
    // a view into a larger buffer encodes only its bytes
    const big = bytes(9, 1, 2, 9)
    expect(toBase64url(new Uint8Array(big.buffer, 1, 2))).toBe('AQI')
    expect(toBase64url(new DataView(big.buffer, 1, 2))).toBe('AQI')
  })
  it('decodes base64url and base64, padded or not', () => {
    expect([...fromBase64url('-_8')]).toEqual([0xfb, 0xff])
    expect([...fromBase64url('+/8=')]).toEqual([0xfb, 0xff])
    expect([...fromBase64url('AQIDBA')]).toEqual([1, 2, 3, 4])
    expect([...fromBase64url('AQIDBA==')]).toEqual([1, 2, 3, 4])
    expect(fromBase64url('').length).toBe(0)
  })
  it('round-trips random bytes', () => {
    for (let n = 0; n < 40; n++) {
      const b = new Uint8Array(n).map((_, i) => (i * 37 + n) & 0xff)
      expect([...fromBase64url(toBase64url(b))]).toEqual([...b])
    }
  })
  it('refuses other characters and impossible lengths', () => {
    expect(() => fromBase64url('a b')).toThrow()
    expect(() => fromBase64url('A')).toThrow()
    expect(() => fromBase64url('é')).toThrow()
  })
})

const creation: CreationOptionsJSON = {
  publicKey: {
    rp: { id: 'qtf.example.com', name: 'Qtestframe' },
    user: { id: 'dXNlci0x', name: 'anna@example.com', displayName: 'Anna' },
    challenge: 'AQIDBA',
    pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
    timeout: 300000,
    excludeCredentials: [{ type: 'public-key', id: 'CgsM', transports: ['internal'] }, { type: 'public-key', id: 'DQ' }],
    authenticatorSelection: { residentKey: 'required', userVerification: 'preferred' },
    attestation: 'none',
  },
}
const request: RequestOptionsJSON = {
  publicKey: { challenge: 'BQYH', rpId: 'qtf.example.com', timeout: 300000, userVerification: 'required', allowCredentials: [{ type: 'public-key', id: 'CgsM', transports: ['usb'] }] },
}

describe('options', () => {
  it('decodes the creation options', () => {
    const o = creationOptions(creation)
    expect([...new Uint8Array(o.challenge as ArrayBuffer)]).toEqual([1, 2, 3, 4])
    expect(new TextDecoder().decode(o.user.id as Uint8Array)).toBe('user-1')
    expect(o.user.name).toBe('anna@example.com')
    expect(o.excludeCredentials).toHaveLength(2)
    expect(o.excludeCredentials![0].transports).toEqual(['internal'])
    expect('transports' in o.excludeCredentials![1]).toBe(false)
    expect([...(o.excludeCredentials![1].id as Uint8Array)]).toEqual([13])
    expect(o.authenticatorSelection?.residentKey).toBe('required')
    expect(o.attestation).toBe('none')
    // defaults
    const bare = creationOptions({ publicKey: { ...creation.publicKey, excludeCredentials: undefined, attestation: undefined } })
    expect(bare.excludeCredentials).toEqual([])
    expect(bare.attestation).toBe('none')
  })
  it('decodes the request options', () => {
    const o = requestOptions(request)
    expect([...(o.challenge as Uint8Array)]).toEqual([5, 6, 7])
    expect(o.rpId).toBe('qtf.example.com')
    expect(o.userVerification).toBe('required')
    expect(o.allowCredentials![0].transports).toEqual(['usb'])
    expect(requestOptions({ publicKey: { challenge: 'AA' } }).allowCredentials).toEqual([])
  })
})

const registration = (extra: Partial<CredentialLike['response']> = {}): CredentialLike => ({
  id: 'CgsM',
  rawId: buf(10, 11, 12),
  type: 'public-key',
  authenticatorAttachment: 'platform',
  response: { clientDataJSON: buf(123, 125), attestationObject: buf(0xa0), getTransports: () => ['internal', 'hybrid'], ...extra },
})
const assertion = (userHandle: ArrayBuffer | null = buf(117)): CredentialLike => ({
  id: 'CgsM',
  rawId: buf(10, 11, 12),
  type: 'public-key',
  authenticatorAttachment: null,
  response: { clientDataJSON: buf(123, 125), authenticatorData: buf(1, 2), signature: buf(3, 4), userHandle },
})

describe('credential JSON', () => {
  it('serialises a registration', () => {
    expect(registrationJSON(registration())).toEqual({
      id: 'CgsM',
      rawId: 'CgsM',
      type: 'public-key',
      authenticatorAttachment: 'platform',
      response: { clientDataJSON: 'e30', attestationObject: 'oA', transports: ['internal', 'hybrid'] },
    })
    // browsers without getTransports, or where it throws
    expect(registrationJSON(registration({ getTransports: undefined })).response.transports).toEqual([])
    expect(
      registrationJSON(
        registration({
          getTransports: () => {
            throw new Error('x')
          },
        }),
      ).response.transports,
    ).toEqual([])
    expect(() => registrationJSON(assertion())).toThrow()
  })
  it('serialises an assertion', () => {
    expect(assertionJSON(assertion())).toEqual({
      id: 'CgsM',
      rawId: 'CgsM',
      type: 'public-key',
      response: { clientDataJSON: 'e30', authenticatorData: 'AQI', signature: 'AwQ', userHandle: 'dQ' },
    })
    expect('userHandle' in assertionJSON(assertion(null)).response).toBe(false)
    expect(() => assertionJSON(registration())).toThrow()
  })
})

describe('browser support', () => {
  it('needs PublicKeyCredential, navigator.credentials and a secure context', () => {
    expect(passkeySupport({ isSecureContext: true, PublicKeyCredential: function () {}, navigator: { credentials: {} } })).toBe('ok')
    expect(passkeySupport({ isSecureContext: false, PublicKeyCredential: function () {}, navigator: { credentials: {} } })).toBe('insecure')
    expect(passkeySupport({ isSecureContext: true, navigator: { credentials: {} } })).toBe('unsupported')
    expect(passkeySupport({ isSecureContext: true, PublicKeyCredential: function () {}, navigator: {} })).toBe('unsupported')
    expect(passkeySupport()).toBe('unsupported') // jsdom
  })
  it('detects conditional mediation', async () => {
    expect(await conditionalAvailable(undefined)).toBe(false)
    expect(await conditionalAvailable({})).toBe(false)
    expect(await conditionalAvailable({ isConditionalMediationAvailable: async () => true })).toBe(true)
    expect(await conditionalAvailable({ isConditionalMediationAvailable: async () => Promise.reject(new Error('no')) })).toBe(false)
    expect(await conditionalAvailable()).toBe(false)
  })
})

describe('ceremonies', () => {
  afterEach(() => vi.unstubAllGlobals())
  it('creates and gets credentials through navigator.credentials', async () => {
    const create = vi.fn(async () => registration())
    const get = vi.fn(async () => assertion())
    vi.stubGlobal('navigator', { credentials: { create, get }, userAgent: '' })
    expect((await createPasskey(creation)).rawId).toBe('CgsM')
    expect(create.mock.calls[0]).toHaveLength(1)
    const signal = new AbortController().signal
    expect((await getPasskey(request, { conditional: true, signal })).response.signature).toBe('AwQ')
    const arg = (get.mock.calls[0] as unknown as [CredentialRequestOptions])[0]
    expect(arg.mediation).toBe('conditional')
    expect(arg.signal).toBe(signal)
    await getPasskey(request)
    const arg2 = (get.mock.calls[1] as unknown as [CredentialRequestOptions])[0]
    expect('mediation' in arg2).toBe(false)
  })
  it('treats no credential as cancelled', async () => {
    vi.stubGlobal('navigator', { credentials: { create: async () => null, get: async () => null } })
    await expect(createPasskey(creation)).rejects.toMatchObject({ name: 'NotAllowedError' })
    await expect(getPasskey(request)).rejects.toMatchObject({ name: 'NotAllowedError' })
  })
})

describe('errors', () => {
  it('maps server codes', () => {
    const codes: Record<string, string> = {
      passkey_expired: 'expired',
      passkey_unknown: 'unknown',
      passkey_invalid: 'invalid',
      passkeys_disabled: 'disabled',
      passkeys_unavailable: 'unavailable',
      reauth_required: 'reauth',
      passkey_sso: 'sso',
      rate_limited: 'rateLimited',
    }
    for (const [code, kind] of Object.entries(codes)) expect(passkeyErrorKind(new ApiError(400, code, 'x'))).toBe(kind)
    expect(passkeyErrorKind(new ApiError(500, 'internal', 'x'))).toBeNull()
  })
  it('maps WebAuthn DOMExceptions', () => {
    expect(passkeyErrorKind(new DOMException('x', 'NotAllowedError'))).toBe('cancelled')
    expect(passkeyErrorKind(new DOMException('x', 'AbortError'))).toBe('cancelled')
    expect(passkeyErrorKind(new DOMException('x', 'InvalidStateError'))).toBe('exists')
    expect(passkeyErrorKind(new DOMException('x', 'NotSupportedError'))).toBe('unsupported')
    expect(passkeyErrorKind(new DOMException('x', 'SecurityError'))).toBe('security')
    expect(passkeyErrorKind(new Error('x'))).toBeNull()
    expect(passkeyErrorKind(null)).toBeNull()
  })
})

describe('suggestName', () => {
  it('names the platform and browser', () => {
    expect(suggestName('Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15')).toBe('Mac · Safari')
    expect(suggestName('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36 Edg/120.0')).toBe('Windows · Edge')
    expect(suggestName('Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0')).toBe('Linux · Firefox')
    expect(suggestName('Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/120.0 Mobile Safari/537.36')).toBe('Android · Chrome')
    expect(suggestName('Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)')).toBe('iPhone / iPad')
    expect(suggestName('Mozilla/5.0 (X11; CrOS x86_64)')).toBe('ChromeOS')
    expect(suggestName('curl')).toBe('')
    expect(typeof suggestName()).toBe('string')
  })
})
