<script setup lang="ts">
// The built-in consistency checks of a project (GET analytics/consistency):
// every check with its result, the violating items with the items involved
// and fix actions, and installing the checks the project does not have.
import ItemRef from './ItemRef.vue'
import ConsistencyViolation from './ConsistencyViolation.vue'
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import { api, errorMessage } from '@/lib/api'
import { itemRoute } from '@/lib/consistency'
import { rulesRoute } from '@/lib/traceRules'
import { P } from '@/lib/permissions'
import type { ConsistencyReport, TraceRuleResult } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  projectId: string
  /** dashboard: a short summary with the failing checks only */
  compact?: boolean
  /** a dashboard widget: no card and title of its own (the frame has them) */
  bare?: boolean
}>()
const { t } = useI18n()
const auth = useAuthStore()
const notify = useNotify()
const report = ref<ConsistencyReport | null>(null)
const loading = ref(false)
const error = ref('')
const open = ref<Record<string, boolean>>({})
const installing = ref(false)
const SHOWN = 20

async function load() {
  loading.value = true
  error.value = ''
  try {
    report.value = await api.get<ConsistencyReport>(`/projects/${props.projectId}/analytics/consistency`, { limit: 100 })
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.projectId, load, { immediate: true })
defineExpose({ reload: load })

const results = computed(() => (report.value?.results ?? []).filter((r) => !props.compact || !r.passed))
const failing = computed(() => (report.value?.results ?? []).filter((r) => !r.passed).length)
const total = computed(() => (report.value?.results ?? []).reduce((n, r) => n + r.violation_count, 0))
const missing = computed(() => (report.value ? report.value.checks.length - report.value.installed : 0))
const canInstall = computed(() => auth.can(P.projectUpdate))
const installed = (r: TraceRuleResult) => !r.rule.id.startsWith('builtin:')
const checkName = (r: TraceRuleResult) => (installed(r) ? r.rule.name : t(`consistency.check.${r.rule.kind}.name`))

async function install() {
  installing.value = true
  try {
    const res = await api.post<{ items: unknown[] }>(`/projects/${props.projectId}/trace-rules/builtins`)
    notify.ok(t('consistency.installed', { n: res.items.length }, res.items.length))
    await load()
  } catch (e) {
    notify.fail(e, t('consistency.installFailed'))
  } finally {
    installing.value = false
  }
}
</script>

<template>
  <section :class="['consistency', { card: !bare }]" :aria-busy="loading" :aria-labelledby="bare ? undefined : 'h-consistency'">
    <div class="row" style="justify-content: space-between; flex-wrap: wrap; gap: 8px">
      <h2 v-if="!bare" id="h-consistency" class="plain-heading">{{ t('consistency.title') }}</h2>
      <span v-else />
      <div class="row" style="gap: 8px; flex-wrap: wrap">
        <RouterLink :to="rulesRoute(projectId)" class="small">{{ t('consistency.manage') }}</RouterLink>
        <Button v-if="!compact" icon="pi pi-refresh" text rounded size="small" severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" :loading="loading" @click="load" />
      </div>
    </div>
    <p class="small muted" style="margin-top: 0">{{ t('consistency.intro') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="!report && loading" class="empty"><ProgressSpinner style="width: 28px; height: 28px" :aria-label="t('common.loading')" /></div>
    <template v-else-if="report">
      <div v-if="missing" class="row small install" role="note">
        <i class="pi pi-info-circle" aria-hidden="true" />
        <span class="grow">{{ t('consistency.notInstalled', { n: missing }, missing) }}</span>
        <Button v-if="canInstall" :label="t('consistency.install')" icon="pi pi-plus" size="small" :loading="installing" @click="install" />
      </div>
      <p class="rule-verdict" role="status">
        <i :class="failing ? 'pi pi-exclamation-triangle warn-icon' : 'pi pi-check-circle ok-icon'" aria-hidden="true" />
        {{ failing ? t('consistency.summary', { checks: failing, n: total }, total) : t('consistency.allPassed') }}
      </p>
      <ul class="plain-list checks">
        <li v-for="r in results" :key="r.rule.id" class="check" :class="{ fail: !r.passed }">
          <div class="row check-head">
            <i :class="r.passed ? 'pi pi-check-circle ok-icon' : 'pi pi-exclamation-triangle warn-icon'" aria-hidden="true" />
            <span class="sr-only">{{ r.passed ? t('traceRules.passed') : t('traceRules.failed') }}:</span>
            <b class="grow">{{ checkName(r) }}</b>
            <Tag v-if="!installed(r)" :value="t('consistency.default')" severity="secondary" />
            <Tag v-else-if="!r.rule.enabled" :value="t('traceRules.disabled')" severity="secondary" />
            <Tag v-else :value="t(`traceRules.severity.${r.rule.severity}`)" :severity="r.rule.severity === 'warning' ? 'warn' : 'danger'" />
            <span class="small mono nowrap">{{ t('consistency.count', { n: r.violation_count }, r.violation_count) }}</span>
          </div>
          <div v-if="!compact" class="small muted">{{ t(`consistency.check.${r.rule.kind}.description`) }}</div>
          <template v-if="r.violation_count">
            <Button
              size="small"
              text
              :icon="open[r.rule.id] ? 'pi pi-chevron-down' : 'pi pi-chevron-right'"
              :label="t('traceRules.showViolations', { n: r.violation_count }, r.violation_count)"
              :aria-expanded="!!open[r.rule.id]"
              :aria-controls="'cv-' + r.rule.kind"
              class="toggle"
              @click="open[r.rule.id] = !open[r.rule.id]"
            />
            <ul v-if="open[r.rule.id]" :id="'cv-' + r.rule.kind" class="plain-list violations" :aria-label="t('traceRules.violationsOf', { name: checkName(r) })">
              <li v-for="v in r.violations.slice(0, SHOWN)" :key="v.kind + v.id" class="violation">
                <span class="sr-only">{{ t(`consistency.kind.${v.kind}`) }}</span>
                <ItemRef :to="itemRoute(v, projectId)" :item-key="v.key" :title="v.title" :hint="v.title" title-class="ellipsis" />
                <ConsistencyViolation :violation="v" :check="r.rule.kind" :project-id="projectId" @changed="load" />
              </li>
              <li v-if="r.violation_count > Math.min(SHOWN, r.violations.length)" class="small muted">
                {{ t('traceRules.more', { n: r.violation_count - Math.min(SHOWN, r.violations.length) }) }}
                <RouterLink :to="rulesRoute(projectId)">{{ t('consistency.manage') }}</RouterLink>
              </li>
            </ul>
          </template>
        </li>
      </ul>
    </template>
  </section>
</template>

<style scoped>
.consistency { margin-top: 16px; }
.install { gap: 8px; padding: 8px 10px; border: 1px solid var(--qtf-border); border-radius: 8px; margin-bottom: 10px; flex-wrap: wrap; }
.rule-verdict { display: flex; align-items: center; gap: 8px; font-weight: 600; margin: 0 0 8px; }
.ok-icon { color: var(--p-green-600, #16a34a); }
.warn-icon { color: var(--p-amber-600, #d97706); }
:global(.app-dark) .ok-icon { color: var(--p-green-400, #4ade80); }
:global(.app-dark) .warn-icon { color: var(--p-amber-400, #fbbf24); }
.checks { display: grid; gap: 0; }
.check { padding: 8px 0; border-top: 1px solid var(--qtf-border); display: grid; gap: 2px; }
.check-head { flex-wrap: wrap; gap: 6px 8px; }
.toggle { justify-self: start; }
.violations { display: grid; gap: 8px; padding-left: 8px; }
.violation { display: grid; gap: 2px; min-width: 0; }
</style>
