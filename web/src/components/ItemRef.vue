<script setup lang="ts">
// An item's key and title as one link (or button), so clicking either opens
// the item. One focus stop and one accessible name ("KEY Title") instead of
// two links to the same place.
import type { RouteLocationRaw } from 'vue-router'

defineProps<{
  /** where the link leads; without it the item is a button (emits click) */
  to?: RouteLocationRaw
  itemKey?: string | null
  title?: string | null
  /** classes for the title, e.g. "ellipsis grow" or "muted" */
  titleClass?: string
  /** take the remaining width of a flex row (the title is cut with an ellipsis) */
  grow?: boolean
  /** tooltip, e.g. the full title when it is cut */
  hint?: string
}>()
const emit = defineEmits<{ click: [e: MouseEvent] }>()
</script>

<template>
  <RouterLink v-if="to" :to="to" class="item-ref" :class="{ grow }" :title="hint" @click="(e: MouseEvent) => emit('click', e)">
    <span class="key">{{ itemKey }}</span><span v-if="title" class="item-ref-title" :class="titleClass">{{ title }}</span>
  </RouterLink>
  <button v-else type="button" class="item-ref link-button" :class="{ grow }" :title="hint" @click="(e: MouseEvent) => emit('click', e)">
    <span class="key">{{ itemKey }}</span><span v-if="title" class="item-ref-title" :class="titleClass">{{ title }}</span>
  </button>
</template>

<style>
.item-ref {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  max-width: 100%;
  color: inherit;
  text-decoration: none;
  text-align: left;
}
.item-ref.link-button {
  display: inline-flex;
  gap: 6px;
  cursor: pointer;
}
.item-ref.link-button:hover {
  text-decoration: none;
}
.item-ref.grow {
  flex: 1 1 auto;
}
.item-ref-title {
  min-width: 0;
  color: var(--qtf-text);
}
.item-ref:hover .item-ref-title,
.item-ref:focus-visible .item-ref-title {
  color: var(--p-primary-color);
  text-decoration: underline;
  text-underline-offset: 2px;
}
</style>
