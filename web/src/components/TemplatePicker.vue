<script setup lang="ts">
// The template picker of "New requirement" / "New test": the project's
// templates, the organisation's for the project type and the built-in
// ones. The project default is chosen (and applied) when it opens.
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from 'primevue/select'
import { api } from '@/lib/api'
import { uid } from '@/lib/a11y'
import { groupTemplates, type ItemTemplate, type TemplateContent, type TemplateKind } from '@/lib/customFields'

const props = defineProps<{ projectId: string; kind: TemplateKind }>()
const emit = defineEmits<{ apply: [content: TemplateContent, template: ItemTemplate] }>()
const { t, locale } = useI18n()
const list = ref<ItemTemplate[]>([])
const chosen = ref<string | null>(null)
const inputId = uid('tpl-pick')

const groups = computed(() =>
  groupTemplates(list.value).map((g) => ({
    label: t(`templates.source.${g.source}`),
    items: g.items.map((x) => ({ value: x.id, label: x.default ? t('templates.defaultName', { name: x.name }) : x.name, description: x.description })),
  })),
)

onMounted(async () => {
  try {
    const res = await api.get<{ items: ItemTemplate[] }>(`/projects/${props.projectId}/templates`, { kind: props.kind, lang: locale.value })
    list.value = res.items ?? []
    const def = list.value.find((x) => x.default)
    if (def) choose(def.id)
  } catch {
    list.value = [] // creating works without templates
  }
})

function choose(id: string | null) {
  chosen.value = id
  const tpl = list.value.find((x) => x.id === id)
  if (tpl) emit('apply', tpl.content, tpl)
}
</script>

<template>
  <div v-if="list.length" class="field template-picker">
    <label :for="inputId">{{ t('templates.pick') }}</label>
    <Select
      :input-id="inputId"
      :model-value="chosen"
      :options="groups"
      option-group-label="label"
      option-group-children="items"
      option-label="label"
      option-value="value"
      :placeholder="t('templates.none')"
      :aria-describedby="`${inputId}-hint`"
      filter
      fluid
      @update:model-value="choose"
    >
      <template #option="{ option }">
        <div>
          <div>{{ option.label }}</div>
          <div v-if="option.description" class="small muted">{{ option.description }}</div>
        </div>
      </template>
    </Select>
    <small :id="`${inputId}-hint`" class="muted">{{ t('templates.pickHint') }}</small>
  </div>
</template>
