// Passkeys (WebAuthn): base64url and the JSON form of credential options
// and credentials, browser support checks and the error kinds the UI
// explains. The server sends options with binary values as base64url
// strings and expects credentials in the same form
// (PublicKeyCredential.toJSON()).
import { ApiError } from './api'

/** A passkey of the user (GET /auth/passkeys). */
export interface Passkey {
  id: string
  name: string
  credential_id: string
  alg: number
  aaguid: string
  transports: string[]
  attestation: string
  user_verified: boolean
  backup_eligible: boolean
  backed_up: boolean
  created_at: string
  last_used_at: string | null
}

/** GET /auth/passkeys */
export interface PasskeyList {
  items: Passkey[]
  enabled: boolean
  available: boolean
  reason?: string
  passwordless: boolean
  rp_id?: string
  recent_auth_until: string | null
  reauth_minutes: number
}

// ---------------------------------------------------------------- base64url

/** Bytes as unpadded base64url. */
export function toBase64url(data: ArrayBuffer | ArrayBufferView): string {
  const bytes = data instanceof ArrayBuffer ? new Uint8Array(data) : new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
  let bin = ''
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i])
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

/** base64url (or base64, padded or not) as bytes; throws on other characters. */
export function fromBase64url(s: string): Uint8Array<ArrayBuffer> {
  const b64 = s.replace(/-/g, '+').replace(/_/g, '/').replace(/=+$/, '')
  if (!/^[A-Za-z0-9+/]*$/.test(b64) || b64.length % 4 === 1) throw new Error('not base64url')
  const bin = atob(b64 + '='.repeat((4 - (b64.length % 4)) % 4))
  const out = new Uint8Array(new ArrayBuffer(bin.length))
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

// ---------------------------------------------------------------- options (server → browser)

interface DescriptorJSON {
  type: 'public-key'
  id: string
  transports?: string[]
}

/** Creation options as the server sends them ({publicKey: …}). */
export interface CreationOptionsJSON {
  publicKey: {
    rp: { id?: string; name: string }
    user: { id: string; name: string; displayName: string }
    challenge: string
    pubKeyCredParams: { type: 'public-key'; alg: number }[]
    timeout?: number
    excludeCredentials?: DescriptorJSON[]
    authenticatorSelection?: AuthenticatorSelectionCriteria
    attestation?: AttestationConveyancePreference
  }
}

/** Request options as the server sends them ({publicKey: …}). */
export interface RequestOptionsJSON {
  publicKey: {
    challenge: string
    rpId?: string
    timeout?: number
    userVerification?: UserVerificationRequirement
    allowCredentials?: DescriptorJSON[]
  }
}

function descriptor(d: DescriptorJSON): PublicKeyCredentialDescriptor {
  return { type: 'public-key', id: fromBase64url(d.id), ...(d.transports?.length ? { transports: d.transports as AuthenticatorTransport[] } : {}) }
}

/** The argument of navigator.credentials.create(). */
export function creationOptions(o: CreationOptionsJSON): PublicKeyCredentialCreationOptions {
  const pk = o.publicKey
  return {
    rp: pk.rp,
    user: { ...pk.user, id: fromBase64url(pk.user.id) },
    challenge: fromBase64url(pk.challenge),
    pubKeyCredParams: pk.pubKeyCredParams,
    timeout: pk.timeout,
    excludeCredentials: (pk.excludeCredentials ?? []).map(descriptor),
    authenticatorSelection: pk.authenticatorSelection,
    attestation: pk.attestation ?? 'none',
  }
}

/** The publicKey argument of navigator.credentials.get(). */
export function requestOptions(o: RequestOptionsJSON): PublicKeyCredentialRequestOptions {
  const pk = o.publicKey
  return {
    challenge: fromBase64url(pk.challenge),
    rpId: pk.rpId,
    timeout: pk.timeout,
    userVerification: pk.userVerification,
    allowCredentials: (pk.allowCredentials ?? []).map(descriptor),
  }
}

// ---------------------------------------------------------------- credentials (browser → server)

/** The JSON the server reads of a registration. */
export interface RegistrationJSON {
  id: string
  rawId: string
  type: string
  authenticatorAttachment?: string
  response: { clientDataJSON: string; attestationObject: string; transports: string[] }
}

/** The JSON the server reads of an assertion. */
export interface AssertionJSON {
  id: string
  rawId: string
  type: string
  authenticatorAttachment?: string
  response: { clientDataJSON: string; authenticatorData: string; signature: string; userHandle?: string }
}

/** The parts of a PublicKeyCredential the helpers read (also in tests). */
export interface CredentialLike {
  id: string
  rawId: ArrayBuffer
  type: string
  authenticatorAttachment?: string | null
  response: {
    clientDataJSON: ArrayBuffer
    attestationObject?: ArrayBuffer
    getTransports?: () => string[]
    authenticatorData?: ArrayBuffer
    signature?: ArrayBuffer
    userHandle?: ArrayBuffer | null
  }
}

function attachment(c: CredentialLike) {
  return c.authenticatorAttachment ? { authenticatorAttachment: c.authenticatorAttachment } : {}
}

/** A new credential (navigator.credentials.create()) as JSON. */
export function registrationJSON(c: CredentialLike): RegistrationJSON {
  if (!c.response.attestationObject) throw new Error('not a registration')
  let transports: string[] = []
  try {
    transports = c.response.getTransports?.() ?? []
  } catch {
    transports = []
  }
  return {
    id: c.id,
    rawId: toBase64url(c.rawId),
    type: c.type,
    ...attachment(c),
    response: { clientDataJSON: toBase64url(c.response.clientDataJSON), attestationObject: toBase64url(c.response.attestationObject), transports },
  }
}

/** An assertion (navigator.credentials.get()) as JSON. */
export function assertionJSON(c: CredentialLike): AssertionJSON {
  const r = c.response
  if (!r.authenticatorData || !r.signature) throw new Error('not an assertion')
  return {
    id: c.id,
    rawId: toBase64url(c.rawId),
    type: c.type,
    ...attachment(c),
    response: {
      clientDataJSON: toBase64url(r.clientDataJSON),
      authenticatorData: toBase64url(r.authenticatorData),
      signature: toBase64url(r.signature),
      ...(r.userHandle ? { userHandle: toBase64url(r.userHandle) } : {}),
    },
  }
}

// ---------------------------------------------------------------- browser support

/** Why passkeys cannot be used in this browser, or null when they can. */
export type PasskeySupport = 'ok' | 'insecure' | 'unsupported'

export function passkeySupport(w: { isSecureContext?: boolean; PublicKeyCredential?: unknown; navigator?: { credentials?: unknown } } = globalThis): PasskeySupport {
  if (!w.PublicKeyCredential || !w.navigator?.credentials) return 'unsupported'
  if (w.isSecureContext === false) return 'insecure'
  return 'ok'
}

/** Whether the browser can suggest passkeys in the e-mail field (conditional mediation). */
export async function conditionalAvailable(pkc: unknown = (globalThis as { PublicKeyCredential?: unknown }).PublicKeyCredential): Promise<boolean> {
  const f = (pkc as { isConditionalMediationAvailable?: () => Promise<boolean> } | undefined)?.isConditionalMediationAvailable
  if (typeof f !== 'function') return false
  try {
    return await f.call(pkc)
  } catch {
    return false
  }
}

// ---------------------------------------------------------------- ceremonies

/** Runs navigator.credentials.create() for the options and returns its JSON. */
export async function createPasskey(o: CreationOptionsJSON): Promise<RegistrationJSON> {
  const c = (await navigator.credentials.create({ publicKey: creationOptions(o) })) as unknown as CredentialLike | null
  if (!c) throw new DOMException('no credential', 'NotAllowedError')
  return registrationJSON(c)
}

/** Runs navigator.credentials.get() (conditional: as suggestions of the e-mail field). */
export async function getPasskey(o: RequestOptionsJSON, opts: { conditional?: boolean; signal?: AbortSignal } = {}): Promise<AssertionJSON> {
  const c = (await navigator.credentials.get({
    publicKey: requestOptions(o),
    ...(opts.conditional ? { mediation: 'conditional' as CredentialMediationRequirement } : {}),
    ...(opts.signal ? { signal: opts.signal } : {}),
  })) as unknown as CredentialLike | null
  if (!c) throw new DOMException('no credential', 'NotAllowedError')
  return assertionJSON(c)
}

// ---------------------------------------------------------------- errors

/** Kind of a passkey error the UI explains in the active language. */
export type PasskeyErrorKind =
  | 'cancelled'
  | 'exists'
  | 'unsupported'
  | 'security'
  | 'expired'
  | 'unknown'
  | 'invalid'
  | 'disabled'
  | 'unavailable'
  | 'reauth'
  | 'sso'
  | 'rateLimited'
  | null

export function passkeyErrorKind(e: unknown): PasskeyErrorKind {
  if (e instanceof ApiError) {
    switch (e.code) {
      case 'passkey_expired':
        return 'expired'
      case 'passkey_unknown':
        return 'unknown'
      case 'passkey_invalid':
        return 'invalid'
      case 'passkeys_disabled':
        return 'disabled'
      case 'passkeys_unavailable':
        return 'unavailable'
      case 'reauth_required':
        return 'reauth'
      case 'passkey_sso':
        return 'sso'
      case 'rate_limited':
        return 'rateLimited'
    }
    return null
  }
  const name = (e as { name?: unknown } | null)?.name
  switch (name) {
    case 'NotAllowedError':
    case 'AbortError':
      return 'cancelled'
    case 'InvalidStateError':
      return 'exists'
    case 'NotSupportedError':
      return 'unsupported'
    case 'SecurityError':
      return 'security'
  }
  return null
}

/** A default name for a new passkey from the browser's platform. */
export function suggestName(ua: string = globalThis.navigator?.userAgent ?? ''): string {
  const platforms: [RegExp, string][] = [
    [/iPhone|iPad|iPod/, 'iPhone / iPad'],
    [/Android/, 'Android'],
    [/Mac OS X|Macintosh/, 'Mac'],
    [/Windows/, 'Windows'],
    [/CrOS/, 'ChromeOS'],
    [/Linux/, 'Linux'],
  ]
  const browsers: [RegExp, string][] = [
    [/Edg\//, 'Edge'],
    [/Firefox\//, 'Firefox'],
    [/Chrome\//, 'Chrome'],
    [/Safari\//, 'Safari'],
  ]
  const p = platforms.find(([re]) => re.test(ua))?.[1]
  const b = browsers.find(([re]) => re.test(ua))?.[1]
  return [p, b].filter(Boolean).join(' · ')
}
