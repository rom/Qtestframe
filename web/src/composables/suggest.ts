// The behaviour of a suggesting field (SuggestInput.vue, MentionInput.vue):
// after a pause in typing (SUGGEST_DEBOUNCE_MS) the field asks GET /suggest
// for its whole text and lists the answers under the field. Nothing is
// chosen until the user moves to a suggestion with the arrow keys, so Enter
// and Tab keep their usual meaning while the list is just open:
//
//   ↓ / ↑          move through the suggestions (and back to the typed text)
//   Enter or Tab   take the highlighted suggestion (replaces the field's text)
//   Escape         close the list (not the dialog around the field)
//   ↓ (Alt+↓ in a multi-line field)  open the list without typing: the
//                  most used recent values
//
// The field follows the ARIA combobox pattern: a single-line field is a
// role=combobox with aria-expanded, aria-controls and aria-activedescendant
// pointing into a role=listbox; a multi-line field keeps its textbox role
// (role=combobox is not allowed on a textarea) with aria-autocomplete,
// aria-controls and aria-activedescendant. A polite live region announces
// how many suggestions there are and what was inserted. When the personal
// preference turns autocomplete off, the field is a plain field again: no
// requests, no list and the browser's own autocomplete attribute back.
import { computed, nextTick, onBeforeUnmount, ref, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { usePrefsStore } from '@/stores/prefs'
import { uid } from '@/lib/a11y'
import { autocompleteEnabled, canSuggest, debounce, fetchSuggestions, nextActive, queryOf, SUGGEST_DEBOUNCE_MS, type SuggestField, type Suggestion } from '@/lib/suggest'

/** Attributes a suggesting field sets on its input. */
export interface SuggestAttrs {
  autocomplete?: string
  role?: string
  'aria-autocomplete'?: 'list'
  'aria-controls'?: string
  'aria-activedescendant'?: string
  'aria-expanded'?: 'true' | 'false'
}

export interface SuggestOptions {
  field: () => string | null | undefined
  projectId: () => string | null | undefined
  model: Ref<string>
  el: () => HTMLInputElement | HTMLTextAreaElement | null
  multiline: () => boolean
  /** Another popup owns the field for now (the @mention list). */
  suppressed?: () => boolean
}

export function useSuggest(o: SuggestOptions) {
  const { t } = useI18n()
  const prefs = usePrefsStore()
  const open = ref(false)
  const options = ref<Suggestion[]>([])
  const active = ref(-1)
  const status = ref('')
  const listId = uid('suggest-list')
  let seq = 0
  let ctrl: AbortController | null = null

  const enabled = computed(() => autocompleteEnabled(prefs.prefs) && canSuggest(o.field(), o.projectId()))

  function usable(): boolean {
    const e = o.el()
    return enabled.value && !!e && !e.disabled && !e.readOnly && !o.suppressed?.()
  }

  function close() {
    seq++ // answers still on the way are dropped
    ctrl?.abort()
    ctrl = null
    debounced.cancel()
    if (open.value) status.value = ''
    open.value = false
    active.value = -1
  }

  async function query(allowEmpty: boolean) {
    if (!usable()) return close()
    const q = queryOf(o.model.value, { allowEmpty })
    if (q === null) return close()
    const my = ++seq
    ctrl?.abort()
    ctrl = typeof AbortController === 'undefined' ? null : new AbortController()
    try {
      const items = await fetchSuggestions(o.field() as SuggestField, q, o.projectId() ?? '', { signal: ctrl?.signal })
      const e = o.el()
      if (my !== seq || !e || document.activeElement !== e || !usable()) return
      options.value = items
      active.value = -1
      open.value = items.length > 0
      status.value = open.value ? t('suggest.count', { count: items.length }, items.length) : ''
    } catch {
      if (my === seq) close()
    }
  }
  const debounced = debounce(() => void query(false), SUGGEST_DEBOUNCE_MS)

  /** After every edit of the text: ask again after a pause. */
  function onInput() {
    if (!usable()) {
      if (open.value) close()
      return
    }
    debounced.call()
  }

  function choose(i: number) {
    const s = options.value[i]
    const e = o.el()
    if (!s) return
    o.model.value = s.value
    close()
    status.value = t('suggest.inserted', { value: s.value.length > 80 ? s.value.slice(0, 80) + '…' : s.value })
    void nextTick(() => {
      if (!e) return
      e.focus()
      const end = s.value.length
      e.setSelectionRange?.(end, end)
    })
  }

  /** Keyboard handling; true when the key was used by the list. */
  function onKeydown(e: KeyboardEvent): boolean {
    if (o.suppressed?.()) return false
    if (!open.value) {
      if (e.key === 'ArrowDown' && (e.altKey || !o.multiline()) && usable()) {
        e.preventDefault()
        debounced.cancel()
        void query(true)
        return true
      }
      if (e.key === 'Escape' && debounced.pending()) debounced.cancel()
      return false
    }
    const n = options.value.length
    const plain = !e.ctrlKey && !e.metaKey && !e.altKey && !e.shiftKey
    switch (e.key) {
      case 'ArrowDown':
      case 'ArrowUp':
        e.preventDefault()
        active.value = nextActive(active.value, n, e.key === 'ArrowDown' ? 1 : -1)
        return true
      case 'Enter':
        if (!plain || active.value < 0) return false
        e.preventDefault()
        e.stopImmediatePropagation()
        choose(active.value)
        return true
      case 'Tab':
        if (!plain || active.value < 0) {
          close()
          return false
        }
        e.preventDefault()
        choose(active.value)
        return true
      case 'Escape':
        // close the list only, not the drawer or dialog around the field
        e.preventDefault()
        e.stopPropagation()
        close()
        return true
    }
    return false
  }

  let blurTimer: ReturnType<typeof setTimeout> | undefined
  function onBlur() {
    blurTimer = setTimeout(close, 150)
  }
  function onFocus() {
    clearTimeout(blurTimer)
  }
  onBeforeUnmount(() => {
    clearTimeout(blurTimer)
    close()
  })

  const optionId = (i: number) => `${listId}-${i}`
  /** Attributes of the field (nothing while autocomplete is off). */
  const attrs = computed<SuggestAttrs>(() => {
    if (!enabled.value) return {}
    const a: SuggestAttrs = {
      autocomplete: 'off',
      'aria-autocomplete': 'list',
      'aria-controls': open.value ? listId : undefined,
      'aria-activedescendant': open.value && active.value >= 0 ? optionId(active.value) : undefined,
    }
    if (!o.multiline()) {
      a.role = 'combobox'
      a['aria-expanded'] = open.value ? 'true' : 'false'
    }
    return a
  })

  return { open, options, active, status, listId, optionId, enabled, attrs, onInput, onKeydown, onBlur, onFocus, choose, close }
}

export type SuggestState = ReturnType<typeof useSuggest>
