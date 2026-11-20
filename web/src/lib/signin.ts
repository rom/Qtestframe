// Sign-in information: the "last signed in" notice and generated passwords.
import type { Me } from './types'

export const SIGNIN_NOTICE_KEY = 'qtf.signin.shown'

/** Identifies the current sign-in (same previous login + session token = same session). */
export function noticeKey(me: Me | null | undefined): string {
  if (!me?.previous_login) return ''
  return `${me.user?.id ?? ''}|${me.previous_login.created_at}|${(me.csrf_token ?? '').slice(0, 12)}`
}

/**
 * Whether to show the last sign-in notice: there was a previous login, it is
 * not the first login, the intro tour is done (it runs while intro_done is
 * not true) and it was not shown in this session yet.
 */
export function shouldShowNotice(me: Me | null | undefined, shownKey: string | null | undefined): boolean {
  if (!me || me.first_login || !me.previous_login || me.user?.must_change_password) return false
  if (me.user?.preferences?.intro_done !== true) return false
  return noticeKey(me) !== (shownKey ?? '')
}

/** Failed attempts to warn about (0 when none). */
export function failedAttempts(me: Me | null | undefined): number {
  const n = me?.failed_logins_since_previous
  return typeof n === 'number' && n > 0 ? Math.floor(n) : 0
}

/** Whether the user has to change the password before doing anything else. */
export function mustChangePassword(me: Me | null | undefined): boolean {
  return !!me?.user?.must_change_password
}

const ALPHABET = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789'
const SYMBOLS = '-_.!+'

/**
 * A random temporary password (no look-alike characters) with upper and
 * lower case letters, a digit and a symbol. `random(n)` returns 0…n-1.
 */
export function generatePassword(length = 16, random: (n: number) => number = cryptoRandom): string {
  const len = Math.max(12, length)
  const pick = (set: string) => set[random(set.length)]
  const chars = [pick('ABCDEFGHJKLMNPQRSTUVWXYZ'), pick('abcdefghijkmnopqrstuvwxyz'), pick('23456789'), pick(SYMBOLS)]
  while (chars.length < len) chars.push(pick(ALPHABET))
  // shuffle (Fisher–Yates)
  for (let i = chars.length - 1; i > 0; i--) {
    const j = random(i + 1)
    ;[chars[i], chars[j]] = [chars[j], chars[i]]
  }
  return chars.join('')
}

function cryptoRandom(n: number): number {
  const buf = new Uint32Array(1)
  globalThis.crypto.getRandomValues(buf)
  return buf[0] % n
}
