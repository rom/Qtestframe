<script setup lang="ts">
import { QUERY_FIELDS, QUERY_HELP, QUERY_OPERATORS } from '@/lib/query'
import { useI18n } from 'vue-i18n'
import { queryGuidePath } from '@/guide/tips'
defineProps<{ scope: keyof typeof QUERY_FIELDS }>()
const { t, locale } = useI18n()
</script>

<template>
  <div style="max-width: min(560px, calc(100vw - 48px)); overflow-x: auto; font-size: 13px">
    <div style="font-weight: 700; margin-bottom: 6px">{{ t('query.title') }}</div>
    <table style="border-collapse: collapse; width: 100%">
      <tr v-for="h in QUERY_HELP" :key="h.example">
        <th scope="row" style="padding: 3px 10px 3px 0; white-space: nowrap; text-align: left; font-weight: normal"><code>{{ h.example }}</code></th>
        <td class="muted" style="padding: 3px 0">{{ t('query.meaning.' + h.id) }}</td>
      </tr>
    </table>
    <div style="font-weight: 700; margin: 10px 0 4px">{{ t('query.operators') }}</div>
    <table style="border-collapse: collapse; width: 100%">
      <tr v-for="h in QUERY_OPERATORS" :key="h.example">
        <th scope="row" style="padding: 2px 10px 2px 0; white-space: nowrap; text-align: left; font-weight: normal"><code>{{ h.example }}</code></th>
        <td class="muted">{{ t('query.meaning.' + h.id) }}</td>
      </tr>
    </table>
    <i18n-t keypath="query.keywordsHint" tag="div" class="muted small" style="margin-top: 6px">
      <template #and><code>AND</code></template>
      <template #or><code>OR</code></template>
      <template #not><code>NOT</code></template>
    </i18n-t>
    <div style="font-weight: 700; margin: 10px 0 4px">{{ t('query.fields') }}</div>
    <div style="display: flex; flex-wrap: wrap; gap: 4px">
      <code v-for="f in QUERY_FIELDS[scope]" :key="f" class="tag-chip">{{ f }}</code>
    </div>
    <RouterLink :to="queryGuidePath(String(locale))" class="row nowrap" style="gap: 6px; margin-top: 10px; font-weight: 600">
      <i class="pi pi-book" aria-hidden="true" />{{ t('guide.queryGuide') }}
    </RouterLink>
  </div>
</template>
