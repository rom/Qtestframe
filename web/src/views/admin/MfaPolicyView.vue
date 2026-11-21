<script setup lang="ts">
// Administration › Multi-factor authentication: the MFA policy of the
// tenant (tenant.manage) and of the whole server (super administrators).
// A tenant policy adds to the server policy; it cannot lift it.
import { computed, onMounted, ref } from 'vue'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import MfaPolicyForm from '@/components/MfaPolicyForm.vue'
import { api, errorMessage } from '@/lib/api'
import type { MfaMode, MfaPolicy, MfaPolicyView } from '@/lib/mfa'
import { P } from '@/lib/permissions'
import { useAuthStore } from '@/stores/auth'
import { useRolesStore } from '@/stores/roles'
import { useNotify } from '@/composables/notify'
import { enumLabel } from '@/i18n/labels'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const auth = useAuthStore()
const roles = useRolesStore()
const notify = useNotify()
const view = ref<MfaPolicyView | null>(null)
const error = ref('')
const saving = ref<'' | 'tenant' | 'server'>('')

async function load() {
  error.value = ''
  try {
    view.value = await api.get<MfaPolicyView>('/admin/mfa')
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(() => {
  void roles.ensure()
  void load()
})

const tenantRoles = computed(() => roles.roles.map((r) => ({ label: enumLabel('role', r.name), value: r.name })))
const serverRoles = computed(() =>
  (view.value?.server_roles ?? []).map((r) => ({ label: enumLabel('role', r), value: r })),
)
function summary(p: MfaPolicy, opts: { label: string; value: string }[]): string {
  if (p.mode === 'roles') {
    const names = p.roles.map((r) => opts.find((o) => o.value === r)?.label ?? r).join(', ')
    return t('mfa.policy.summaryRoles', { roles: names })
  }
  return t(`mfa.policy.mode.${p.mode}`)
}

async function save(scope: 'tenant' | 'server', req: { mode: MfaMode; roles: string[]; grace_days: number; remember_days: number }) {
  saving.value = scope
  try {
    await api.put<MfaPolicy>(`/admin/mfa/${scope}`, req)
    notify.ok(t('mfa.policy.saved'))
    await load()
    void auth.fetchMe()
  } catch (e) {
    notify.fail(e)
  } finally {
    saving.value = ''
  }
}
</script>

<template>
  <div class="grid mfa-policy" style="gap: 16px; max-width: 820px">
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="!view" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <template v-else>
      <section class="card" aria-labelledby="h-mfa-overview">
        <h2 id="h-mfa-overview" class="plain-heading" style="margin: 0">{{ t('mfa.adminTitle') }}</h2>
        <p class="small muted" style="margin: 4px 0 10px">{{ t('mfa.adminIntro') }}</p>
        <ul class="stats">
          <li v-if="view.tenant">
            <b>{{ view.members_enrolled }}</b> / {{ view.members }} <span class="muted">{{ t('mfa.policy.membersEnrolled', { tenant: auth.tenant?.name ?? '' }) }}</span>
          </li>
          <li v-if="view.users !== undefined">
            <b>{{ view.users_enrolled ?? 0 }}</b> / {{ view.users }} <span class="muted">{{ t('mfa.policy.usersEnrolled') }}</span>
          </li>
        </ul>
        <p class="small muted" style="margin: 8px 0 0">{{ t('mfa.policy.resetWhere') }}</p>
      </section>

      <section v-if="view.tenant" class="card" aria-labelledby="h-mfa-tenant">
        <h2 id="h-mfa-tenant" class="plain-heading" style="margin: 0">{{ t('mfa.policy.tenantTitle', { tenant: auth.tenant?.name ?? '' }) }}</h2>
        <p class="small muted" style="margin: 4px 0 12px">{{ t('mfa.policy.tenantIntro') }}</p>
        <Message v-if="view.server.mode !== 'optional'" severity="info" :closable="false" style="margin-bottom: 12px">
          {{ t('mfa.policy.serverApplies', { summary: summary(view.server, serverRoles) }) }}
        </Message>
        <MfaPolicyForm idp="mfa-t" :policy="view.tenant" :role-options="tenantRoles" :can-edit="auth.can(P.tenantManage)" :saving="saving === 'tenant'" @save="(r) => save('tenant', r)" />
      </section>

      <section class="card" aria-labelledby="h-mfa-server">
        <h2 id="h-mfa-server" class="plain-heading" style="margin: 0">{{ t('mfa.policy.serverTitle') }}</h2>
        <p class="small muted" style="margin: 4px 0 12px">{{ t('mfa.policy.serverIntro') }}</p>
        <MfaPolicyForm v-if="view.can_edit_server" idp="mfa-s" :policy="view.server" :role-options="serverRoles" show-remember can-edit :saving="saving === 'server'" @save="(r) => save('server', r)" />
        <p v-else>
          {{ summary(view.server, serverRoles) }}<template v-if="view.server.remember_days > 0"> · {{ t('mfa.remember', { n: view.server.remember_days }, view.server.remember_days) }}</template>
          <span class="small muted"> — {{ t('mfa.policy.serverReadOnly') }}</span>
        </p>
      </section>
    </template>
  </div>
</template>

<style scoped>
.stats {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 8px 24px;
}
</style>
