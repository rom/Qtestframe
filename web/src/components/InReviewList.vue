<script setup lang="ts">
// Requirements and tests whose status is "review" and that concern the
// signed-in user, across projects (My work → Reviews → In review status).
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { api, asItems, errorMessage } from '@/lib/api'
import { daysWaiting, filterInReview, type InReviewItem } from '@/lib/inReview'
import { notificationTarget } from '@/lib/notifications'
import { formatDate } from '@/lib/format'
import { readPref, writePref } from '@/stores/ui'

const { t } = useI18n()
const items = ref<InReviewItem[]>([])
const loading = ref(false)
const error = ref('')
const onlyMine = ref(readPref('qtf.reviews.onlyMine') === '1')
watch(onlyMine, (v) => writePref('qtf.reviews.onlyMine', v ? '1' : '0'))
const shown = computed(() => filterInReview(items.value, onlyMine.value))

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = asItems(await api.get<{ items: InReviewItem[] }>('/reviews/in-review'))
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)
defineExpose({ load })

const route = (it: InReviewItem) => notificationTarget({ project_id: it.project_id, entity_type: it.entity_type, entity_id: it.entity_id })!
const severity = (reason: string) => (reason === 'requested' ? 'warn' : reason === 'reviewer' ? 'secondary' : 'info')
</script>

<template>
  <section class="card in-review" aria-labelledby="in-review-title">
    <div class="row in-review-head">
      <div class="grow">
        <h2 id="in-review-title" class="plain-heading">{{ t('reviews.inReview.title') }} <span v-if="items.length" class="count">{{ shown.length }}</span></h2>
        <div class="small muted">{{ t('reviews.inReview.intro') }}</div>
      </div>
      <span class="row nowrap small"><ToggleSwitch v-model="onlyMine" input-id="in-review-mine" /><label for="in-review-mine">{{ t('reviews.inReview.onlyMine') }}</label></span>
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <DataTable v-else v-stack :value="shown" data-key="entity_id" :loading="loading" :aria-busy="loading" size="small" paginator :rows="25" :always-show-paginator="false">
      <template #empty>{{ onlyMine ? t('reviews.inReview.emptyMine') : t('reviews.inReview.empty') }}</template>
      <Column :header="t('reviews.item')" body-class="stack-main">
        <template #body="{ data }">
          <RouterLink :to="route(data)" class="row nowrap" style="gap: 6px; min-width: 0">
            <i :class="data.entity_type === 'test' ? 'pi pi-check-square' : 'pi pi-list-check'" aria-hidden="true" />
            <span class="key">{{ data.key }}</span>
            <span class="ellipsis">{{ data.title }}</span>
          </RouterLink>
        </template>
      </Column>
      <Column field="project_key" :header="t('reviews.inReview.project')" style="width: 110px"><template #body="{ data }"><span :title="data.project_name">{{ data.project_key }}</span></template></Column>
      <Column :header="t('reviews.inReview.ownerOrAuthor')" style="width: 190px">
        <template #body="{ data }"><span class="small">{{ data.entity_type === 'requirement' && data.owner ? data.owner : data.created_by || '—' }}</span></template>
      </Column>
      <Column :header="t('reviews.inReview.waiting')" style="width: 150px">
        <template #body="{ data }"><span class="small" :title="formatDate(data.updated_at)">{{ t('reviews.inReview.days', { n: daysWaiting(data.updated_at) }, daysWaiting(data.updated_at)) }}</span></template>
      </Column>
      <Column :header="t('reviews.inReview.why')" style="width: 240px">
        <template #body="{ data }">
          <span class="row" style="gap: 4px">
            <Tag v-for="r in data.reasons" :key="r" :value="t(`reviews.inReview.reason.${r}`)" :severity="severity(r)" />
          </span>
        </template>
      </Column>
    </DataTable>
  </section>
</template>

<style scoped>
.in-review {
  margin-top: 16px;
}
.in-review-head {
  margin-bottom: 8px;
  align-items: flex-start;
}
.in-review h2 {
  font-size: 16px;
  margin: 0 0 2px;
}
</style>
