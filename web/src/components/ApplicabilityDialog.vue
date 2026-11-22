<script setup lang="ts">
// "Applies to": the variants one or several requirements or tests apply to —
// all variants, selected variants (each with the variants below it) or the
// variants an expression matches (PUT …/applicability).
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import SelectButton from 'primevue/selectbutton'
import HelpLink from '@/guide/HelpLink.vue'
import { api, errorMessage } from '@/lib/api'
import { applicabilityBody, applicabilityMode, variantOptions, type Applicability, type ApplicabilityView, type Variant } from '@/lib/variants'
import { uid } from '@/lib/a11y'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ projectId: string; itemType: 'requirement' | 'test'; itemIds: string[]; initial?: Applicability | null; variants: Variant[] }>()
const visible = defineModel<boolean>('visible', { required: true })
const emit = defineEmits<{ saved: [view: ApplicabilityView] }>()
const { t } = useI18n()
const id = uid('appl')

type Mode = 'all' | 'selected' | 'expression'
const mode = ref<Mode>('all')
const ids = ref<string[]>([])
const expression = ref('')
const busy = ref(false)
const error = ref('')
const modeOptions = computed(() => (['all', 'selected', 'expression'] as Mode[]).map((m) => ({ label: t(`variants.mode.${m}`), value: m })))
const options = computed(() => variantOptions(props.variants))

watch(
  visible,
  (v) => {
    if (!v) return
    mode.value = applicabilityMode(props.initial)
    ids.value = [...(props.initial?.variant_ids ?? [])]
    expression.value = props.initial?.expression ?? ''
    error.value = ''
  },
  { immediate: true },
)

const invalid = computed(() => (mode.value === 'selected' && !ids.value.length) || (mode.value === 'expression' && !expression.value.trim()))

async function save() {
  if (invalid.value || busy.value) return
  busy.value = true
  error.value = ''
  const body = applicabilityBody(mode.value, ids.value, expression.value)
  try {
    let view: ApplicabilityView
    if (props.itemIds.length === 1) {
      view = await api.put<ApplicabilityView>(`/${props.itemType === 'test' ? 'tests' : 'requirements'}/${props.itemIds[0]}/applicability`, body)
    } else {
      const res = await api.put<{ updated: number; applicability: ApplicabilityView }>(`/projects/${props.projectId}/applicability`, {
        item_type: props.itemType,
        item_ids: props.itemIds,
        ...body,
      })
      view = res.applicability
    }
    visible.value = false
    emit('saved', view)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="itemIds.length > 1 ? t('variants.appliesToTitleN', { n: itemIds.length }, itemIds.length) : t('variants.appliesToTitle')" style="width: min(560px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
      <div class="field">
        <span :id="`${id}-mode`" class="field-label">{{ t('variants.appliesTo') }}</span>
        <SelectButton v-model="mode" :options="modeOptions" option-label="label" option-value="value" :allow-empty="false" :aria-labelledby="`${id}-mode`" />
      </div>
      <div v-if="mode === 'selected'" class="field">
        <label :for="`${id}-ids`">{{ t('variants.selectedVariants') }}</label>
        <MultiSelect v-model="ids" :input-id="`${id}-ids`" :options="options" option-label="label" option-value="value" display="chip" filter :placeholder="t('variants.chooseVariants')" :aria-describedby="`${id}-ids-help`">
          <template #option="{ option }">
            <span :style="{ paddingInlineStart: `${option.depth * 14}px` }">{{ option.label }}</span>
          </template>
        </MultiSelect>
        <small :id="`${id}-ids-help`" class="muted">{{ t('variants.selectedHelp') }}</small>
      </div>
      <div v-if="mode === 'expression'" class="field">
        <label :for="`${id}-expr`">{{ t('variants.expression') }}</label>
        <InputText :id="`${id}-expr`" v-model="expression" class="mono" maxlength="1000" placeholder='region:EU AND NOT voltage:110V' :aria-describedby="`${id}-expr-help`" autocomplete="off" />
        <small :id="`${id}-expr-help`" class="muted">{{ t('variants.expressionHelp') }} <HelpLink page="variants-and-code" :anchor="{ en: 'applicability', sv: 'tillämplighet' }" /></small>
      </div>
      <p v-if="mode === 'all'" class="small muted" style="margin: 0">{{ t('variants.allHelp') }}</p>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" severity="secondary" text @click="visible = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :loading="busy" :disabled="invalid" />
      </div>
    </form>
  </Dialog>
</template>
