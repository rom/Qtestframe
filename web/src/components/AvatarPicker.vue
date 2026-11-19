<script setup lang="ts">
// Avatar / project icon picker: preset gallery, initials, or an uploaded
// image cropped and scaled client-side to 256×256 (webp or png, < 1 MB).
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import Slider from 'primevue/slider'
import Message from 'primevue/message'
import UserAvatar from './UserAvatar.vue'
import ProjectIcon from './ProjectIcon.vue'
import { useI18n } from 'vue-i18n'
import { AVATAR_PRESETS, ICON_PRESETS } from '@/assets/presets'
import { parseImageRef, presetValue } from '@/lib/avatars'
import { IMAGE_ACCEPT, IMAGE_TYPES, uploadProblem } from '@/lib/files'
import { DEFAULT_CROP, MAX_ZOOM, MIN_ZOOM, imageFileName, type Crop } from '@/lib/image'
import { upload } from '@/lib/upload'
import { errorMessage } from '@/lib/api'
import { drawCrop, encodeAvatar, loadImage } from '@/composables/avatarImage'
import { uid } from '@/lib/a11y'

const props = withDefaults(
  defineProps<{
    kind: 'avatar' | 'icon'
    value: string | null | undefined
    /** display name (avatar) or project key (icon) for the initials */
    name: string
    /** PUT target of an uploaded image */
    uploadPath: string
    /** store a preset or "" (initials) */
    save: (value: string) => Promise<void>
    disabled?: boolean
  }>(),
  { disabled: false },
)
const emit = defineEmits<{ uploaded: [response: unknown] }>()
const { t } = useI18n()
const id = uid('picker')
const presets = computed(() => (props.kind === 'avatar' ? AVATAR_PRESETS : ICON_PRESETS))
const presetNames = computed(() => Object.keys(presets.value).sort())
const current = computed(() => {
  const r = parseImageRef(props.value)
  return r.kind === 'preset' ? r.name : r.kind === 'file' ? '__file' : ''
})
const choice = ref(current.value)
watch(current, (v) => (choice.value = v))
const busy = ref(false)
const status = ref('')
const error = ref('')

const label = (n: string) => t(`avatar.presets.${n.replace(/-/g, '_')}`)

