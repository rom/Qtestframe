<script setup lang="ts">
// The logotype on the first page of reports and generated documents: the
// organisation's (Administration → Documents, settings.manage) or a
// project's own (project settings, project.update), its placement and
// maximum size, with a preview of the first page. PNG, JPEG or SVG; the
// server converts an SVG into an image with its own renderer (no scripts,
// links or embedded images).
import { computed, ref, watch } from 'vue'
import Button from 'primevue/button'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import SelectButton from 'primevue/selectbutton'
import ToggleSwitch from 'primevue/toggleswitch'
import { useI18n } from 'vue-i18n'
import { api, errorMessage } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { upload } from '@/lib/upload'
import {
  LOGO_ACCEPT,
  LOGO_MAX_HEIGHT_MM,
  LOGO_MAX_WIDTH_MM,
  LOGO_MIN_MM,
  LOGO_PLACEMENTS,
  logoProblem,
  logoSizeMM,
  type LogoPlacement,
  type LogoSettings,
  type ProjectLogo,
} from '@/lib/logo'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ projectId?: string; canEdit: boolean }>()
const { t } = useI18n()
const notify = useNotify()
const id = uid('logo')

const isProject = computed(() => !!props.projectId)
const base = computed(() => (props.projectId ? `/projects/${props.projectId}/logo` : '/settings/logo'))
const org = ref<LogoSettings | null>(null)
const own = ref<LogoSettings | null>(null)
const effective = ref<ProjectLogo['effective']>('none')
const loadError = ref('')
const busy = ref(false)
const problem = ref('')
const status = ref('')
const fileInput = ref<HTMLInputElement>()
const form = ref({ placement: 'left' as LogoPlacement, max_width_mm: 50, max_height_mm: 20 })

