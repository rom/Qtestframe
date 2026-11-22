<script setup lang="ts">
// Picks one or more roles (a member holds the union of their permissions):
// the chosen roles as chips with a remove button each, and an "Add role"
// select with the roles not chosen yet. The model is kept in role order
// (admin and the built-in roles first, rbac.NormalizeRoles) and never
// becomes empty: the last role has no remove button, because a member
// always has at least one role. "change" is emitted with the new roles
// after every change by the user.
import { computed } from 'vue'
import Select from 'primevue/select'
import { normalizeRoles } from '@/lib/permissions'
import { enumLabel } from '@/i18n/labels'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  /** role names that may be added or removed */
  options: readonly string[]
  /** roles shown but not removable (e.g. ones the caller may not give or take) */
  locked?: readonly string[]
  /** id of the add-role select (for a <label for>) */
  inputId?: string
  /** accessible name of the group: whose roles these are */
  ariaLabel?: string
  ariaLabelledby?: string
  ariaDescribedby?: string
  disabled?: boolean
  size?: 'small' | 'large'
}>()
const model = defineModel<string[]>({ default: () => [] })
const emit = defineEmits<{ change: [roles: string[]] }>()
const { t } = useI18n()

const chosen = computed(() => normalizeRoles(model.value))
const addable = computed(() => normalizeRoles(props.options.filter((r) => !chosen.value.includes(r))).map((r) => ({ label: enumLabel('role', r), value: r })))
const removable = (r: string) => !props.disabled && chosen.value.length > 1 && props.options.includes(r) && !(props.locked ?? []).includes(r)

function set(next: string[]) {
  model.value = next
  emit('change', next)
}
function add(r: string | null) {
  if (!r || chosen.value.includes(r)) return
  set(normalizeRoles([...chosen.value, r]))
}
function remove(r: string) {
  if (!removable(r)) return
  set(chosen.value.filter((x) => x !== r))
}
</script>

<template>
  <div class="role-picker" role="group" :aria-label="ariaLabel" :aria-labelledby="ariaLabelledby" :aria-describedby="ariaDescribedby" :class="{ small: size === 'small' }">
    <ul class="chips" :aria-label="t('roles.chosen')">
      <li v-for="r in chosen" :key="r" class="role-chip">
        <span>{{ enumLabel('role', r) }}</span>
        <button v-if="removable(r)" type="button" class="chip-remove" :aria-label="t('roles.remove', { role: enumLabel('role', r) })" v-tooltip.bottom="t('roles.remove', { role: enumLabel('role', r) })" @click="remove(r)">
          <i class="pi pi-times" aria-hidden="true" />
        </button>
      </li>
    </ul>
    <Select
      v-if="addable.length && !disabled"
      :model-value="null"
      :input-id="inputId"
      :options="addable"
      option-label="label"
      option-value="value"
      :placeholder="t('roles.add')"
      :aria-label="ariaLabel ? t('roles.addTo', { name: ariaLabel }) : t('roles.add')"
      :size="size"
      class="role-add"
      @update:model-value="add"
    />
  </div>
</template>

<style scoped>
.role-picker {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 6px;
  min-width: 0;
}
.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 6px;
  list-style: none;
  margin: 0;
  padding: 0;
}
.role-chip {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px 4px 2px 10px;
  border-radius: 999px;
  background: var(--p-content-hover-background, var(--qtf-border));
  border: 1px solid var(--qtf-border);
  font-size: 13px;
  line-height: 1.5;
  white-space: nowrap;
}
.role-chip:not(:has(.chip-remove)) {
  padding-right: 10px;
}
.chip-remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border: none;
  border-radius: 50%;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font-size: 10px;
}
.chip-remove:hover {
  background: var(--qtf-border);
}
.chip-remove:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 1px;
}
.role-add {
  min-width: 130px;
}
.small .role-chip {
  font-size: 12px;
}
</style>
