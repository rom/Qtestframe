<script setup lang="ts">
// Plain text with its obligation keywords (MUST, SKA INTE …) highlighted,
// when the viewer's preference allows it. The text is rendered as text
// nodes only (no HTML); keywords in code spans are not marked. For texts
// with mentions and links use MentionText with `obligations`.
import { computed } from 'vue'
import ObligationMark from './ObligationMark.vue'
import { obligationPieces, type ObligationPiece } from '@/lib/obligationKeywords'
import { usePrefsStore } from '@/stores/prefs'

const props = defineProps<{ text: string | null | undefined; lang?: string }>()
const prefs = usePrefsStore()
const pieces = computed<ObligationPiece[]>(() => {
  const text = props.text ?? ''
  return prefs.highlightObligations ? obligationPieces(text, { lang: props.lang }) : [{ text }]
})
</script>

<template>
  <span class="obligation-text"
    ><template v-for="(p, i) in pieces" :key="i"
      ><ObligationMark v-if="p.display" :text="p.text" :level="p.level ?? ''" :display="p.display" :keyword="p.keyword" /><template v-else>{{ p.text }}</template></template
    ></span
  >
</template>
