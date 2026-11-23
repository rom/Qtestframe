<script setup lang="ts">
// Settings of a dashboard widget: its title, size and the settings of its
// type (lib/widgets.ts settingFields), e.g. the release, the query or the
// chart type. Releases, cycles, variants and projects are offered from
// the widget's project.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Select from 'primevue/select'
import Textarea from 'primevue/textarea'
import { api, asItems } from '@/lib/api'
import { GRID_COLS, MAX_H } from '@/lib/dashboardGrid'
import { cleanSettings, settingFields, setting, type DashboardWidget, type SettingField } from '@/lib/widgets'

const props = defineProps<{ widget: DashboardWidget | null; projectId: string; home: boolean; defaultTitle: (w: DashboardWidget) => string }>()
const emit = defineEmits<{ save: [w: DashboardWidget]; close: [] }>()
const { t, te } = useI18n()

const visible = computed({ get: () => !!props.widget, set: (v) => !v && emit('close') })
const title = ref('')
const w = ref(6)
const h = ref(4)
const values = ref<Record<string, unknown>>({})
const fields = computed<SettingField[]>(() => (props.widget ? settingFields(props.widget.type, props.home) : []))

type Opt = { value: string; label: string }
const lists = ref<Record<string, Opt[]>>({})
const project = computed(() => props.projectId || String(values.value.project_id ?? ''))

async function loadList(kind: string) {
  const pid = project.value
  try {
    switch (kind) {
      case 'project':
        lists.value.project = asItems(await api.get<{ items: { id: string; key: string; name: string }[] }>('/projects')).map((p) => ({ value: p.id, label: `${p.key} · ${p.name}` }))
        break
      case 'release':
        lists.value.release = pid ? asItems(await api.get<{ items: { id: string; name: string; version?: string }[] }>(`/projects/${pid}/releases`)).map((r) => ({ value: r.id, label: r.version && r.version !== r.name ? `${r.name} (${r.version})` : r.name })) : []
        break
      case 'cycle':
        lists.value.cycle = pid ? asItems(await api.get<{ items: { id: string; name: string }[] }>(`/projects/${pid}/cycles`)).map((c) => ({ value: c.id, label: c.name })) : []
        break
      case 'variant':
        lists.value.variant = pid ? asItems(await api.get<{ items: { id: string; name: string }[] }>(`/projects/${pid}/variants`)).map((v) => ({ value: v.id, label: v.name })) : []
        break
    }
  } catch {
    lists.value[kind] = []
  }
}

watch(
  () => props.widget,
  (wd) => {
    if (!wd) return
    title.value = wd.title ?? ''
    w.value = wd.w
    h.value = wd.h
    const v: Record<string, unknown> = {}
    for (const f of settingFields(wd.type, props.home)) {
      const cur = setting(wd, f.key) ?? ''
      v[f.key] = f.kind === 'select' ? String(cur) : cur
    }
    values.value = v
    lists.value = {}
    for (const f of fields.value) if (['project', 'release', 'cycle', 'variant'].includes(f.kind)) void loadList(f.kind)
  },
  { immediate: true },
)
// another project: its releases, cycles and variants
watch(
  () => values.value.project_id,
  () => {
    for (const k of ['release', 'cycle', 'variant']) if (fields.value.some((f) => f.kind === k)) void loadList(k)
  },
)

const fid = (key: string) => `ws-${key}`
const fieldLabel = (key: string) => t(`dashboards.field.${key}`)
function optionLabel(key: string, v: string): string {
  if (key === 'range') return t('dashboard.rangeDays', { n: Number(v) })
  if (key === 'dimension') return t(`dashboards.dimension.${v}`)
  const k = `dashboards.opt.${key}.${v || 'any'}`
  return te(k) ? t(k) : v
}
function selectOptions(f: SettingField): Opt[] {
  if (f.kind === 'select') return (f.options ?? []).map((v) => ({ value: v, label: optionLabel(f.key, v) }))
  const any: Opt = { value: '', label: t(`dashboards.auto.${f.kind}`) }
  const list = lists.value[f.kind] ?? []
  return f.kind === 'project' ? list : [any, ...list]
}

