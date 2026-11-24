// Multi-factor authentication helpers: the enrolment QR code, the manual
// key, code input and the error kinds of the MFA endpoints.
import qrcode from 'qrcode-generator'
import { ApiError } from './api'

/** POST /auth/mfa/totp */
export interface TotpSetup {
  secret: string
  otpauth_uri: string
  issuer: string
  account: string
  digits: number
  period: number
  algorithm: string
}

/** GET /auth/mfa (also Me.mfa) */
export interface MfaStatus {
  enabled: boolean
  enabled_at: string | null
  pending: boolean
  recovery_codes_left: number
  recovery_codes_total: number
  trusted_devices: number
  required: boolean
  enrolment_deadline: string | null
  enrolment_required: boolean
  remember_days: number
  /** number of passkeys (a second factor too) */
  passkeys?: number
}

/** POST /auth/login or GET /auth/mfa/challenge when a second factor is needed. */
export interface MfaChallenge {
  mfa_required: true
  methods: string[]
  remember_days: number
  expires_at: string
}

/** A server or tenant MFA policy (GET /admin/mfa). */
export interface MfaPolicy {
  scope: string
  mode: MfaMode
  roles: string[]
  grace_days: number
  remember_days: number
  required_since: string | null
  updated_by: string
  updated_at: string | null
}

export type MfaMode = 'optional' | 'all' | 'roles'
export const MFA_MODES: MfaMode[] = ['optional', 'all', 'roles']

/** GET /admin/mfa */
export interface MfaPolicyView {
  server: MfaPolicy
  tenant: MfaPolicy | null
  can_edit_server: boolean
  members: number
  members_enrolled: number
  server_roles: string[]
  users?: number
  users_enrolled?: number
}

export function isChallenge(x: unknown): x is MfaChallenge {
  return !!x && typeof x === 'object' && (x as { mfa_required?: unknown }).mfa_required === true
}

/** The manual key in groups of four ("JBSW Y3DP …"), easier to type. */
export function groupSecret(secret: string): string {
  return (secret.replace(/\s+/g, '').match(/.{1,4}/g) ?? []).join(' ')
}

/** A typed code without spaces and dashes. */
export function normalizeCode(code: string): string {
  return code.replace(/[\s-]+/g, '')
}

/** A complete 6-digit authenticator code. */
export function isTotpCode(code: string): boolean {
  return /^\d{6}$/.test(normalizeCode(code))
}

/** A complete recovery code (12 letters or digits, dashes optional). */
export function isRecoveryCode(code: string): boolean {
  return /^[a-z0-9]{12}$/i.test(normalizeCode(code))
}

/** Whether the sign-in form may be submitted with this code. */
export function codeComplete(code: string, recovery: boolean): boolean {
  return recovery ? isRecoveryCode(code) : isTotpCode(code)
}

/** QR code modules (true = dark) of text, error correction M. */
export function qrModules(text: string): boolean[][] {
  const qr = qrcode(0, 'M')
  qr.addData(text)
  qr.make()
  const n = qr.getModuleCount()
  return Array.from({ length: n }, (_, r) => Array.from({ length: n }, (_, c) => qr.isDark(r, c)))
}

/** SVG path data drawing the dark modules (one unit per module, offset by margin). */
export function qrPath(modules: boolean[][], margin = 4): string {
  const parts: string[] = []
  modules.forEach((row, r) =>
    row.forEach((dark, c) => {
      if (dark) parts.push(`M${c + margin} ${r + margin}h1v1h-1z`)
    }),
  )
  return parts.join('')
}

/** The plain text file of recovery codes offered for download. */
export function recoveryCodesFile(codes: string[], account: string, heading: string, created: Date): string {
  return [heading, account, created.toISOString().slice(0, 10), '', ...codes, ''].join('\n')
}

/** Kind of an MFA error the UI explains in the active language. */
export type MfaErrorKind = 'invalid' | 'expired' | 'rateLimited' | 'required' | null

export function mfaErrorKind(e: unknown): MfaErrorKind {
  if (!(e instanceof ApiError)) return null
  switch (e.code) {
    case 'mfa_invalid':
      return 'invalid'
    case 'mfa_expired':
      return 'expired'
    case 'rate_limited':
      return 'rateLimited'
    case 'mfa_required':
      return 'required'
  }
  return null
}

/** Whole days until deadline (0 when due), or null without one. */
export function daysLeft(deadline: string | null | undefined, now: Date = new Date()): number | null {
  if (!deadline) return null
  const ms = new Date(deadline).getTime() - now.getTime()
  return ms <= 0 ? 0 : Math.ceil(ms / 86400000)
}
