<script setup lang="ts">
// Create or edit a test environment: what it is, its components and
// versions, its state, capacity and approval, the credentials reference and
// the write-only secret, the health check.
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import TagsInput from './TagsInput.vue'
import { useI18n } from 'vue-i18n'
import { api, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { cleanComponents, duplicateComponents, ENVIRONMENT_STATUSES, ENVIRONMENT_TYPES, type Component, type Environment } from '@/lib/environments'
import { useProjectPeopleStore } from '@/stores/projectPeople'
import { useConfigurations } from '@/composables/configurations'
import { useVariants } from '@/composables/variants'

const props = defineProps<{ projectId: string; environment?: Environment | null; canShare?: boolean }>()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ saved: [env: Environment] }>()
const { t } = useI18n()
const id = uid('envd')
const people = useProjectPeopleStore()
const cfg = useConfigurations(() => props.projectId)
const vs = useVariants(() => props.projectId)

const blank = () => ({
  name: '',
  type: 'test',
  status: 'available',
  status_note: '',
  description: '',
  urls: [] as string[],
  owner_id: '',
  tags: [] as string[],
  configuration_ids: [] as string[],
  variant_ids: [] as string[],
  capacity: 1,
  requires_approval: false,
  credentials_ref: '',
  secret: '',
  clear_secret: false,
  health_url: '',
  health_interval_minutes: 5,
  shared: false,
})
const fm = ref(blank())
const comps = ref<Component[]>([])
const urlText = ref('')
const error = ref('')
const busy = ref(false)
const editing = computed(() => !!props.environment)
watch(visible, (open) => {
  if (!open) return
  Object.assign(fm.value, blank())
  error.value = ''
  const e = props.environment
  if (e) {
    Object.assign(fm.value, {
      name: e.name,
      type: e.type,
      status: e.status,
      status_note: e.status_note,
      description: e.description,
      owner_id: e.owner_id,
      tags: [...e.tags],
      configuration_ids: [...e.configuration_ids],
      variant_ids: [...e.variant_ids],
      capacity: e.capacity,
      requires_approval: e.requires_approval,
      credentials_ref: e.credentials_ref,
      health_url: e.health_url,
      health_interval_minutes: e.health_interval_minutes,
      shared: e.shared,
    })
    comps.value = e.components.map((c) => ({ ...c }))
    urlText.value = e.urls.join('\n')
  } else {
    comps.value = [{ name: '', version: '' }]
    urlText.value = ''
  }
  void people.ensure(props.projectId)
  void cfg.load()
  void vs.load()
})
const typeOptions = computed(() => ENVIRONMENT_TYPES.map((v) => ({ label: t(`environments.type.${v}`), value: v })))
const statusOptions = computed(() => ENVIRONMENT_STATUSES.map((v) => ({ label: t(`environments.status.${v}`), value: v })))
const ownerOptions = computed(() => [
  { label: t('environments.noOwner'), value: '' },
  ...people.people.filter((p) => p.kind === 'user' && p.user_id).map((p) => ({ label: p.name && p.email ? `${p.name} (${p.email})` : p.name || p.email || '', value: p.user_id! })),
])
const variantOpts = computed(() => vs.variants.value.map((v) => ({ label: v.name, value: v.id })))
const dups = computed(() => duplicateComponents(comps.value))
const scopeShared = computed(() => (editing.value ? !!props.environment?.shared : fm.value.shared))

