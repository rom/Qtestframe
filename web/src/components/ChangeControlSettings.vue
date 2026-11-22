<script setup lang="ts">
// The project's change control: off (items are edited directly), warn or
// enforce (protected items change only through approved change requests),
// which statuses are protected, whether signed baselines lock their items,
// the change control board and its quorum, the signature meaning and
// whether closing a change request creates a baseline.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import MultiSelect from 'primevue/multiselect'
import SelectButton from 'primevue/selectbutton'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { api, asItems } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { formatDateTime } from '@/lib/format'
import { CHANGE_CONTROL_MODES, parseEmails, type ChangeControlSettings } from '@/lib/changeRequests'
import { enumLabel } from '@/i18n/labels'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string; canEdit: boolean }>()
const { t } = useI18n()
const notify = useNotify()
const meta = useMetaStore()
const id = uid('ccset')

interface Answer {
  settings: ChangeControlSettings
  effective_statuses: string[]
  default_statuses: string[]
  default_signature_meaning: string
}
const answer = ref<Answer | null>(null)
const mode = ref('off')
const statuses = ref<string[]>([])
const lock = ref(true)
const users = ref('')
const roles = ref<string[]>([])
const quorum = ref('all')
const meaning = ref('')
const baseline = ref(false)
const busy = ref(false)
const roleNames = ref<string[]>([])

function fill(a: Answer) {
  answer.value = a
  const s = a.settings
  mode.value = s.mode
  statuses.value = [...s.protected_statuses]
  lock.value = s.lock_signed_baselines
  users.value = s.board.users.join(', ')
  roles.value = [...s.board.roles]
  quorum.value = s.board.quorum
  meaning.value = s.signature_meaning
  baseline.value = s.baseline_on_close
}
watch(
  () => props.projectId,
  async (pid) => {
    try {
      fill(await api.get<Answer>(`/projects/${pid}/change-control`))
    } catch {
      answer.value = null
    }
    try {
      roleNames.value = asItems(await api.get<{ items: { name: string }[] }>('/roles')).map((r) => r.name)
    } catch {
      roleNames.value = []
    }
  },
  { immediate: true },
)
const modeOptions = computed(() => CHANGE_CONTROL_MODES.map((m) => ({ label: t(`changeRequests.settings.mode.${m}`), value: m })))
const statusOptions = computed(() =>
  [...new Set([...meta.meta.requirement_statuses, ...meta.meta.test_statuses])].map((s) => ({ label: enumLabel('status', s), value: s })),
)
const roleOptions = computed(() => roleNames.value.map((r) => ({ label: enumLabel('role', r), value: r })))
const statusPlaceholder = computed(() => t('changeRequests.settings.defaultStatuses', { list: (answer.value?.default_statuses ?? []).map((s) => enumLabel('status', s)).join(', ') }))

async function save() {
  busy.value = true
  try {
    fill(
      await api.put<Answer>(`/projects/${props.projectId}/change-control`, {
        mode: mode.value,
        protected_statuses: statuses.value,
        lock_signed_baselines: lock.value,
        board: { users: parseEmails(users.value), roles: roles.value, quorum: quorum.value.trim() || 'all' },
        signature_meaning: meaning.value,
        baseline_on_close: baseline.value,
      }),
    )
    notify.ok(t('changeRequests.settings.saved'))
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section v-if="answer" class="card" style="max-width: 760px; margin-top: 16px" :aria-labelledby="`${id}-h`">
    <h2 :id="`${id}-h`" class="plain-heading" style="margin-bottom: 6px">{{ t('changeRequests.settings.title') }}</h2>
    <p class="small muted" style="margin-top: 0">{{ t('changeRequests.settings.intro') }}</p>
    <form class="grid" style="gap: 10px" @submit.prevent="save">
      <span :id="`${id}-mode`" class="field-label">{{ t('changeRequests.settings.modeLabel') }}</span>
      <SelectButton v-model="mode" :options="modeOptions" option-label="label" option-value="value" :allow-empty="false" :disabled="!canEdit" :aria-labelledby="`${id}-mode`" :aria-describedby="`${id}-mode-hint`" />
      <small :id="`${id}-mode-hint`" class="muted">{{ t(`changeRequests.settings.modeHint.${mode}`) }}</small>
      <label :for="`${id}-statuses`">{{ t('changeRequests.settings.statuses') }}</label>
      <MultiSelect v-model="statuses" :input-id="`${id}-statuses`" :options="statusOptions" option-label="label" option-value="value" :placeholder="statusPlaceholder" :disabled="!canEdit" display="chip" />
      <div class="row nowrap" style="gap: 8px; align-items: flex-start">
        <ToggleSwitch v-model="lock" :input-id="`${id}-lock`" :disabled="!canEdit" :aria-describedby="`${id}-lock-hint`" />
        <div>
          <label :for="`${id}-lock`"><b>{{ t('changeRequests.settings.lock') }}</b></label>
          <p :id="`${id}-lock-hint`" class="small muted" style="margin: 2px 0 0">{{ t('changeRequests.settings.lockHint') }}</p>
        </div>
      </div>
      <h3 class="plain-heading small" style="margin: 6px 0 0">{{ t('changeRequests.settings.board') }}</h3>
      <p class="small muted" style="margin: 0">{{ t('changeRequests.settings.boardHint') }}</p>
      <label :for="`${id}-users`">{{ t('changeRequests.settings.users') }}</label>
      <InputText :id="`${id}-users`" v-model="users" :disabled="!canEdit" :placeholder="t('changeRequests.settings.usersPlaceholder')" />
      <label :for="`${id}-roles`">{{ t('changeRequests.settings.roles') }}</label>
      <MultiSelect v-model="roles" :input-id="`${id}-roles`" :options="roleOptions" option-label="label" option-value="value" :disabled="!canEdit" display="chip" :placeholder="t('changeRequests.settings.noRoles')" />
      <label :for="`${id}-quorum`">{{ t('changeRequests.settings.quorum') }}</label>
      <InputText :id="`${id}-quorum`" v-model="quorum" :disabled="!canEdit" :aria-describedby="`${id}-quorum-hint`" style="max-width: 160px" />
      <small :id="`${id}-quorum-hint`" class="muted">{{ t('changeRequests.settings.quorumHint') }}</small>
      <label :for="`${id}-meaning`">{{ t('changeRequests.settings.meaning') }}</label>
      <Textarea :id="`${id}-meaning`" v-model="meaning" rows="2" auto-resize :disabled="!canEdit" :placeholder="answer.default_signature_meaning" />
      <div class="row nowrap" style="gap: 8px; align-items: flex-start">
        <ToggleSwitch v-model="baseline" :input-id="`${id}-baseline`" :disabled="!canEdit" />
        <label :for="`${id}-baseline`"><b>{{ t('changeRequests.settings.baselineOnClose') }}</b></label>
      </div>
      <p v-if="answer.settings.updated_by" class="small muted" style="margin: 0">{{ t('verification.settingsChanged', { user: answer.settings.updated_by, date: formatDateTime(answer.settings.updated_at ?? '') }) }}</p>
      <div v-if="canEdit" class="row" style="justify-content: flex-end">
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" />
      </div>
    </form>
  </section>
</template>
