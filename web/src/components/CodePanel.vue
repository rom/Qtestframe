<script setup lang="ts">
// "Code" tab of a requirement or test: the commits and merge / pull requests
// that refer to it (from the GitLab / GitHub webhooks, or linked by hand), the
// builds — for a requirement the builds its tests ran in and the latest build
// that verified it, for a test its build history.
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Select from 'primevue/select'
import SelectButton from 'primevue/selectbutton'
import Tag from 'primevue/tag'
import StatusTag from './StatusTag.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { buildLabel, changeRef, type BuildCoverage, type CodeChange, type ItemCode, type TestBuild } from '@/lib/variants'
import { uid } from '@/lib/a11y'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ itemType: 'requirement' | 'test'; itemId: string; projectId: string; canWrite: boolean }>()
const emit = defineEmits<{ count: [n: number] }>()
const { t } = useI18n()
const notify = useNotify()
const id = uid('code')

const data = ref<ItemCode | null>(null)
const error = ref('')
const loading = ref(false)
const base = computed(() => `/${props.itemType === 'test' ? 'tests' : 'requirements'}/${props.itemId}/code`)

async function load() {
  loading.value = true
  error.value = ''
  try {
    data.value = await api.get<ItemCode>(base.value)
    emit('count', data.value.changes.length)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.itemId, load, { immediate: true })

const changes = computed(() => data.value?.changes ?? [])
const reqBuilds = computed(() => (props.itemType === 'requirement' ? ((data.value?.builds ?? []) as BuildCoverage[]) : []))
const testBuilds = computed(() => (props.itemType === 'test' ? ((data.value?.builds ?? []) as TestBuild[]) : []))
const statusSeverity = (s: string) => (s === 'merged' ? 'success' : s === 'closed' ? 'secondary' : 'info')
const kindIcon = (c: CodeChange) => (c.kind === 'commit' ? 'pi pi-code' : 'pi pi-arrow-right-arrow-left')

// ---------- add / remove ----------
const dialog = ref(false)
const form = ref({ kind: 'commit' as 'commit' | 'merge_request', ref: '', url: '', title: '', repository: '', status: 'open' })
const formError = ref('')
const busy = ref(false)
const kindOptions = computed(() => [
  { label: t('code.kind.commit'), value: 'commit' },
  { label: t('code.kind.merge_request'), value: 'merge_request' },
])
const statusOptions = computed(() => ['open', 'merged', 'closed'].map((s) => ({ label: t(`code.status.${s}`), value: s })))
function openAdd() {
  form.value = { kind: 'commit', ref: '', url: '', title: '', repository: '', status: 'open' }
  formError.value = ''
  dialog.value = true
}
async function add() {
  if (!form.value.ref.trim() && !form.value.url.trim()) {
    formError.value = t('code.refRequired')
    return
  }
  busy.value = true
  formError.value = ''
  try {
    await api.post(base.value, { ...form.value, status: form.value.kind === 'commit' ? '' : form.value.status })
    dialog.value = false
    notify.ok(t('code.linked'))
    await load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
async function remove(c: CodeChange) {
  try {
    await api.del(`${base.value}/${c.id}`)
    notify.ok(t('code.unlinked'))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
const sourceOf = (c: CodeChange) => c.items.find((i) => i.item_id === props.itemId)?.source ?? 'auto'
</script>

<template>
  <section class="code-panel" :aria-label="t('code.tab')">
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>

    <div v-if="itemType === 'requirement' && data" class="verified row" role="status">
      <template v-if="data.verified_build">
        <i class="pi pi-verified" aria-hidden="true" style="color: var(--p-green-500)" />
        <span>{{ t('code.verifiedIn') }}</span>
        <a v-if="data.verified_build.pipeline_url" :href="data.verified_build.pipeline_url" target="_blank" rel="noopener noreferrer" class="mono"
          >{{ buildLabel(data.verified_build) }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a
        >
        <b v-else class="mono">{{ buildLabel(data.verified_build) }}</b>
        <span class="small muted">{{ formatDateTime(data.verified_build.created_at) }}</span>
      </template>
      <span v-else class="muted small"><i class="pi pi-info-circle" aria-hidden="true" /> {{ t('code.notVerified') }}</span>
    </div>

    <div class="row" style="justify-content: space-between; margin: 8px 0 6px">
      <h3 class="plain-heading small-heading">{{ t('code.changes') }} <span v-if="changes.length" class="count">{{ changes.length }}</span></h3>
      <span class="row nowrap">
        <Button v-if="canWrite" :label="t('code.add')" icon="pi pi-plus" size="small" text aria-haspopup="dialog" @click="openAdd" />
        <HelpLink page="variants-and-code" :anchor="{ en: 'code-links', sv: 'kodlänkar' }" />
      </span>
    </div>
    <p v-if="!loading && !changes.length" class="small muted" style="margin: 0">{{ t('code.none') }}</p>
    <ul v-if="changes.length" class="plain-list change-list">
      <li v-for="c in changes" :key="c.id" class="change">
        <div class="row nowrap">
          <i :class="kindIcon(c)" aria-hidden="true" class="muted" />
          <span class="sr-only">{{ t(`code.kind.${c.kind}`) }}:</span>
          <a v-if="c.url" :href="c.url" target="_blank" rel="noopener noreferrer" class="mono"
            >{{ changeRef(c) }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a
          >
          <span v-else class="mono">{{ changeRef(c) }}</span>
          <span class="grow ellipsis" :title="c.title">{{ c.title }}</span>
          <Tag v-if="c.kind === 'merge_request'" :value="t(`code.status.${c.status || 'open'}`)" :severity="statusSeverity(c.status)" />
          <Tag v-if="sourceOf(c) === 'manual'" :value="t('code.manual')" severity="secondary" />
          <Button v-if="canWrite" icon="pi pi-times" text rounded size="small" severity="secondary" :aria-label="t('code.remove', { ref: changeRef(c) })" v-tooltip="t('code.removeHint')" @click="remove(c)" />
        </div>
        <div class="small muted meta">
          <span v-if="c.repository">{{ c.repository }}</span>
          <span v-if="c.branch"><i class="pi pi-share-alt" aria-hidden="true" /> {{ c.branch }}<template v-if="c.target_branch"> → {{ c.target_branch }}</template></span>
          <span v-if="c.author">{{ c.author }}</span>
          <span>{{ formatDateTime(c.occurred_at) }}</span>
          <span>{{ t(`code.provider.${c.provider}`) }}</span>
          <span v-for="b in c.builds ?? []" :key="b.id"><i class="pi pi-box" aria-hidden="true" /> {{ t('code.buildN', { n: buildLabel(b) }) }}</span>
        </div>
      </li>
    </ul>

    <template v-if="reqBuilds.length">
      <h3 class="plain-heading small-heading" style="margin-top: 14px">{{ t('code.builds') }}</h3>
      <div class="table-scroll">
        <table class="code-table">
          <caption class="sr-only">{{ t('code.buildsCaptionRequirement') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('code.build') }}</th>
              <th scope="col">{{ t('code.branch') }}</th>
              <th scope="col">{{ t('code.commit') }}</th>
              <th scope="col">{{ t('common.created') }}</th>
              <th scope="col">{{ t('traceability.coverage') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="b in reqBuilds" :key="b.build.id">
              <th scope="row" class="mono">
                <a v-if="b.build.pipeline_url" :href="b.build.pipeline_url" target="_blank" rel="noopener noreferrer">{{ buildLabel(b.build) }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
                <template v-else>{{ buildLabel(b.build) }}</template>
              </th>
              <td>{{ b.build.branch || '—' }}</td>
              <td class="mono">{{ b.build.commit_sha ? b.build.commit_sha.slice(0, 8) : '—' }}</td>
              <td>{{ formatDateTime(b.build.created_at) }}</td>
              <td><StatusTag :value="b.coverage" kind="coverage" /></td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <template v-if="itemType === 'test'">
      <h3 class="plain-heading small-heading" style="margin-top: 14px">{{ t('code.buildHistory') }}</h3>
      <p v-if="!testBuilds.length" class="small muted" style="margin: 0">{{ t('code.noBuilds') }}</p>
      <div v-else class="table-scroll">
        <table class="code-table">
          <caption class="sr-only">{{ t('code.buildsCaptionTest') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('code.build') }}</th>
              <th scope="col">{{ t('code.branch') }}</th>
              <th scope="col">{{ t('code.commit') }}</th>
              <th scope="col">{{ t('common.created') }}</th>
              <th scope="col">{{ t('code.result') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="b in testBuilds" :key="b.build.id">
              <th scope="row" class="mono">
                <a v-if="b.build.pipeline_url" :href="b.build.pipeline_url" target="_blank" rel="noopener noreferrer">{{ buildLabel(b.build) }}<span class="sr-only"> ({{ t('links.newTab') }})</span></a>
                <template v-else>{{ buildLabel(b.build) }}</template>
              </th>
              <td>{{ b.build.branch || '—' }}</td>
              <td class="mono">{{ b.build.commit_sha ? b.build.commit_sha.slice(0, 8) : '—' }}</td>
              <td>{{ formatDateTime(b.build.created_at) }}</td>
              <td><StatusTag :value="b.result" kind="result" /></td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <Dialog v-model:visible="dialog" modal :header="t('code.addTitle')" style="width: min(560px, 95vw)">
      <form class="grid" style="gap: 12px" @submit.prevent="add">
        <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
        <div class="field">
          <span :id="`${id}-kind`" class="field-label">{{ t('code.kindLabel') }}</span>
          <SelectButton v-model="form.kind" :options="kindOptions" option-label="label" option-value="value" :allow-empty="false" :aria-labelledby="`${id}-kind`" />
        </div>
        <div class="field">
          <label :for="`${id}-ref`">{{ form.kind === 'commit' ? t('code.sha') : t('code.number') }}</label>
          <InputText :id="`${id}-ref`" v-model="form.ref" class="mono" maxlength="100" autocomplete="off" autofocus />
        </div>
        <div class="field">
          <label :for="`${id}-url`">{{ t('links.url') }}</label>
          <InputText :id="`${id}-url`" v-model="form.url" type="url" inputmode="url" placeholder="https://" maxlength="2000" />
        </div>
        <div class="field">
          <label :for="`${id}-title`">{{ t('common.title') }}</label>
          <InputText :id="`${id}-title`" v-model="form.title" maxlength="500" />
        </div>
        <div class="field">
          <label :for="`${id}-repo`">{{ t('code.repository') }}</label>
          <InputText :id="`${id}-repo`" v-model="form.repository" maxlength="250" placeholder="group/project" />
        </div>
        <div v-if="form.kind === 'merge_request'" class="field">
          <label :id="`${id}-status-label`" :for="`${id}-status`">{{ t('common.status') }}</label>
          <Select v-model="form.status" :input-id="`${id}-status`" :aria-labelledby="`${id}-status-label`" :options="statusOptions" option-label="label" option-value="value" />
        </div>
        <div class="row" style="justify-content: flex-end">
          <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="dialog = false" />
          <Button type="submit" :label="t('code.add')" icon="pi pi-link" :loading="busy" />
        </div>
      </form>
    </Dialog>
  </section>
</template>

<style scoped>
.verified {
  gap: 6px;
  padding: 8px 10px;
  border: 1px solid var(--qtf-border);
  border-radius: 6px;
}
.change {
  padding: 6px 0;
  border-bottom: 1px solid var(--qtf-border);
}
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-top: 2px;
  padding-inline-start: 22px;
}
.table-scroll {
  overflow-x: auto;
}
.code-table {
  border-collapse: collapse;
  width: 100%;
}
.code-table th,
.code-table td {
  text-align: left;
  padding: 4px 8px;
  border-bottom: 1px solid var(--qtf-border);
  white-space: nowrap;
}
.code-table thead th {
  font-size: 12px;
  font-weight: 600;
}
</style>
