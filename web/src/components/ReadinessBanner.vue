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
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{ readiness: Readiness | null | undefined; projectId: string; compact?: boolean }>()
const { t } = useI18n()
const auth = useAuthStore()
const issues = computed(() => props.readiness?.issues ?? [])
const blocked = computed(() => !!props.readiness && !props.readiness.allowed)

function text(i: ReadinessIssue): string {
  switch (i.kind) {
    case 'test_status':
      return t('readiness.testStatus', { status: enumLabel('status', i.status) })
    case 'requirement_status':
      return t('readiness.requirementStatus', { status: enumLabel('status', i.status) })
    default:
      return t('readiness.dependency', { result: enumLabel('result', i.status) })
  }
}
function target(i: ReadinessIssue) {
  const name = i.kind === 'requirement_status' ? 'requirements' : 'tests'
  return { name, params: { projectId: props.projectId }, query: { item: i.id } }
}
</script>

<template>
  <Message v-if="issues.length" :severity="blocked ? 'error' : 'warn'" :closable="false" class="readiness-banner">
    <div class="grid" style="gap: 4px">
      <b>{{ blocked ? t('readiness.blockedTitle') : t('readiness.warnTitle') }}</b>
      <ul class="plain-list readiness-list">
        <li v-for="i in issues" :key="i.kind + i.id">
          <i :class="i.blocking ? 'pi pi-ban' : 'pi pi-exclamation-triangle'" aria-hidden="true" />
          <span class="sr-only">{{ i.blocking ? t('readiness.blocking') : t('readiness.warning') }}:</span>
          <RouterLink :to="target(i)" class="key">{{ i.key }}</RouterLink>
          <span v-if="!compact" class="muted ellipsis readiness-title">{{ i.title }}</span>
          <span>{{ text(i) }}</span>
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
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px;
}
.readiness-title {
  max-width: 28ch;
}
</style>