async function apply(v: string) {
  busy.value = true
  error.value = ''
  try {
    await props.save(v)
    status.value = v ? t('avatar.saved') : t('avatar.reset')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
function useChoice() {
  if (choice.value && choice.value !== '__file') apply(presetValue(choice.value))
}

// ---------- upload + crop ----------
const fileInput = ref<HTMLInputElement>()
const cropOpen = ref(false)
const crop = ref<Crop>({ ...DEFAULT_CROP })
const canvas = ref<HTMLCanvasElement>()
let loaded: { img: HTMLImageElement; release: () => void } | null = null
let baseName = ''

async function onFile(e: Event) {
  const el = e.target as HTMLInputElement
  const f = el.files?.[0]
  el.value = ''
  if (!f) return
  error.value = ''
  if (uploadProblem(f, 0, IMAGE_TYPES)) {
    error.value = t('avatar.badType')
    return
  }
  try {
    loaded?.release()
    loaded = await loadImage(f)
    baseName = f.name
    crop.value = { ...DEFAULT_CROP }
    cropOpen.value = true
    await nextTick()
    redraw()
  } catch {
    error.value = t('avatar.unreadable')
  }
}
function redraw() {
  if (canvas.value && loaded) drawCrop(canvas.value, loaded.img, crop.value)
}
watch(crop, redraw, { deep: true })
watch(cropOpen, async (v) => {
  if (v) {
    await nextTick()
    redraw()
  }
})
onBeforeUnmount(() => loaded?.release())

async function uploadCrop() {
  if (!loaded) return
  busy.value = true
  error.value = ''
  try {
    const blob = await encodeAvatar(loaded.img, crop.value)
    if (!blob) {
      error.value = t('avatar.tooLarge')
      return
    }
    const file = new File([blob], imageFileName(baseName, blob.type), { type: blob.type })
    const res = await upload(props.uploadPath, file, { method: 'PUT' })
    emit('uploaded', res)
    cropOpen.value = false
    status.value = t('avatar.uploaded')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="avatar-picker">
    <div class="row" style="gap: 14px; align-items: center">
      <UserAvatar v-if="kind === 'avatar'" :avatar="value ?? ''" :name="name" :size="72" />
      <ProjectIcon v-else :icon="value ?? ''" :project-key="name" :size="72" />
      <div class="grid" style="gap: 6px">
        <div class="row" style="gap: 6px">
          <Button :label="t('avatar.upload')" icon="pi pi-upload" size="small" :disabled="disabled || busy" @click="fileInput?.click()" />
          <Button :label="kind === 'avatar' ? t('avatar.useInitials') : t('avatar.useKey')" icon="pi pi-refresh" size="small" severity="secondary" outlined :disabled="disabled || busy || !value" @click="apply('')" />
        </div>
        <span class="small muted">{{ t('avatar.uploadHint') }}</span>
      </div>
      <input ref="fileInput" type="file" :accept="IMAGE_ACCEPT" style="display: none" @change="onFile" />
    </div>
    <Message v-if="error" severity="error" :closable="false" style="margin-top: 10px">{{ error }}</Message>
    <div class="sr-only" role="status" aria-live="polite">{{ status }}</div>

    <fieldset class="plain-fieldset" :disabled="disabled || busy">
      <legend class="section-title">{{ kind === 'avatar' ? t('avatar.gallery') : t('avatar.iconGallery') }}</legend>
      <div class="gallery">
        <label v-for="n in presetNames" :key="n" class="gallery-item" :class="{ checked: choice === n }" :title="label(n)">
          <input v-model="choice" type="radio" class="sr-only" :name="id" :value="n" />
          <img :src="presets[n]" alt="" width="44" height="44" :style="{ borderRadius: kind === 'avatar' ? '50%' : '11px' }" />
          <span class="sr-only">{{ label(n) }}</span>
        </label>
      </div>
      <div class="row" style="margin-top: 8px">
        <Button :label="t('avatar.usePreset')" icon="pi pi-check" size="small" :disabled="!choice || choice === '__file' || choice === current" :loading="busy" @click="useChoice" />
      </div>
    </fieldset>

    <Dialog v-model:visible="cropOpen" modal :header="t('avatar.cropTitle')" :style="{ width: 'min(440px, calc(100vw - 16px))' }">
      <div class="grid" style="gap: 12px; justify-items: center">
        <canvas ref="canvas" class="crop-preview" :class="{ round: kind === 'avatar' }" role="img" :aria-label="t('avatar.preview')" />
        <div class="grid w-full" style="gap: 14px">
          <div class="field">
            <span :id="`${id}-zoom`" class="field-label">{{ t('avatar.zoom') }}</span>
            <Slider v-model="crop.zoom" :min="MIN_ZOOM" :max="MAX_ZOOM" :step="0.05" :aria-labelledby="`${id}-zoom`" />
          </div>
          <div class="field">
            <span :id="`${id}-x`" class="field-label">{{ t('avatar.horizontal') }}</span>
            <Slider v-model="crop.x" :min="-1" :max="1" :step="0.02" :aria-labelledby="`${id}-x`" />
          </div>
          <div class="field">
            <span :id="`${id}-y`" class="field-label">{{ t('avatar.vertical') }}</span>
            <Slider v-model="crop.y" :min="-1" :max="1" :step="0.02" :aria-labelledby="`${id}-y`" />
          </div>
        </div>
        <span class="small muted">{{ t('avatar.cropHint') }}</span>
      </div>
      <template #footer>
        <Button :label="t('common.cancel')" text severity="secondary" @click="cropOpen = false" />
        <Button :label="t('avatar.saveImage')" icon="pi pi-check" :loading="busy" @click="uploadCrop" />
      </template>
    </Dialog>
  </div>
</template>

<style scoped>
.gallery {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(52px, 1fr));
  gap: 6px;
}
.gallery-item {
  display: grid;
  place-items: center;
  padding: 3px;
  border-radius: 12px;
  border: 2px solid transparent;
  cursor: pointer;
}
.gallery-item:hover {
  background: var(--qtf-hover);
}
.gallery-item.checked {
  border-color: var(--p-primary-color);
}
.gallery-item:has(input:focus-visible) {
  outline: 2px solid var(--qtf-focus);
  outline-offset: 2px;
}
.crop-preview {
  width: 160px;
  height: 160px;
  border: 1px solid var(--qtf-border);
  border-radius: 12px;
  background: var(--qtf-card);
}
.crop-preview.round {
  border-radius: 50%;
}
</style>
