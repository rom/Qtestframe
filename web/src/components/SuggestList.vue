<script setup lang="ts">
// The listbox and live region of a suggesting field (composables/suggest.ts).
import { useI18n } from 'vue-i18n'
import type { SuggestState } from '@/composables/suggest'

const props = defineProps<{ state: SuggestState }>()
const { t } = useI18n()
const s = props.state
</script>

<template>
  <ul v-if="s.open.value" :id="s.listId" class="suggest-list" role="listbox" :aria-label="t('suggest.label')">
    <li
      v-for="(o, i) in s.options.value"
      :id="s.optionId(i)"
      :key="o.value"
      role="option"
      :aria-selected="i === s.active.value"
      :class="{ active: i === s.active.value }"
      :title="o.source === 'project' ? t('suggest.fromProject') : t('suggest.fromHistory')"
      @mousedown.prevent="s.choose(i)"
      @mousemove="s.active.value = i"
    >
      <i class="pi" :class="o.source === 'project' ? 'pi-folder' : 'pi-history'" aria-hidden="true" />
      <span class="suggest-text">{{ o.value }}</span>
      <span v-if="o.source === 'project'" class="sr-only">{{ t('suggest.fromProject') }}</span>
    </li>
  </ul>
  <span class="sr-only" aria-live="polite">{{ s.status.value }}</span>
</template>

<style>
.suggest-list {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  z-index: 1200;
  margin: 4px 0 0;
  padding: 4px;
  list-style: none;
  max-height: min(260px, 50vh);
  overflow-y: auto;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  background: var(--qtf-card);
  color: var(--qtf-text);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.18);
  text-align: left;
}
.suggest-list li {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  cursor: pointer;
  min-width: 0;
  /* comfortable touch targets */
  min-height: 32px;
}
.suggest-list li > .pi {
  margin-top: 3px;
  opacity: 0.6;
  font-size: 0.8rem;
}
.suggest-list li.active {
  background: var(--p-highlight-background, var(--qtf-hover));
  color: var(--p-highlight-color, inherit);
}
.suggest-text {
  flex: 1;
  min-width: 0;
  white-space: pre-line;
  overflow-wrap: anywhere;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
@media (forced-colors: active) {
  .suggest-list li.active {
    outline: 2px solid Highlight;
  }
}
</style>