function fill(x: LogoSettings | ProjectLogo) {
  if ('effective' in x) {
    own.value = x.own
    org.value = x.organisation
    effective.value = x.effective
  } else {
    org.value = x
    own.value = null
    effective.value = x.file_id ? 'organisation' : 'none'
  }
  const s = settings.value
  if (s) form.value = { placement: s.placement, max_width_mm: s.max_width_mm, max_height_mm: s.max_height_mm }
}
async function load() {
  loadError.value = ''
  try {
    fill(await api.get<LogoSettings | ProjectLogo>(base.value))
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
watch(base, load, { immediate: true })

/** the settings the form edits: the organisation's, or the project's own */
const settings = computed(() => (isProject.value ? own.value : org.value))
/** the logo the documents show (with the form's placement and size where it edits it) */
const shown = computed<LogoSettings | null>(() => {
  if (effective.value === 'none') return null
  const s = effective.value === 'project' ? own.value : org.value
  if (!s?.file_id) return null
  return isProject.value && effective.value === 'organisation' ? s : { ...s, ...form.value }
})
const hasImage = computed(() => !!settings.value?.file_id)
const dirty = computed(() => {
  const s = settings.value
  return !!s && (s.placement !== form.value.placement || s.max_width_mm !== form.value.max_width_mm || s.max_height_mm !== form.value.max_height_mm)
})
const placementOptions = computed(() => LOGO_PLACEMENTS.map((p) => ({ value: p, label: t(`logo.placement.${p}`) })))
const effectiveText = computed(() => t(`logo.effective.${effective.value}`))

// preview: an A4 page at 0.9 px per millimetre
const SCALE = 0.9
const page = { w: 210 * SCALE, h: 297 * SCALE, margin: 18 * SCALE }
const box = computed(() => {
  const l = shown.value
  if (!l) return null
  const { w, h } = logoSizeMM(l)
  const usable = page.w - 2 * page.margin
  const pw = Math.min(w * SCALE, usable)
  const left = l.placement === 'center' ? page.margin + (usable - pw) / 2 : l.placement === 'right' ? page.margin + usable - pw : page.margin
  return { left, top: page.margin, width: pw, height: (h * SCALE * pw) / Math.max(w * SCALE, 0.001), w, h }
})
const sizeText = computed(() => (box.value ? t('logo.printedSize', { w: Math.round(box.value.w), h: Math.round(box.value.h) }) : ''))

async function onFile(e: Event) {
  const el = e.target as HTMLInputElement
  const f = el.files?.[0]
  el.value = ''
  if (!f) return
  problem.value = ''
  const p = logoProblem(f)
  if (p) {
    problem.value = t(`logo.problem.${p}`)
    return
  }
  busy.value = true
  try {
    fill(await upload<LogoSettings | ProjectLogo>(`${base.value}/image`, f, { method: 'PUT' }))
    status.value = t('logo.uploaded')
    notify.ok(t('logo.uploaded'))
  } catch (e) {
    problem.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
async function remove() {
  busy.value = true
  try {
    fill(await api.del<LogoSettings | ProjectLogo>(`${base.value}/image`))
    status.value = t('logo.removed')
    notify.ok(t('logo.removed'))
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}
async function save(body: Record<string, unknown> = form.value) {
  busy.value = true
  try {
    fill(await api.put<LogoSettings | ProjectLogo>(base.value, body))
    status.value = t('logo.saved')
    notify.ok(t('logo.saved'))
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="card logo-card" :aria-labelledby="`${id}-h`">
    <h2 :id="`${id}-h`" class="plain-heading" style="margin-bottom: 4px">{{ t('logo.title') }}</h2>
    <p class="small muted" style="margin: 0 0 12px">{{ isProject ? t('logo.introProject') : t('logo.introOrg') }}</p>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <div v-else class="logo-layout">
      <div class="grid" style="gap: 14px; min-width: 0">
        <p v-if="isProject" class="logo-effective" :id="`${id}-eff`">
          <i class="pi pi-info-circle" aria-hidden="true" /> {{ effectiveText }}
        </p>
        <div v-if="isProject" class="row nowrap" style="gap: 8px; align-items: flex-start">
          <ToggleSwitch
            :model-value="!!own?.off"
            :input-id="`${id}-off`"
            :disabled="!canEdit || busy"
            :aria-describedby="`${id}-off-hint`"
            @update:model-value="(v: boolean) => save({ off: v })"
          />
          <div>
            <label :for="`${id}-off`"><b>{{ t('logo.off') }}</b></label>
            <p :id="`${id}-off-hint`" class="small muted" style="margin: 2px 0 0">{{ t('logo.offHint') }}</p>
          </div>
        </div>

        <fieldset class="plain-fieldset" :disabled="!canEdit || busy">
          <legend class="field-legend">{{ isProject ? t('logo.ownImage') : t('logo.image') }}</legend>
          <p v-if="hasImage" class="small" style="margin: 0 0 6px">
            {{ t('logo.current', { name: settings?.name || '—', w: settings?.width ?? 0, h: settings?.height ?? 0 }) }}
            <template v-if="settings?.source === 'svg'"> · {{ t('logo.fromSvg') }}</template>
          </p>
          <p v-else class="small muted" style="margin: 0 0 6px">{{ isProject ? t('logo.noOwnImage') : t('logo.noImage') }}</p>
          <div v-if="canEdit" class="row" style="gap: 8px">
            <input ref="fileInput" type="file" :accept="LOGO_ACCEPT" class="sr-only" tabindex="-1" aria-hidden="true" @change="onFile" />
            <Button :label="hasImage ? t('logo.replace') : t('logo.upload')" icon="pi pi-upload" size="small" :loading="busy" :aria-describedby="`${id}-types`" @click="fileInput?.click()" />
            <Button v-if="hasImage" :label="t('logo.remove')" icon="pi pi-trash" size="small" severity="danger" outlined :disabled="busy" @click="remove" />
          </div>
          <p :id="`${id}-types`" class="small muted" style="margin: 6px 0 0">{{ t('logo.types') }}</p>
          <Message v-if="problem" severity="error" :closable="false" style="margin-top: 6px">{{ problem }}</Message>
        </fieldset>

        <form v-if="hasImage" class="grid" style="gap: 10px" @submit.prevent="save()">
          <div class="field">
            <span :id="`${id}-pl`" class="field-label">{{ t('logo.placementLabel') }}</span>
            <SelectButton
              v-model="form.placement"
              :options="placementOptions"
              option-label="label"
              option-value="value"
              :allow-empty="false"
              :disabled="!canEdit || busy"
              :aria-labelledby="`${id}-pl`"
            />
          </div>
          <div class="row" style="gap: 16px; align-items: flex-end">
            <div class="field">
              <label :for="`${id}-w`">{{ t('logo.maxWidth') }}</label>
              <InputNumber v-model="form.max_width_mm" :input-id="`${id}-w`" :min="LOGO_MIN_MM" :max="LOGO_MAX_WIDTH_MM" suffix=" mm" show-buttons :disabled="!canEdit || busy" style="width: 130px" />
            </div>
            <div class="field">
              <label :for="`${id}-h2`">{{ t('logo.maxHeight') }}</label>
              <InputNumber v-model="form.max_height_mm" :input-id="`${id}-h2`" :min="LOGO_MIN_MM" :max="LOGO_MAX_HEIGHT_MM" suffix=" mm" show-buttons :disabled="!canEdit || busy" style="width: 130px" />
            </div>
          </div>
          <p class="small muted" style="margin: 0">{{ t('logo.sizeHint') }}</p>
          <div v-if="canEdit" class="row" style="gap: 8px">
            <Button :label="t('common.save')" icon="pi pi-check" type="submit" :loading="busy" :disabled="!dirty" />
            <span v-if="dirty" class="small muted">{{ t('logo.unsaved') }}</span>
          </div>
        </form>
        <p class="sr-only" role="status" aria-live="polite">{{ status }}</p>
      </div>

      <figure class="logo-preview-wrap" :aria-labelledby="`${id}-cap`">
        <div class="logo-page" :style="{ width: `${page.w}px`, height: `${page.h}px` }">
          <img v-if="box && shown" :src="shown.url" :alt="t('logo.previewAlt')" class="logo-img" :style="{ left: `${box.left}px`, top: `${box.top}px`, width: `${box.width}px`, height: `${box.height}px` }" />
          <div class="logo-rule" :style="{ top: `${64 * SCALE}px`, left: `${page.margin}px`, right: `${page.margin}px` }" />
          <div class="logo-title" :style="{ top: `${70 * SCALE}px`, left: `${page.margin}px` }" aria-hidden="true">{{ t('logo.previewTitle') }}</div>
          <div v-for="i in 4" :key="i" class="logo-line" :style="{ top: `${(92 + i * 7) * SCALE}px`, left: `${page.margin}px`, width: `${(i % 2 ? 120 : 90) * SCALE}px` }" />
        </div>
        <figcaption :id="`${id}-cap`" class="small muted">
          {{ t('logo.previewCaption') }}<template v-if="sizeText"> · {{ sizeText }}</template>
          <template v-if="!shown"> · {{ t('logo.previewNone') }}</template>
        </figcaption>
      </figure>
    </div>
  </section>
</template>

<style scoped>
.logo-card {
  max-width: 860px;
  margin-top: 16px;
}
.logo-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 24px;
  align-items: start;
}
@media (max-width: 720px) {
  .logo-layout {
    grid-template-columns: minmax(0, 1fr);
  }
}
.logo-effective {
  margin: 0;
  font-weight: 600;
}
.logo-preview-wrap {
  margin: 0;
  display: grid;
  gap: 6px;
  justify-items: center;
}
.logo-page {
  position: relative;
  background: #fff;
  border: 1px solid var(--qtf-border);
  box-shadow: 0 1px 4px rgb(0 0 0 / 0.15);
  overflow: hidden;
}
.logo-img {
  position: absolute;
  object-fit: contain;
}
.logo-rule {
  position: absolute;
  height: 1px;
  background: #2f5496;
}
.logo-title {
  position: absolute;
  font-weight: 700;
  font-size: 11px;
  color: #1f3864;
}
.logo-line {
  position: absolute;
  height: 3px;
  border-radius: 2px;
  background: #d0d0d0;
}
.logo-preview-wrap figcaption {
  max-width: 220px;
  text-align: center;
}
</style>
