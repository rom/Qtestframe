<script setup lang="ts">
// Administration → Field sets and templates: per project type, the custom
// fields every new project of that type gets and the organisation's
// default item templates (settings.manage to change them).
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import Select from 'primevue/select'
import CustomFieldsSettings from '@/components/CustomFieldsSettings.vue'
import TemplatesSettings from '@/components/TemplatesSettings.vue'
import { P } from '@/lib/permissions'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const auth = useAuthStore()
const route = useRoute()
const router = useRouter()
const types = ['general', 'software', 'hardware', 'system', 'business']
const projectType = ref(types.includes(route.query.type as string) ? (route.query.type as string) : 'general')
const typeOptions = computed(() => types.map((v) => ({ value: v, label: t(`projectType.${v}`) })))
const canEdit = computed(() => auth.can(P.settingsManage))
function choose(v: string) {
  projectType.value = v
  void router.replace({ query: { ...route.query, type: v } })
}
</script>

<template>
  <div class="grid" style="gap: 16px">
    <section class="card" style="max-width: 980px" aria-labelledby="fs-h">
      <h2 id="fs-h" class="plain-heading" style="margin: 0 0 4px">{{ t('fieldSets.title') }}</h2>
      <p class="small muted" style="margin-top: 0">{{ t('fieldSets.intro') }}</p>
      <div class="field" style="max-width: 280px">
        <label for="fs-type">{{ t('projectType.label') }}</label>
        <Select input-id="fs-type" :model-value="projectType" :options="typeOptions" option-label="label" option-value="value" fluid @update:model-value="choose" />
      </div>
    </section>
    <CustomFieldsSettings :project-type="projectType" :can-edit="canEdit" />
    <TemplatesSettings :project-type="projectType" :can-edit="canEdit" />
  </div>
</template>
