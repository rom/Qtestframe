<script setup lang="ts">
// "Add widget": the widget types the user may see, with a short description;
// choosing one adds it in the first free place of the dashboard.
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import { availableWidgets } from '@/lib/widgets'
import { useAuthStore } from '@/stores/auth'

const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ add: [type: string] }>()
const { t } = useI18n()
const auth = useAuthStore()
const filter = ref('')
const list = computed(() => {
  const q = filter.value.trim().toLowerCase()
  return availableWidgets((perms) => perms.every((p) => auth.can(p)))
    .map((d) => ({ ...d, name: t(`dashboards.types.${d.type}`), about: t(`dashboards.about.${d.type}`) }))
    .filter((d) => !q || d.name.toLowerCase().includes(q) || d.about.toLowerCase().includes(q))
    .sort((a, b) => a.name.localeCompare(b.name))
})
function add(type: string) {
  emit('add', type)
  visible.value = false
  filter.value = ''
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('dashboards.addWidget')" :style="{ width: 'min(640px, calc(100vw - 16px))' }">
    <div class="field">
      <label for="aw-filter" class="sr-only">{{ t('dashboards.filterWidgets') }}</label>
      <InputText id="aw-filter" v-model="filter" :placeholder="t('dashboards.filterWidgets')" class="w-full" autofocus />
    </div>
    <ul class="plain-list widget-types" :aria-label="t('dashboards.widgetTypes')">
      <li v-for="d in list" :key="d.type">
        <button type="button" class="widget-type" :data-type="d.type" @click="add(d.type)">
          <i :class="d.icon" aria-hidden="true" />
          <span class="wt-text">
            <span class="wt-name">{{ d.name }}</span>
            <span class="wt-about small muted">{{ d.about }}</span>
          </span>
        </button>
      </li>
      <li v-if="!list.length" class="muted small">{{ t('dashboards.noWidgetMatch') }}</li>
    </ul>
  </Dialog>
</template>

<style scoped>
.widget-types { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 8px; margin-top: 12px; }
.widget-type { display: flex; gap: 10px; align-items: flex-start; width: 100%; text-align: left; padding: 10px; border: 1px solid var(--qtf-border); border-radius: 8px; background: var(--qtf-card); color: var(--qtf-text); cursor: pointer; font: inherit; }
.widget-type:hover { background: var(--qtf-hover); }
.widget-type:focus-visible { outline: 2px solid var(--qtf-focus); outline-offset: 2px; }
.widget-type i { font-size: 18px; margin-top: 2px; }
.wt-text { display: grid; gap: 2px; }
.wt-name { font-weight: 600; }
</style>
