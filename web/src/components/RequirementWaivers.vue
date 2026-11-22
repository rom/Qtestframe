<script setup lang="ts">
// The waivers and deviations that cover a requirement (shown in the
// requirement editor), with a link to request a new one.
import ItemRef from './ItemRef.vue'
import { computed, ref, watch } from 'vue'
import Tag from 'primevue/tag'
import { api, asItems } from '@/lib/api'
import { formatDate } from '@/lib/format'
import { P } from '@/lib/permissions'
import { waiverStatusIcon, waiverStatusSeverity, type Waiver } from '@/lib/waivers'
import { useAuthStore } from '@/stores/auth'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ requirementId: string; projectId: string }>()
const { t } = useI18n()
const auth = useAuthStore()
const list = ref<Waiver[]>([])
const failed = ref(false)
const canWrite = computed(() => auth.can(P.requirementWrite))
const inForce = computed(() => list.value.filter((w) => w.active))

watch(
  () => props.requirementId,
  async (id) => {
    failed.value = false
    try {
      list.value = asItems(await api.get<{ items: Waiver[] }>(`/requirements/${id}/waivers`))
    } catch {
      list.value = []
      failed.value = true
    }
  },
  { immediate: true },
)
</script>

<template>
  <div v-if="!failed" class="requirement-waivers">
    <div class="section-title">
      <i class="pi pi-verified" aria-hidden="true" /> {{ t('waivers.ofRequirement', { count: list.length }, list.length) }}
      <span class="grow" />
      <RouterLink v-if="canWrite" :to="{ name: 'waivers', params: { projectId }, query: { new: '1', requirement: requirementId } }" class="small" style="text-transform: none; letter-spacing: 0">{{ t('waivers.request') }}</RouterLink>
    </div>
    <p v-if="inForce.length" class="small" style="margin: 2px 0 6px"><Tag :value="t('waivers.acceptedBy', { keys: inForce.map((w) => w.key).join(', ') })" severity="contrast" icon="pi pi-verified" /></p>
    <div v-if="!list.length" class="muted small">{{ t('waivers.noneForRequirement') }}</div>
    <div v-for="w in list" :key="w.id" class="row nowrap" style="padding: 5px 0; border-bottom: 1px solid var(--qtf-border)">
      <ItemRef :to="{ name: 'waivers', params: { projectId }, query: { waiver: w.id } }" :item-key="w.key" :title="w.title" title-class="ellipsis" grow />
      <span class="small muted">{{ w.release_id ? w.release_name : t('waivers.wholeProject') }}</span>
      <span v-if="w.expires_on" class="small muted">{{ t('waivers.until', { date: formatDate(w.expires_on) }) }}</span>
      <Tag :value="t(`waivers.status.${w.status}`)" :severity="waiverStatusSeverity(w.status)" :icon="waiverStatusIcon(w.status)" />
    </div>
  </div>
</template>
