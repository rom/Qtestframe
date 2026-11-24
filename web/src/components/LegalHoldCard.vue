<script setup lang="ts">
// The legal hold of a project and of its releases (project settings):
// while a hold is on, the retention job deletes nothing of it and it
// cannot be deleted. Organisation and super administrators set and lift
// holds, with a reason (GET/PUT /projects/{pid}/legal-hold and
// /projects/{pid}/releases/{rid}/legal-hold).
import { ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Tag from 'primevue/tag'
import { useI18n } from 'vue-i18n'
import { api } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { formatDateTime } from '@/lib/format'
import type { LegalHold, LegalHoldView } from '@/lib/privacy'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId: string }>()
const { t } = useI18n()
const notify = useNotify()
const id = uid('hold')
const view = ref<LegalHoldView | null>(null)
const announce = ref('')

watch(
  () => props.projectId,
  async (pid) => {
    try {
      view.value = await api.get<LegalHoldView>(`/projects/${pid}/legal-hold`)
    } catch {
      view.value = null
    }
  },
  { immediate: true },
)

// set: a dialog asks for the reason; lift: a confirmation
const dialog = ref(false)
const target = ref<{ releaseId: string; name: string } | null>(null)
const reason = ref('')
const busy = ref(false)

function path(releaseId: string) {
  return releaseId ? `/projects/${props.projectId}/releases/${releaseId}/legal-hold` : `/projects/${props.projectId}/legal-hold`
}

async function send(releaseId: string, h: LegalHold, name: string) {
  busy.value = true
  try {
    view.value = await api.put<LegalHoldView>(path(releaseId), h)
    const msg = h.on ? t('privacy.hold.setDone', { name }) : t('privacy.hold.liftDone', { name })
    notify.ok(msg)
    announce.value = msg
    dialog.value = false
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}

function openSet(releaseId: string, name: string) {
  target.value = { releaseId, name }
  reason.value = ''
  dialog.value = true
}

function lift(releaseId: string, name: string) {
  notify.confirmDanger(t('privacy.hold.confirmLift', { name }), () => send(releaseId, { on: false, reason: '' }, name), t('privacy.hold.lift'), t('privacy.hold.lift'))
}
</script>

<template>
  <section v-if="view" class="card" style="max-width: 640px; margin-top: 16px" :aria-labelledby="`${id}-h`">
    <h2 :id="`${id}-h`" class="plain-heading" style="margin-bottom: 6px"><i class="pi pi-lock" aria-hidden="true" /> {{ t('privacy.hold.title') }}</h2>
    <p class="small muted" style="margin: 0 0 8px">{{ t('privacy.hold.intro') }}</p>
    <div class="row hold-row">
      <div class="grow">
        <b>{{ t('privacy.hold.project') }}</b>
        <Tag v-if="view.project.on" :value="t('privacy.hold.on')" severity="warn" icon="pi pi-lock" class="hold-tag" />
        <div v-if="view.project.on" class="small">{{ view.project.reason }} <span class="muted">— {{ t('privacy.hold.setBy', { who: view.project.by, date: formatDateTime(view.project.at) }) }}</span></div>
      </div>
      <template v-if="view.can_edit">
        <Button v-if="view.project.on" :label="t('privacy.hold.lift')" icon="pi pi-lock-open" severity="secondary" outlined size="small" :loading="busy" @click="lift('', t('privacy.hold.project'))" />
        <Button v-else :label="t('privacy.hold.set')" icon="pi pi-lock" severity="warn" outlined size="small" @click="openSet('', t('privacy.hold.project'))" />
      </template>
    </div>
    <h3 class="plain-heading small" style="margin: 12px 0 4px">{{ t('privacy.hold.releases') }}</h3>
    <ul v-if="view.releases.length" class="plain-list">
      <li v-for="r in view.releases" :key="r.id" class="row hold-row">
        <div class="grow">
          <span>{{ r.name }}</span> <span v-if="r.version" class="small muted">{{ r.version }}</span>
          <Tag v-if="r.hold.on" :value="t('privacy.hold.on')" severity="warn" icon="pi pi-lock" class="hold-tag" />
          <div v-if="r.hold.on" class="small">{{ r.hold.reason }} <span class="muted">— {{ t('privacy.hold.setBy', { who: r.hold.by, date: formatDateTime(r.hold.at) }) }}</span></div>
        </div>
        <template v-if="view.can_edit">
          <Button v-if="r.hold.on" icon="pi pi-lock-open" text rounded severity="secondary" :aria-label="t('privacy.hold.liftNamed', { name: r.name })" v-tooltip="t('privacy.hold.lift')" @click="lift(r.id, r.name)" />
          <Button v-else icon="pi pi-lock" text rounded severity="warn" :aria-label="t('privacy.hold.setNamed', { name: r.name })" v-tooltip="t('privacy.hold.set')" @click="openSet(r.id, r.name)" />
        </template>
      </li>
    </ul>
    <p v-else class="small muted">{{ t('privacy.hold.noReleases') }}</p>
    <p v-if="!view.can_edit" class="small muted"><i class="pi pi-info-circle" aria-hidden="true" /> {{ t('privacy.hold.whoCan') }}</p>
    <div class="sr-only" role="status" aria-live="polite">{{ announce }}</div>

    <Dialog v-model:visible="dialog" :header="t('privacy.hold.setTitle', { name: target?.name ?? '' })" modal style="width: min(480px, 95vw)">
      <form class="grid" style="gap: 12px" @submit.prevent="target && reason.trim() && send(target.releaseId, { on: true, reason: reason.trim() }, target.name)">
        <p class="small">{{ t('privacy.hold.setText') }}</p>
        <div v-field class="field">
          <label>{{ t('privacy.hold.reason') }}</label>
          <InputText v-model="reason" maxlength="500" required />
        </div>
        <div class="row" style="justify-content: flex-end; gap: 8px">
          <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
          <Button type="submit" :label="t('privacy.hold.set')" icon="pi pi-lock" severity="warn" :loading="busy" :disabled="!reason.trim()" />
        </div>
      </form>
    </Dialog>
  </section>
</template>

<style scoped>
.hold-row {
  gap: 8px;
  align-items: flex-start;
  padding: 4px 0;
}
.hold-tag {
  margin-left: 6px;
}
</style>
