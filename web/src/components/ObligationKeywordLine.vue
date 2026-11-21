<script setup lang="ts">
// The line under a requirement's description while editing: the obligation
// keyword(s) found as the author types (in the browser, with the server's
// keyword tables), whether they match the obligation field, and hints for
// several keywords or keywords in lower case. Links to the obligation help.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ObligationMark from './ObligationMark.vue'
import StatusTag from './StatusTag.vue'
import { enumLabel } from '@/i18n/labels'
import { checkKeywords, displayLevel, keywordExamples } from '@/lib/obligationKeywords'

const props = defineProps<{
  text: string
  /** the obligation field ("" = detect automatically) */
  chosen: string
  /** language pack ("" / "auto" = detect) */
  lang?: string
  canWrite?: boolean
}>()
const emit = defineEmits<{ apply: [level: string]; help: [] }>()
const { t } = useI18n()

const lang = computed(() => (props.lang === 'auto' ? '' : (props.lang ?? '')))
const check = computed(() => checkKeywords(props.text, props.chosen, lang.value))
/** each distinct keyword once, as first written */
const shown = computed(() => {
  const seen = new Set<string>()
  return check.value.matches.filter((m) => (seen.has(m.keyword) ? false : (seen.add(m.keyword), true))).slice(0, 6)
})
const quote = (xs: string[]) => xs.join(', ')
</script>

<template>
  <div class="obligation-preview" role="group" :aria-label="t('obligationKw.previewLabel')">
    <span class="muted">{{ t('obligationKw.preview') }}:</span>
    <template v-if="shown.length">
      <span v-for="m in shown" :key="m.keyword" class="ob-note">
        <ObligationMark v-if="displayLevel(m.level)" :text="m.text" :level="m.level" :display="displayLevel(m.level)!" :keyword="m.keyword" />
        <span v-else>{{ m.text }}</span>
        <StatusTag :value="m.level" kind="obligation" />
      </span>
    </template>
    <span v-else class="muted">{{ t('obligationKw.none', { examples: keywordExamples(text, lang) }) }}</span>
    <span v-if="chosen && check.detected && !check.mismatch" class="ob-note"><i class="pi pi-check" aria-hidden="true" />{{ t('obligationKw.matches') }}</span>
    <span aria-live="polite" class="ob-note-group">
      <span v-if="check.mismatch" class="ob-note ob-warn">
        <i class="pi pi-exclamation-triangle" aria-hidden="true" />
        {{ t('obligationKw.mismatch', { detected: enumLabel('obligation', check.detected), chosen: enumLabel('obligation', chosen) }) }}
        <button v-if="canWrite" type="button" class="linklike-btn" @click="emit('apply', check.detected)">{{ t('obligationKw.apply', { level: enumLabel('obligation', check.detected) }) }}</button>
      </span>
      <span v-if="check.multiple" class="ob-note ob-warn"><i class="pi pi-exclamation-triangle" aria-hidden="true" />{{ t('obligationKw.multiple', { keywords: quote(check.distinct) }) }}</span>
      <span v-if="check.lowercase.length" class="ob-note"><i class="pi pi-info-circle" aria-hidden="true" />{{ t('obligationKw.lowercase', { keywords: quote([...new Set(check.lowercase.map((m) => m.text))]) }) }}</span>
    </span>
    <button type="button" class="linklike-btn" @click="emit('help')"><i class="pi pi-question-circle" aria-hidden="true" /> {{ t('obligationKw.help') }}</button>
  </div>
</template>

<style scoped>
.ob-note-group { display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: 4px 10px; }
.ob-note-group:empty { display: none; }
.linklike-btn { background: none; border: 0; padding: 0; font: inherit; color: var(--p-primary-color); cursor: pointer; text-decoration: underline; text-underline-offset: 2px; }
.linklike-btn:focus-visible { outline: 2px solid var(--qtf-focus, var(--p-primary-color)); outline-offset: 2px; border-radius: 2px; }
</style>
