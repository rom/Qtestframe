<script setup lang="ts">
// "Variants" section of a requirement or test: the variants it applies to
// (editable) and, for a requirement, its coverage in every variant and, per
// variant, in every test configuration. Hidden while the project has no
// variants.
import { computed, onMounted, ref, watch } from 'vue'
import Button from 'primevue/button'
import Tag from 'primevue/tag'
import StatusTag from './StatusTag.vue'
import ApplicabilityDialog from './ApplicabilityDialog.vue'
import HelpLink from '@/guide/HelpLink.vue'
import { api } from '@/lib/api'
import { appliesToAll, type Applicability, type ApplicabilityView, type VariantCoverageRow } from '@/lib/variants'
import { useVariants } from '@/composables/variants'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ itemType: 'requirement' | 'test'; itemId: string; projectId: string; canWrite: boolean; refresh?: unknown }>()
const emit = defineEmits<{ changed: [a: Applicability | null] }>()
const { t } = useI18n()
const notify = useNotify()
const vs = useVariants(() => props.projectId)
onMounted(vs.load)

const view = ref<ApplicabilityView | null>(null)
const coverage = ref<VariantCoverageRow[]>([])
const dialog = ref(false)
const base = computed(() => `/${props.itemType === 'test' ? 'tests' : 'requirements'}/${props.itemId}`)

async function load() {
  try {
    view.value = await api.get<ApplicabilityView>(`${base.value}/applicability`)
  } catch {
    view.value = null
  }
  if (props.itemType === 'requirement') {
    try {
      coverage.value = (await api.get<{ variants: VariantCoverageRow[] }>(`${base.value}/variants`)).variants ?? []
    } catch {
      coverage.value = []
    }
  }
}
watch(() => [props.itemId, props.refresh], load, { immediate: true })

const all = computed(() => appliesToAll(view.value))
function saved(v: ApplicabilityView) {
  view.value = v
  notify.ok(t('variants.applicabilitySaved'))
  emit('changed', v.all ? null : v)
  void load()
}
const cfgLabel = (c: { id: string; name: string }) => (c.id ? c.name : t('configurations.withoutShort'))
</script>

<template>
  <section v-if="vs.loaded.value && vs.variants.value.length" class="applicability" aria-labelledby="appl-heading">
    <div class="section-title">
      <i class="pi pi-clone" aria-hidden="true" /> <span id="appl-heading">{{ t('variants.sectionTitle') }}</span>
      <span class="grow" />
      <Button v-if="canWrite" :label="t('variants.editApplicability')" icon="pi pi-pencil" size="small" text aria-haspopup="dialog" @click="dialog = true" />
      <HelpLink page="variants-and-code" :anchor="{ en: 'applicability', sv: 'tillämplighet' }" />
    </div>
    <div class="row" style="gap: 6px; flex-wrap: wrap">
      <span class="small muted">{{ t('variants.appliesTo') }}:</span>
      <Tag v-if="all" :value="t('variants.allVariants')" severity="secondary" />
      <template v-else>
        <Tag v-for="v in view?.variants ?? []" :key="v.id" :value="v.name" />
        <code v-if="view?.expression" class="small">{{ view.expression }}</code>
        <span class="small muted">→ {{ (view?.applies_to ?? []).map((v) => v.name).join(', ') || t('variants.noVariant') }}</span>
      </template>
    </div>
    <div v-if="itemType === 'requirement' && coverage.length" class="table-scroll" style="margin-top: 8px">
      <table class="var-table">
        <caption class="sr-only">{{ t('variants.coverageCaption') }}</caption>
        <thead>
          <tr>
            <th scope="col">{{ t('variants.variant') }}</th>
            <th scope="col">{{ t('traceability.coverage') }}</th>
            <th scope="col">{{ t('variants.byConfiguration') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in coverage" :key="row.variant.id">
            <th scope="row" class="variant-name">{{ row.variant.path || row.variant.name }}</th>
            <td><StatusTag :value="row.coverage" kind="coverage" /></td>
            <td>
              <span v-for="c in row.configurations" :key="c.id" class="row nowrap small cfg-cell">{{ cfgLabel(c) }} <StatusTag :value="c.coverage" kind="coverage" /></span>
              <span v-if="!row.configurations.length" class="muted small">—</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <ApplicabilityDialog v-model:visible="dialog" :project-id="projectId" :item-type="itemType" :item-ids="[itemId]" :initial="view" :variants="vs.variants.value" @saved="saved" />
  </section>
</template>

<style scoped>
.applicability {
  margin-top: 12px;
}
.table-scroll {
  overflow-x: auto;
}
.var-table {
  border-collapse: collapse;
  width: 100%;
}
.var-table th,
.var-table td {
  text-align: left;
  padding: 4px 8px;
  border-bottom: 1px solid var(--qtf-border);
  vertical-align: top;
}
.var-table thead th {
  font-size: 12px;
  font-weight: 600;
}
.variant-name {
  font-weight: 500;
}
.cfg-cell {
  display: inline-flex;
  gap: 4px;
  margin-inline-end: 8px;
}
</style>
