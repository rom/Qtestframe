<script setup lang="ts">
// Help on obligation levels next to a requirement's description, in the
// active language: the keywords per level (RFC 2119 / RFC 8174, and the
// Swedish ones the language analysis knows), what they mean and an example.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { helpPath } from '@/guide/links'

const props = defineProps<{ open?: boolean }>()
const { t, locale } = useI18n()
const guide = computed(() => helpPath('requirements', String(locale.value) === 'sv' ? 'kravnivåer' : 'obligation-levels'))
const expanded = ref(!!props.open)
watch(
  () => props.open,
  (v) => (expanded.value = !!v),
)
const LEVELS = ['must', 'must_not', 'should', 'should_not', 'may', 'will', 'describe'] as const
const rows = computed(() => LEVELS.map((l) => ({ level: l, keywords: t(`obligationHelp.rows.${l}.keywords`), meaning: t(`obligationHelp.rows.${l}.meaning`), example: t(`obligationHelp.rows.${l}.example`) })))
</script>

<template>
  <details class="obligation-help" :open="expanded" @toggle="expanded = ($event.target as HTMLDetailsElement).open">
    <summary><i class="pi pi-question-circle" aria-hidden="true" /> {{ t('obligationHelp.summary') }}</summary>
    <div class="body">
      <p class="small">{{ t('obligationHelp.intro') }}</p>
      <table class="obligation-table">
        <caption class="sr-only">{{ t('obligationHelp.summary') }}</caption>
        <thead>
          <tr>
            <th scope="col">{{ t('obligationHelp.keyword') }}</th>
            <th scope="col">{{ t('obligationHelp.meaning') }}</th>
            <th scope="col">{{ t('obligationHelp.example') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="r in rows" :key="r.level">
            <th scope="row" class="mono">{{ r.keywords }}</th>
            <td>{{ r.meaning }}</td>
            <td class="muted"><i>{{ r.example }}</i></td>
          </tr>
        </tbody>
      </table>
      <ul class="small tips">
        <li>{{ t('obligationHelp.tipCapitals') }}</li>
        <li>{{ t('obligationHelp.tipOne') }}</li>
        <li>{{ t('obligationHelp.tipConsistent') }}</li>
        <li>{{ t('obligationHelp.tipMeasurable') }}</li>
      </ul>
      <RouterLink :to="guide" class="small">{{ t('obligationHelp.more') }}</RouterLink>
    </div>
  </details>
</template>

<style>
.obligation-help {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 6px 10px;
  background: var(--qtf-card);
}
.obligation-help summary {
  cursor: pointer;
  font-size: 13px;
  color: var(--p-primary-color);
}
.obligation-help .body {
  margin-top: 6px;
  overflow-x: auto;
}
.obligation-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12.5px;
}
.obligation-table th,
.obligation-table td {
  text-align: left;
  vertical-align: top;
  padding: 4px 6px;
  border-top: 1px solid var(--qtf-border);
}
.obligation-table th[scope='row'] {
  white-space: nowrap;
  font-weight: 600;
}
.obligation-help .tips {
  margin: 8px 0;
  padding-left: 18px;
}
</style>
