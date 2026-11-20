<script setup lang="ts">
// A small "?" link from an app page to its part of the user guide.
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { helpPath } from './links'

// anchor: a heading slug, or one per language ({ en: 'evidence', sv: 'bevis' })
const props = defineProps<{ page: string; anchor?: string | Record<string, string> }>()
const { t, locale } = useI18n()
const to = computed(() => {
  const a = props.anchor
  const slug = typeof a === 'string' ? a : (a?.[String(locale.value)] ?? '')
  return helpPath(props.page, slug)
})
</script>

<template>
  <RouterLink :to="to" class="qtf-help-link" :aria-label="t('guide.thisPage')" v-tooltip.bottom="t('guide.thisPage')">
    <i class="pi pi-question-circle" aria-hidden="true" />
  </RouterLink>
</template>

<style>
.qtf-help-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border-radius: 50%;
  color: var(--qtf-muted);
  text-decoration: none;
  flex: none;
}
.qtf-help-link:hover {
  background: var(--qtf-hover);
  color: var(--p-primary-color);
}
.qtf-help-link .pi {
  font-size: 1.05rem;
}
</style>
