<script setup lang="ts">
// Organisation settings for test execution (GET/PUT /settings): whether
// tests that are not ready, tests of requirements that are not ready, and
// tests whose prerequisites have not passed may be executed. Editable with
// settings.manage (admins and test leads by default).
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import ProgressSpinner from 'primevue/progressspinner'
import ToggleSwitch from 'primevue/toggleswitch'
import HelpTip from '@/guide/HelpTip.vue'
import { api, errorMessage } from '@/lib/api'
import { P } from '@/lib/permissions'
import type { TenantSettings } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumOptions } from '@/i18n/labels'

const { t } = useI18n()
const auth = useAuthStore()
const meta = useMetaStore()
const notify = useNotify()
const form = ref<TenantSettings | null>(null)
const saved = ref('')
const error = ref('')
const saving = ref(false)
const canEdit = computed(() => auth.can(P.settingsManage))
const statusOptions = computed(() => enumOptions('status', meta.meta.requirement_statuses))
const dirty = computed(() => !!form.value && JSON.stringify(form.value) !== saved.value)

async function load() {
  error.value = ''
  try {
    form.value = await api.get<TenantSettings>('/settings')
    saved.value = JSON.stringify(form.value)
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(load)

async function save() {
  if (!form.value) return
  saving.value = true
  try {
    form.value = await api.put<TenantSettings>('/settings', form.value)
    saved.value = JSON.stringify(form.value)
    notify.ok(t('execSettings.saved'))
  } catch (e) {
    notify.fail(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="card" style="max-width: 820px">
    <div class="row" style="gap: 6px">
      <h2 class="plain-heading" style="margin: 0">{{ t('execSettings.title') }}</h2>
      <HelpTip id="settings.readiness" :label="t('execSettings.title')" />
    </div>
    <p class="small muted" style="margin-top: 4px">{{ t('execSettings.intro') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="!form" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <form v-else class="grid" style="gap: 18px" @submit.prevent="save">
      <Message v-if="!canEdit" severity="info" :closable="false">{{ t('execSettings.readOnly') }}</Message>
      <fieldset class="plain-fieldset setting">
        <legend class="setting-title">{{ t('execSettings.testsTitle') }}</legend>
        <div class="row nowrap" style="gap: 10px; align-items: flex-start">
          <ToggleSwitch v-model="form.allow_unready_tests" input-id="s-tests" :disabled="!canEdit" aria-describedby="s-tests-desc" />
          <div>
            <label for="s-tests"><b>{{ t('execSettings.allowUnreadyTests') }}</b></label>
            <p id="s-tests-desc" class="small muted" style="margin: 2px 0 0">{{ form.allow_unready_tests ? t('execSettings.testsOn') : t('execSettings.testsOff') }}</p>
          </div>
        </div>
      </fieldset>
      <fieldset class="plain-fieldset setting">
        <legend class="setting-title">{{ t('execSettings.requirementsTitle') }}</legend>
        <div v-field class="field">
          <label>{{ t('execSettings.readyStatuses') }}</label>
          <MultiSelect v-model="form.ready_requirement_statuses" :options="statusOptions" option-label="label" option-value="value" display="chip" :disabled="!canEdit" />
          <small class="muted">{{ t('execSettings.readyStatusesHint') }}</small>
        </div>
        <div class="row nowrap" style="gap: 10px; align-items: flex-start; margin-top: 10px">
          <ToggleSwitch v-model="form.allow_unready_requirements" input-id="s-reqs" :disabled="!canEdit" aria-describedby="s-reqs-desc" />
          <div>
            <label for="s-reqs"><b>{{ t('execSettings.allowUnreadyRequirements') }}</b></label>
            <p id="s-reqs-desc" class="small muted" style="margin: 2px 0 0">{{ form.allow_unready_requirements ? t('execSettings.reqsOn') : t('execSettings.reqsOff') }}</p>
          </div>
        </div>
      </fieldset>
      <fieldset class="plain-fieldset setting">
        <legend class="setting-title">{{ t('execSettings.depsTitle') }}</legend>
        <div class="row nowrap" style="gap: 10px; align-items: flex-start">
          <ToggleSwitch v-model="form.allow_unmet_dependencies" input-id="s-deps" :disabled="!canEdit" aria-describedby="s-deps-desc" />
          <div>
            <label for="s-deps"><b>{{ t('execSettings.allowUnmetDependencies') }}</b></label>
            <p id="s-deps-desc" class="small muted" style="margin: 2px 0 0">{{ form.allow_unmet_dependencies ? t('execSettings.depsOn') : t('execSettings.depsOff') }}</p>
          </div>
        </div>
      </fieldset>
      <fieldset class="plain-fieldset setting">
        <legend class="setting-title">{{ t('execSettings.evidenceTitle') }}</legend>
        <div class="row nowrap" style="gap: 10px; align-items: flex-start">
          <ToggleSwitch :model-value="!form.manual_verification" input-id="s-evidence" :disabled="!canEdit" aria-describedby="s-evidence-desc" @update:model-value="(v: boolean) => (form!.manual_verification = !v)" />
          <div>
            <label for="s-evidence"><b>{{ t('execSettings.statusFollowsEvidence') }}</b></label>
            <p id="s-evidence-desc" class="small muted" style="margin: 2px 0 0">{{ form.manual_verification ? t('execSettings.evidenceOff') : t('execSettings.evidenceOn') }}</p>
          </div>
        </div>
      </fieldset>
      <div v-if="canEdit" class="row" style="justify-content: flex-end">
        <span v-if="dirty" class="small muted" style="margin-right: auto">{{ t('execSettings.unsaved') }}</span>
        <Button :label="t('common.cancel')" text severity="secondary" type="button" :disabled="!dirty" @click="load" />
        <Button :label="t('common.save')" icon="pi pi-check" type="submit" :loading="saving" :disabled="!dirty" />
      </div>
    </form>
  </div>
</template>

<style scoped>
.setting {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 12px 14px;
}
.setting-title {
  font-weight: 700;
  padding: 0 4px;
}
</style>
