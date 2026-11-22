<script setup lang="ts">
// One violation of a built-in consistency check: what is wrong, the items
// involved (each a link) and the actions that fix it.
import ItemRef from './ItemRef.vue'
import { computed, ref } from 'vue'
import Button from 'primevue/button'
import { applyFix, fixPermission, fixRoute, isDeleted, itemFixes, itemRoute, relatedFixes, relatedLine, violationLine, type FixAction } from '@/lib/consistency'
import type { TraceRelated, TraceViolation } from '@/lib/types'
import { enumLabel } from '@/i18n/labels'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  violation: TraceViolation
  /** the rule kind (built-in check) */
  check: string
  projectId: string
}>()
const emit = defineEmits<{ changed: [] }>()
const { t } = useI18n()
const auth = useAuthStore()
const notify = useNotify()
const busy = ref('')

const v = computed(() => props.violation)
const line = computed(() => violationLine(v.value, t))
const related = computed(() => v.value.related ?? [])
const own = computed(() => itemFixes(v.value).filter((f) => auth.can(fixPermission(f))))
const fixesOf = (r: TraceRelated) => relatedFixes(props.check, r).filter((f) => auth.can(fixPermission(f, props.check)))
const relText = (r: TraceRelated) => relatedLine(r, t, (s) => enumLabel('status', s))
const fixLabel = (f: FixAction, r?: TraceRelated) => t(`consistency.fix.${f}`, { key: r?.key ?? v.value.key })

async function run(f: FixAction, r: TraceRelated | null) {
  busy.value = f + (r ? r.id : '')
  try {
    if (await applyFix(f, props.check, v.value, r, props.projectId)) {
      notify.ok(t('consistency.fixed'))
      emit('changed')
    }
  } catch (e) {
    notify.fail(e, t('consistency.fixFailed'))
  } finally {
    busy.value = ''
  }
}
</script>

<template>
  <div class="cv">
    <div class="small">{{ line }}</div>
    <ul v-if="related.length" class="plain-list cv-related" :aria-label="t('consistency.involved', { key: v.key })">
      <li v-for="r in related" :key="r.kind + r.id + r.role" class="row cv-rel">
        <span class="sr-only">{{ t(`consistency.kind.${r.kind}`) }}</span>
        <ItemRef v-if="!isDeleted(r) && itemRoute(r, projectId)" :to="itemRoute(r, projectId)" :item-key="r.key" :title="r.title" :hint="r.title" title-class="ellipsis" />
        <span v-else class="muted"><span class="key">{{ r.key || r.id.slice(0, 8) }}</span> {{ r.title }}</span>
        <span class="small muted">{{ relText(r) }}</span>
        <Button
          v-for="f in fixesOf(r)"
          :key="f"
          :label="fixLabel(f, r)"
          size="small"
          text
          :loading="busy === f + r.id"
          :disabled="!!busy"
          class="cv-fix"
          @click="run(f, r)"
        />
      </li>
    </ul>
    <div v-if="own.length" class="row cv-own">
      <template v-for="f in own" :key="f">
        <RouterLink v-if="fixRoute(f, v, projectId)" :to="fixRoute(f, v, projectId)!" class="small">{{ fixLabel(f) }}</RouterLink>
        <Button v-else :label="fixLabel(f)" size="small" text :loading="busy === f" :disabled="!!busy" @click="run(f, null)" />
      </template>
    </div>
  </div>
</template>

<style scoped>
.cv { display: grid; gap: 2px; min-width: 0; }
.cv-related { display: grid; gap: 2px; padding-left: 12px; }
.cv-rel { flex-wrap: wrap; gap: 4px 8px; }
.cv-own { gap: 8px; flex-wrap: wrap; }
.cv-fix { padding-block: 0; }
</style>
