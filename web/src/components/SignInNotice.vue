<script setup lang="ts">
// After sign-in (once per session): "Last signed in … from … (…)" and a
// warning about failed attempts. Not shown on the first login (intro tour).
import { watch } from 'vue'
import { useToast } from 'primevue/usetoast'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { failedAttempts, noticeKey, shouldShowNotice, SIGNIN_NOTICE_KEY } from '@/lib/signin'
import { formatFullDateTime, relativeTime } from '@/lib/format'
import { parseUserAgent } from '@/lib/useragent'

const auth = useAuthStore()
const toast = useToast()
const { t } = useI18n()

function readShown(): string | null {
  try {
    return sessionStorage.getItem(SIGNIN_NOTICE_KEY)
  } catch {
    return null
  }
}
function writeShown(v: string) {
  try {
    sessionStorage.setItem(SIGNIN_NOTICE_KEY, v)
  } catch {
    /* private mode */
  }
}
function device(ua: string): string {
  const info = parseUserAgent(ua)
  if (info.browser && info.os && info.device !== 'cli' && info.device !== 'bot') return t('profile.uaOn', { browser: `${info.browser}${info.version ? ' ' + info.version : ''}`, os: info.os })
  return info.device === 'unknown' && !info.browser && !info.os ? t('profile.unknownDevice') : info.label
}

watch(
  () => auth.me,
  (me) => {
    if (!me) return
    // while the intro tour is pending this session's notice is skipped (not just postponed)
    if (me.previous_login && me.user?.preferences?.intro_done !== true) {
      writeShown(noticeKey(me))
      return
    }
    if (!shouldShowNotice(me, readShown())) return
    writeShown(noticeKey(me))
    const prev = me.previous_login!
    const detail = t('signin.last', { time: relativeTime(prev.created_at), ip: prev.ip || t('common.unknown'), device: device(prev.user_agent) }) + ` (${formatFullDateTime(prev.created_at)})`
    const n = failedAttempts(me)
    if (n > 0) toast.add({ severity: 'warn', summary: t('signin.failed', { n }, n), detail, closable: true })
    else toast.add({ severity: 'info', summary: t('signin.welcomeBack'), detail, life: 10000, closable: true })
  },
  { immediate: true },
)
</script>

<template><span hidden /></template>