async function save() {
  busy.value = true
  error.value = ''
  const body: Record<string, unknown> = {
    name: fm.value.name,
    type: fm.value.type,
    status: fm.value.status,
    status_note: fm.value.status_note,
    description: fm.value.description,
    urls: urlText.value.split(/\n/).map((s) => s.trim()).filter(Boolean),
    components: cleanComponents(comps.value),
    owner_id: fm.value.owner_id,
    tags: fm.value.tags,
    capacity: fm.value.capacity,
    requires_approval: fm.value.requires_approval,
    credentials_ref: fm.value.credentials_ref,
    health_url: fm.value.health_url.trim(),
    health_interval_minutes: fm.value.health_interval_minutes,
  }
  if (!scopeShared.value) {
    body.configuration_ids = fm.value.configuration_ids
    body.variant_ids = fm.value.variant_ids
  }
  if (fm.value.secret) body.secret = fm.value.secret
  if (fm.value.clear_secret) body.clear_secret = true
  try {
    const out = editing.value
      ? await api.patch<Environment>(`/environments/${props.environment!.id}`, body)
      : await api.post<Environment>(`/projects/${props.projectId}/environments`, { ...body, shared: fm.value.shared })
    visible.value = false
    emit('saved', out)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal maximizable :maximize-button-props="{ 'aria-label': t('common.toggleMaximize') }" :header="editing ? t('environments.editHeader', { name: environment?.name }) : t('environments.new')" style="width: min(820px, 96vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div class="form-grid">
        <div class="field">
          <label :for="`${id}-name`">{{ t('common.name') }}</label>
          <InputText :id="`${id}-name`" v-model="fm.name" maxlength="200" required autofocus />
        </div>
        <div class="field">
          <label :for="`${id}-type`">{{ t('environments.typeLabel') }}</label>
          <Select v-model="fm.type" :input-id="`${id}-type`" :options="typeOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label :for="`${id}-status`">{{ t('common.status') }}</label>
          <Select v-model="fm.status" :input-id="`${id}-status`" :options="statusOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label :for="`${id}-note`">{{ t('environments.statusNote') }}</label>
          <InputText :id="`${id}-note`" v-model="fm.status_note" maxlength="2000" :placeholder="t('environments.statusNotePlaceholder')" />
        </div>
        <div class="field">
          <label :for="`${id}-owner`">{{ t('environments.owner') }}</label>
          <Select v-model="fm.owner_id" :input-id="`${id}-owner`" :options="ownerOptions" option-label="label" option-value="value" filter />
        </div>
        <div class="field">
          <label :for="`${id}-cap`">{{ t('environments.capacity') }}</label>
          <InputNumber v-model="fm.capacity" :input-id="`${id}-cap`" :min="1" :max="1000" show-buttons :aria-describedby="`${id}-cap-help`" />
          <small :id="`${id}-cap-help`" class="muted">{{ t('environments.capacityHelp') }}</small>
        </div>
        <div class="field full row" style="gap: 8px">
          <Checkbox v-model="fm.requires_approval" binary :input-id="`${id}-appr`" />
          <label :for="`${id}-appr`">{{ t('environments.requiresApproval') }}</label>
        </div>
        <div v-if="!editing && canShare" class="field full row" style="gap: 8px">
          <Checkbox v-model="fm.shared" binary :input-id="`${id}-shared`" :aria-describedby="`${id}-shared-help`" />
          <label :for="`${id}-shared`">{{ t('environments.sharedLabel') }}</label>
          <small :id="`${id}-shared-help`" class="muted">{{ t('environments.sharedHelp') }}</small>
        </div>
        <div class="field full">
          <label :for="`${id}-desc`">{{ t('common.description') }}</label>
          <Textarea :id="`${id}-desc`" v-model="fm.description" rows="2" auto-resize />
        </div>
        <div class="field full">
          <label :for="`${id}-urls`">{{ t('environments.urls') }}</label>
          <Textarea :id="`${id}-urls`" v-model="urlText" rows="2" auto-resize :aria-describedby="`${id}-urls-help`" />
          <small :id="`${id}-urls-help`" class="muted">{{ t('environments.urlsHelp') }}</small>
        </div>
        <div class="field full">
          <span :id="`${id}-tags-l`" class="label">{{ t('common.tags') }}</span>
          <TagsInput v-model="fm.tags" :aria-label="t('common.tags')" />
        </div>
      </div>

      <fieldset class="env-fieldset">
        <legend>{{ t('environments.components') }}</legend>
        <p class="small muted" style="margin: 0 0 6px">{{ t('environments.componentsHelp') }}</p>
        <div v-for="(c, i) in comps" :key="i" class="row nowrap" style="gap: 6px; margin-bottom: 4px">
          <InputText v-model="c.name" :aria-label="t('environments.componentName', { n: i + 1 })" :placeholder="t('environments.componentNamePlaceholder')" maxlength="100" style="flex: 1" />
          <InputText v-model="c.version" :aria-label="t('environments.componentVersion', { n: i + 1 })" :placeholder="t('environments.componentVersionPlaceholder')" maxlength="200" style="flex: 1" />
          <Button icon="pi pi-times" text rounded severity="secondary" :aria-label="t('environments.removeComponent', { n: i + 1 })" @click="comps.splice(i, 1)" />
        </div>
        <Button :label="t('environments.addComponent')" icon="pi pi-plus" text size="small" @click="comps.push({ name: '', version: '' })" />
        <Message v-if="dups.length" severity="warn" :closable="false">{{ t('environments.duplicateComponents', { names: dups.join(', ') }) }}</Message>
      </fieldset>

      <div v-if="!scopeShared && (cfg.configurations.value.length || vs.variants.value.length)" class="form-grid">
        <div v-if="cfg.configurations.value.length" class="field">
          <label :for="`${id}-cfgs`">{{ t('environments.configurations') }}</label>
          <MultiSelect v-model="fm.configuration_ids" :input-id="`${id}-cfgs`" :options="cfg.options.value" option-label="label" option-value="value" display="chip" filter />
        </div>
        <div v-if="vs.variants.value.length" class="field">
          <label :for="`${id}-vars`">{{ t('environments.variants') }}</label>
          <MultiSelect v-model="fm.variant_ids" :input-id="`${id}-vars`" :options="variantOpts" option-label="label" option-value="value" display="chip" filter />
        </div>
      </div>

      <fieldset class="env-fieldset">
        <legend>{{ t('environments.credentials') }}</legend>
        <div class="form-grid">
          <div class="field full">
            <label :for="`${id}-cref`">{{ t('environments.credentialsRef') }}</label>
            <InputText :id="`${id}-cref`" v-model="fm.credentials_ref" maxlength="2000" :placeholder="t('environments.credentialsRefPlaceholder')" :aria-describedby="`${id}-cref-help`" />
            <small :id="`${id}-cref-help`" class="muted">{{ t('environments.credentialsRefHelp') }}</small>
          </div>
          <div class="field">
            <label :for="`${id}-secret`">{{ t('environments.secret') }}</label>
            <InputText :id="`${id}-secret`" v-model="fm.secret" type="password" autocomplete="new-password" :placeholder="environment?.has_secret ? t('environments.secretKeep') : ''" :aria-describedby="`${id}-secret-help`" />
            <small :id="`${id}-secret-help`" class="muted">{{ t('environments.secretHelp') }}</small>
          </div>
          <div v-if="environment?.has_secret" class="field row" style="gap: 8px; align-self: center">
            <Checkbox v-model="fm.clear_secret" binary :input-id="`${id}-clear`" />
            <label :for="`${id}-clear`">{{ t('environments.clearSecret') }}</label>
          </div>
        </div>
      </fieldset>

      <fieldset class="env-fieldset">
        <legend>{{ t('environments.health') }}</legend>
        <div class="form-grid">
          <div class="field">
            <label :for="`${id}-hurl`">{{ t('environments.healthUrl') }}</label>
            <InputText :id="`${id}-hurl`" v-model="fm.health_url" type="url" maxlength="2000" placeholder="https://" :aria-describedby="`${id}-hurl-help`" />
            <small :id="`${id}-hurl-help`" class="muted">{{ t('environments.healthUrlHelp') }}</small>
          </div>
          <div class="field">
            <label :for="`${id}-hint`">{{ t('environments.healthInterval') }}</label>
            <InputNumber v-model="fm.health_interval_minutes" :input-id="`${id}-hint`" :min="1" :max="1440" suffix=" min" />
          </div>
        </div>
      </fieldset>
    </form>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button :label="editing ? t('common.save') : t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!fm.name.trim() || dups.length > 0" @click="save" />
    </template>
  </Dialog>
</template>

<style scoped>
.env-fieldset {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 8px 12px 10px;
  margin: 0;
}
.env-fieldset legend {
  font-weight: 600;
  padding: 0 4px;
}
.label {
  display: block;
  font-weight: 600;
  margin-bottom: 4px;
}
</style>
