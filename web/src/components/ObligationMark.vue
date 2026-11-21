<script setup lang="ts">
// One highlighted obligation keyword (MUST NOT, BÖR …): a styled span per
// level (not told apart by colour alone, see styles.css), a tooltip, and a
// short level hint for screen readers that is not copied with the text.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { enumLabel, obligationHint } from '@/i18n/labels'
import type { DisplayLevel } from '@/lib/obligationKeywords'

const props = defineProps<{ text: string; level: string; display: DisplayLevel; keyword?: string }>()
const { t } = useI18n()
const label = computed(() => enumLabel('obligation', props.level))
const title = computed(() => t('obligationKw.title', { keyword: props.keyword ?? props.text, hint: obligationHint(props.level) || label.value }))
</script>

<template>
  <span :class="['obligation-kw', `obligation-${display}`]" :title="title"
    >{{ text }}<span class="sr-only obligation-sr">{{ ' ' + t('obligationKw.sr', { level: label }) }}</span></span
  >
</template>
