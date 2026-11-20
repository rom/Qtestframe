<script setup lang="ts">
// Why a test cannot (or should not yet) be executed: a red message when the
// organisation settings block execution, an amber one when they allow it but
// the test, its requirements or its prerequisites are not ready.
import { computed } from 'vue'
import Message from 'primevue/message'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'
import { P } from '@/lib/permissions'
import type { Readiness, ReadinessIssue } from '@/lib/types'
import { explainIssue } from '@/lib/readiness'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{ readiness: Readiness | null | undefined; projectId: string; compact?: boolean }>()
const { t } = useI18n()
const auth = useAuthStore()
const issues = computed(() => props.readiness?.issues ?? [])
const blocked = computed(() => !!props.readiness && !props.readiness.allowed)

const explain = (i: ReadinessIssue) => explainIssue(i, t, enumLabel)
const blockingCount = computed(() => issues.value.filter((i) => i.blocking).length)
// blocking reasons first
const sorted = computed(() => [...issues.value].sort((a, b) => Number(b.blocking) - Number(a.blocking)))
function target(i: ReadinessIssue) {
  const name = i.kind === 'requirement_status' ? 'requirements' : 'tests'
  return { name, params: { projectId: props.projectId }, query: { item: i.id } }
}
</script>

<template>
  <Message v-if="issues.length" :severity="blocked ? 'error' : 'warn'" :closable="false" class="readiness-banner">
    <div class="grid" style="gap: 4px">
      <b>{{ blocked ? t('readiness.blockedTitleCount', { count: blockingCount }, blockingCount) : t('readiness.warnTitle') }}</b>
      <ul class="plain-list readiness-list">
        <li v-for="i in sorted" :key="i.kind + i.id">
          <div class="readiness-reason">
            <i :class="i.blocking ? 'pi pi-ban' : 'pi pi-exclamation-triangle'" aria-hidden="true" />
            <span class="sr-only">{{ i.blocking ? t('readiness.blocking') : t('readiness.warning') }}:</span>
            <RouterLink :to="target(i)" class="key">{{ i.key }}</RouterLink>
            <span v-if="!compact" class="muted ellipsis readiness-title">{{ i.title }}</span>
          </div>
          <div class="readiness-text">{{ explain(i).reason }}</div>
          <div v-if="!compact" class="small readiness-fix"><b>{{ t('readiness.whatToDo') }}</b> {{ explain(i).fix }}</div>
          <div v-if="!compact && explain(i).rule" class="small muted">{{ explain(i).rule }}</div>
        </li>
      </ul>
      <span v-if="!compact" class="small">
        {{ blocked ? t('readiness.blockedHint') : t('readiness.warnHint') }}
        <RouterLink v-if="auth.can(P.settingsManage)" :to="{ name: 'executionSettings' }">{{ t('readiness.settingsLink') }}</RouterLink>
      </span>
    </div>
  </Message>
</template>

<style>
.readiness-list li {
  display: grid;
  gap: 2px;
  padding: 4px 0;
}
.readiness-list li + li {
  border-top: 1px solid color-mix(in srgb, currentColor 18%, transparent);
}
.readiness-reason {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px;
}
.readiness-title {
  max-width: 28ch;
}
</style>
