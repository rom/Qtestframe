<script setup lang="ts">
// Add or edit an OpenID Connect provider (System settings → Single
// sign-on). The client secret is write-only; "Test connection" fetches the
// issuer's discovery document on the server and reports the result.
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { useI18n } from 'vue-i18n'
import { api, errorMessage } from '@/lib/api'
import { formatRoleMapping, newProvider, parseRoleMapping, parseScopes, PROVIDER_NAME, type OidcProvider, type OidcProviderView, type TestResult } from '@/lib/systemSettings'

const props = defineProps<{ visible: boolean; provider: OidcProviderView | null; busy?: boolean; error?: string }>()
const emit = defineEmits<{ 'update:visible': [v: boolean]; save: [p: OidcProvider & { clear_secret?: boolean }] }>()
const { t } = useI18n()

const form = ref<OidcProvider>(newProvider())
const scopes = ref('')
const mapping = ref('')
const clearSecret = ref(false)
const localError = ref('')
const testing = ref(false)
const result = ref<TestResult | null>(null)
const editing = computed(() => !!props.provider)

watch(
  () => props.visible,
  (v) => {
    if (!v) return
    const p = props.provider
    form.value = p ? { ...p, client_secret: '' } : newProvider()
    scopes.value = form.value.scopes.join(' ')
    mapping.value = formatRoleMapping(form.value.role_mapping)
    clearSecret.value = false
    localError.value = ''
    result.value = null
  },
  { immediate: true },
)

const nameOk = computed(() => PROVIDER_NAME.test(form.value.name))
const canSave = computed(() => nameOk.value && !!form.value.issuer.trim() && !!form.value.client_id.trim() && !props.busy)
const secretText = computed(() => {
  const p = props.provider
  if (!p) return ''
  if (p.secret_source === 'environment') return t('sysSettings.oidc.secretFrom.environment', { env: p.secret_env })
  if (p.secret_source === 'file') return t('sysSettings.oidc.secretFrom.file')
  if (p.secret_source === 'database') return t('sysSettings.oidc.secretFrom.database')
  return t('sysSettings.oidc.secretFrom.none')
})
const endpoints = computed(() => {
  const d = result.value?.details ?? {}
  return ['authorization_endpoint', 'token_endpoint', 'userinfo_endpoint', 'jwks_uri'].filter((k) => typeof d[k] === 'string' && d[k]).map((k) => ({ k, v: String(d[k]) }))
})

function submit() {
  localError.value = ''
  const m = parseRoleMapping(mapping.value)
  if (m.bad) {
    localError.value = t('sysSettings.oidc.badMapping', { n: m.bad })
    return
  }
  emit('save', {
    ...form.value,
    name: form.value.name.trim(),
    issuer: form.value.issuer.trim(),
    client_id: form.value.client_id.trim(),
    scopes: parseScopes(scopes.value),
    role_mapping: Object.keys(m.mapping).length ? m.mapping : null,
    clear_secret: clearSecret.value || undefined,
  })
}

async function test() {
  testing.value = true
  result.value = null
  try {
    result.value = await api.post<TestResult>('/admin/settings/test/oidc', { issuer: form.value.issuer.trim() })
  } catch (e) {
    result.value = { ok: false, error: errorMessage(e) }
  } finally {
    testing.value = false
  }
}
</script>