function save() {
  if (!props.widget) return
  const settings: Record<string, unknown> = {}
  for (const f of fields.value) {
    let v = values.value[f.key]
    if (f.key === 'range' && v !== '' && v != null) v = Number(v)
    if (f.kind === 'number' && typeof v === 'number') v = Math.min(f.max ?? v, Math.max(f.min ?? v, v))
    settings[f.key] = v
  }
  const ww = Math.min(GRID_COLS, Math.max(1, Math.round(w.value || 1)))
  emit('save', {
    ...props.widget,
    title: title.value.trim() || undefined,
    w: ww,
    x: Math.min(props.widget.x, GRID_COLS - ww),
    h: Math.min(MAX_H, Math.max(1, Math.round(h.value || 1))),
    settings: cleanSettings({ ...(props.widget.settings ?? {}), ...settings }),
  })
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="widget ? t('dashboards.settingsOf', { title: defaultTitle(widget) }) : ''" :style="{ width: 'min(520px, calc(100vw - 16px))' }">
    <form v-if="widget" class="grid" style="gap: 12px" @submit.prevent="save">
      <div class="field">
        <label :for="fid('title')">{{ t('dashboards.field.title') }}</label>
        <InputText :id="fid('title')" v-model="title" :placeholder="defaultTitle({ ...widget, title: '' })" maxlength="200" class="w-full" />
      </div>
      <div class="row size-row">
        <div class="field">
          <label :for="fid('w')">{{ t('dashboards.field.w') }}</label>
          <InputNumber v-model="w" :input-id="fid('w')" :min="1" :max="12" show-buttons :aria-describedby="fid('size-hint')" />
        </div>
        <div class="field">
          <label :for="fid('h')">{{ t('dashboards.field.h') }}</label>
          <InputNumber v-model="h" :input-id="fid('h')" :min="1" :max="24" show-buttons :aria-describedby="fid('size-hint')" />
        </div>
      </div>
      <small :id="fid('size-hint')" class="muted">{{ t('dashboards.sizeHint') }}</small>
      <div v-for="f in fields" :key="f.key" class="field">
        <label :for="fid(f.key)">{{ fieldLabel(f.key) }}</label>
        <Select
          v-if="['select', 'project', 'release', 'cycle', 'variant'].includes(f.kind)"
          v-model="values[f.key] as string"
          :input-id="fid(f.key)"
          :options="selectOptions(f)"
          option-label="label"
          option-value="value"
          :filter="f.kind === 'project'"
          class="w-full"
        />
        <InputNumber v-else-if="f.kind === 'number'" v-model="values[f.key] as number" :input-id="fid(f.key)" :min="f.min" :max="f.max" show-buttons />
        <template v-else-if="f.kind === 'query'">
          <InputText :id="fid(f.key)" v-model="values[f.key] as string" class="w-full mono" :aria-describedby="fid('q-hint')" />
          <small :id="fid('q-hint')" class="muted">{{ t('dashboards.queryHint') }}</small>
        </template>
        <template v-else-if="f.kind === 'markdown'">
          <Textarea :id="fid(f.key)" v-model="values[f.key] as string" rows="6" auto-resize class="w-full" :aria-describedby="fid('md-hint')" />
          <small :id="fid('md-hint')" class="muted">{{ t('dashboards.markdownHint') }}</small>
        </template>
        <InputText v-else :id="fid(f.key)" v-model="values[f.key] as string" class="w-full" />
      </div>
      <div class="row" style="justify-content: flex-end; gap: 8px">
        <Button type="button" :label="t('common.cancel')" severity="secondary" outlined @click="emit('close')" />
        <Button type="submit" :label="t('dashboards.apply')" icon="pi pi-check" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.size-row { gap: 12px; flex-wrap: wrap; }
.mono { font-family: var(--qtf-mono); }
</style>
