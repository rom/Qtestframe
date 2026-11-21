<script setup lang="ts">
// A text field (or textarea with `multiline`) where typing "@" suggests the
// people of the project: users of the organisation and names already used
// as owner or assignee. Arrow keys move through the list, Enter or Tab
// inserts "@handle", Escape closes the list. With `suggest` (a field key of
// lib/suggest.ts) the field also suggests the values saved in it before
// whenever no "@" list is open (composables/suggest.ts). All other
// attributes and listeners go to the inner PrimeVue field.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import InputText from 'primevue/inputtext'
import Textarea from 'primevue/textarea'
import UserAvatar from './UserAvatar.vue'
import SuggestList from './SuggestList.vue'
import { useSuggest } from '@/composables/suggest'
import type { SuggestField } from '@/lib/suggest'
import { activeMention, insertMention, rankPeople, type Person } from '@/lib/mentions'
import { uid } from '@/lib/a11y'
import { useProjectPeopleStore } from '@/stores/projectPeople'

defineOptions({ inheritAttrs: false })
const model = defineModel<string>({ default: '' })
const props = defineProps<{ multiline?: boolean; projectId?: string; suggest?: SuggestField }>()
const { t } = useI18n()
const route = useRoute()
const store = useProjectPeopleStore()

const field = ref<{ $el: HTMLInputElement | HTMLTextAreaElement } | null>(null)
const open = ref(false)
const options = ref<Person[]>([])
const active = ref(0)
const status = ref('')
let mention: { start: number; query: string } | null = null
const listId = uid('mention-list')
const pid = computed(() => props.projectId ?? (typeof route.params.projectId === 'string' ? route.params.projectId : ''))
const sg = useSuggest({
  field: () => props.suggest,
  projectId: () => pid.value,
  model,
  el: () => el(),
  multiline: () => !!props.multiline,
  suppressed: () => open.value,
})

function el(): HTMLInputElement | HTMLTextAreaElement | null {
  return field.value?.$el ?? null
}
function close() {
  if (open.value) status.value = ''
  open.value = false
  mention = null
}
async function refresh() {
  const e = el()
  if (!e || e.disabled || e.readOnly) return close()
  const m = activeMention(model.value ?? '', e.selectionStart ?? (model.value ?? '').length)
  if (!m) return close()
  await store.ensure(pid.value)
  mention = m
  options.value = rankPeople(store.people, m.query)
  active.value = 0
  open.value = options.value.length > 0
  if (open.value) sg.close()
  status.value = open.value ? t('mentions.count', { count: options.value.length }) : t('mentions.none')
}
function choose(p: Person) {
  const e = el()
  if (!e || !mention) return
  const r = insertMention(model.value ?? '', mention.start, e.selectionStart ?? mention.start + 1 + mention.query.length, p.handle)
  model.value = r.text
  close()
  status.value = t('mentions.inserted', { name: p.name })
  void nextTick(() => {
    e.focus()
    e.setSelectionRange(r.caret, r.caret)
  })
}
function onKeydown(e: KeyboardEvent) {
  if (!open.value) {
    sg.onKeydown(e)
    return
  }
  const n = options.value.length
  if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
    e.preventDefault()
    active.value = (active.value + (e.key === 'ArrowDown' ? 1 : n - 1)) % n
  } else if ((e.key === 'Enter' && !e.ctrlKey && !e.metaKey) || e.key === 'Tab') {
    e.preventDefault()
    e.stopImmediatePropagation()
    choose(options.value[active.value])
  } else if (e.key === 'Escape') {
    // close the list only, not the drawer or dialog around the field
    e.preventDefault()
    e.stopPropagation()
    close()
  }
}
function onInput() {
  void nextTick(refresh)
  sg.onInput()
}
function onKeyup(e: KeyboardEvent) {
  if (e.key === 'ArrowLeft' || e.key === 'ArrowRight' || e.key === 'Home' || e.key === 'End') void refresh()
}
let blurTimer: ReturnType<typeof setTimeout> | undefined
function onBlur() {
  blurTimer = setTimeout(close, 150)
  sg.onBlur()
}
function onFocus() {
  clearTimeout(blurTimer)
  sg.onFocus()
}
watch(pid, () => close())
onMounted(() => {
  // warm the cache so the first "@" answers at once
  if (pid.value) void store.ensure(pid.value)
})
const optionId = (i: number) => `${listId}-${i}`
// the @mention list while it is open, otherwise the suggestions (if any)
const comboAttrs = computed(() => ({
  ...sg.attrs.value,
  'aria-autocomplete': 'list' as const,
  ...(open.value ? { 'aria-controls': listId, 'aria-activedescendant': optionId(active.value) } : {}),
  ...(open.value && sg.attrs.value.role ? { 'aria-expanded': 'true' as const } : {}),
}))
</script>

<template>
  <div class="mention-input">
    <Textarea
      v-if="multiline"
      ref="field"
      v-model="model"
      v-bind="{ ...$attrs, ...comboAttrs }"
      @keydown="onKeydown"
      @input="onInput"
      @keyup="onKeyup"
      @click="refresh"
      @blur="onBlur"
      @focus="onFocus"
    />
    <InputText
      v-else
      ref="field"
      v-model="model"
      v-bind="{ ...$attrs, ...comboAttrs }"
      @keydown="onKeydown"
      @input="onInput"
      @keyup="onKeyup"
      @click="refresh"
      @blur="onBlur"
      @focus="onFocus"
    />
    <ul v-if="open" :id="listId" class="mention-list" role="listbox" :aria-label="t('mentions.label')">
      <li
        v-for="(p, i) in options"
        :id="optionId(i)"
        :key="p.handle"
        role="option"
        :aria-selected="i === active"
        :class="{ active: i === active }"
        @mousedown.prevent="choose(p)"
        @mousemove="active = i"
      >
        <UserAvatar :avatar="p.avatar ?? ''" :user-id="p.user_id" :email="p.email" :name="p.name" :size="22" />
        <span class="grow ellipsis">
          <span class="mention-name">{{ p.name }}</span>
          <span class="muted small"> @{{ p.handle }}</span>
        </span>
        <span v-if="p.kind !== 'user'" class="muted small nowrap">{{ t('mentions.noAccount') }}</span>
      </li>
    </ul>
    <span class="sr-only" aria-live="polite">{{ status }}</span>
    <SuggestList :state="sg" />
  </div>
</template>

<style>
.mention-input {
  position: relative;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.mention-input > .p-inputtext,
.mention-input > .p-textarea {
  width: 100%;
}
.mention-list {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  z-index: 1200;
  margin: 4px 0 0;
  padding: 4px;
  list-style: none;
  max-height: 260px;
  overflow-y: auto;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  background: var(--qtf-card);
  color: var(--qtf-text);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.18);
}
.mention-list li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border-radius: 6px;
  cursor: pointer;
  min-width: 0;
}
.mention-list li.active {
  background: var(--p-highlight-background, var(--qtf-hover));
  color: var(--p-highlight-color, inherit);
}
.mention-name {
  font-weight: 600;
}
</style>