<template>
  <Dialog :visible="visible" :header="editing ? t('sysSettings.oidc.editNamed', { name: provider?.name ?? '' }) : t('sysSettings.oidc.add')" modal style="width: min(640px, 95vw)" @update:visible="(v: boolean) => emit('update:visible', v)">
    <form id="oidc-provider-form" class="grid" style="gap: 12px" @submit.prevent="submit">
      <Message v-if="error || localError" severity="error" :closable="false">{{ localError || error }}</Message>
      <div class="oidc-grid">
        <div v-field class="field">
          <label>{{ t('sysSettings.oidc.name') }}</label>
          <InputText v-model="form.name" :disabled="editing" required maxlength="40" autocomplete="off" spellcheck="false" class="mono" aria-describedby="oidc-name-hint" :invalid="!!form.name && !nameOk" />
          <small id="oidc-name-hint" class="muted">{{ t('sysSettings.oidc.nameHint') }}</small>
        </div>
        <div v-field class="field">
          <label>{{ t('sysSettings.oidc.displayName') }}</label>
          <InputText v-model="form.display_name" maxlength="100" aria-describedby="oidc-dn-hint" />
          <small id="oidc-dn-hint" class="muted">{{ t('sysSettings.oidc.displayNameHint') }}</small>
        </div>
      </div>
      <div class="row nowrap" style="gap: 10px">
        <ToggleSwitch v-model="form.enabled" input-id="oidc-enabled" aria-describedby="oidc-enabled-hint" />
        <div>
          <label for="oidc-enabled"><b>{{ t('sysSettings.oidc.enabled') }}</b></label>
          <p id="oidc-enabled-hint" class="small muted" style="margin: 2px 0 0">{{ t('sysSettings.oidc.enabledHint') }}</p>
        </div>
      </div>
      <div v-field class="field">
        <label>{{ t('sysSettings.oidc.issuer') }}</label>
        <div class="row nowrap">
          <InputText v-model="form.issuer" type="url" required class="grow mono" spellcheck="false" aria-describedby="oidc-issuer-hint" />
          <Button type="button" :label="t('sysSettings.oidc.test')" icon="pi pi-bolt" severity="secondary" outlined :loading="testing" :disabled="!form.issuer.trim()" @click="test" />
        </div>
        <small id="oidc-issuer-hint" class="muted">{{ t('sysSettings.oidc.issuerHint') }}</small>
      </div>
      <div class="oidc-test" role="status" aria-live="polite">
        <span v-if="testing" class="small muted">{{ t('sysSettings.testing') }}</span>
        <Message v-else-if="result" :severity="result.ok ? 'success' : 'error'" :closable="false">
          <div>
            <b>{{ result.ok ? t('sysSettings.oidc.testOk') : t('sysSettings.testFailed') }}</b>
            {{ result.error ?? '' }}
          </div>
          <template v-if="result.ok && endpoints.length">
            <div class="small" style="margin-top: 4px">{{ t('sysSettings.oidc.endpoints') }}</div>
            <ul class="small mono endpoint-list">
              <li v-for="e in endpoints" :key="e.k">{{ e.k }}: {{ e.v }}</li>
            </ul>
          </template>
        </Message>
      </div>
      <div class="oidc-grid">
        <div v-field class="field">
          <label>{{ t('sysSettings.oidc.clientId') }}</label>
          <InputText v-model="form.client_id" required autocomplete="off" spellcheck="false" class="mono" />
        </div>
        <div v-field class="field">
          <label>{{ t('sysSettings.oidc.clientSecret') }}</label>
          <InputText v-model="form.client_secret" type="password" autocomplete="new-password" :placeholder="provider?.has_secret ? t('sysSettings.secretKeep') : ''" :disabled="clearSecret" aria-describedby="oidc-secret-hint" />
          <small id="oidc-secret-hint" class="muted">{{ secretText }}</small>
        </div>
      </div>
      <div v-if="provider?.secret_source === 'database'" class="row nowrap" style="gap: 6px">
        <Checkbox v-model="clearSecret" input-id="oidc-clear-secret" binary />
        <label for="oidc-clear-secret" class="small">{{ t('sysSettings.secretRemove') }}</label>
      </div>
      <div v-field class="field">
        <label>{{ t('sysSettings.oidc.scopes') }}</label>
        <InputText v-model="scopes" class="mono" spellcheck="false" aria-describedby="oidc-scopes-hint" />
        <small id="oidc-scopes-hint" class="muted">{{ t('sysSettings.oidc.scopesHint') }}</small>
      </div>
      <fieldset class="plain-fieldset oidc-set">
        <legend>{{ t('sysSettings.oidc.claimsTitle') }}</legend>
        <div class="oidc-grid three">
          <div v-field class="field"><label>{{ t('sysSettings.oidc.emailClaim') }}</label><InputText v-model="form.email_claim" class="mono" spellcheck="false" /></div>
          <div v-field class="field"><label>{{ t('sysSettings.oidc.nameClaim') }}</label><InputText v-model="form.name_claim" class="mono" spellcheck="false" /></div>
          <div v-field class="field"><label>{{ t('sysSettings.oidc.groupsClaim') }}</label><InputText v-model="form.groups_claim" class="mono" spellcheck="false" /></div>
        </div>
      </fieldset>
      <fieldset class="plain-fieldset oidc-set">
        <legend>{{ t('sysSettings.oidc.provisioningTitle') }}</legend>
        <div class="row nowrap" style="gap: 10px; margin-bottom: 8px">
          <ToggleSwitch v-model="form.auto_provision" input-id="oidc-auto" aria-describedby="oidc-auto-hint" />
          <div>
            <label for="oidc-auto"><b>{{ t('sysSettings.oidc.autoProvision') }}</b></label>
            <p id="oidc-auto-hint" class="small muted" style="margin: 2px 0 0">{{ t('sysSettings.oidc.autoProvisionHint') }}</p>
          </div>
        </div>
        <div class="oidc-grid">
          <div v-field class="field">
            <label>{{ t('sysSettings.oidc.defaultTenant') }}</label>
            <InputText v-model="form.default_tenant" class="mono" spellcheck="false" aria-describedby="oidc-tenant-hint" />
            <small id="oidc-tenant-hint" class="muted">{{ t('sysSettings.oidc.defaultTenantHint') }}</small>
          </div>
          <div v-field class="field">
            <label>{{ t('sysSettings.oidc.defaultRole') }}</label>
            <InputText v-model="form.default_role" class="mono" spellcheck="false" aria-describedby="oidc-role-hint" />
            <small id="oidc-role-hint" class="muted">{{ t('sysSettings.oidc.defaultRoleHint') }}</small>
          </div>
        </div>
        <div v-field class="field" style="margin-top: 8px">
          <label>{{ t('sysSettings.oidc.roleMapping') }}</label>
          <Textarea v-model="mapping" rows="3" auto-resize class="mono" spellcheck="false" aria-describedby="oidc-map-hint" />
          <small id="oidc-map-hint" class="muted">{{ t('sysSettings.oidc.roleMappingHint') }}</small>
        </div>
      </fieldset>
      <div v-if="provider" v-field class="field">
        <label>{{ t('sysSettings.oidc.callback') }}</label>
        <InputText :model-value="provider.callback_url" readonly class="mono" aria-describedby="oidc-cb-hint" @focus="(e: FocusEvent) => (e.target as HTMLInputElement).select()" />
        <small id="oidc-cb-hint" class="muted">{{ t('sysSettings.oidc.callbackHint') }}</small>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="emit('update:visible', false)" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" :disabled="!canSave" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.oidc-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 240px), 1fr));
  gap: 12px;
}
.oidc-grid.three {
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 160px), 1fr));
}
.oidc-set {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 10px 12px;
}
.oidc-set legend {
  font-weight: 700;
  padding: 0 4px;
}
.endpoint-list {
  margin: 4px 0 0;
  padding-left: 18px;
  word-break: break-all;
}
</style>
