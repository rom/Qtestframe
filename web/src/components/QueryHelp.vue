<script setup lang="ts">
import { QUERY_FIELDS, QUERY_HELP, QUERY_OPERATORS } from '@/lib/query'
defineProps<{ scope: keyof typeof QUERY_FIELDS }>()
</script>

<template>
  <div style="max-width: 560px; font-size: 13px">
    <div style="font-weight: 700; margin-bottom: 6px">Filter query language</div>
    <table style="border-collapse: collapse; width: 100%">
      <tr v-for="h in QUERY_HELP" :key="h.example">
        <td style="padding: 3px 10px 3px 0; white-space: nowrap"><code>{{ h.example }}</code></td>
        <td class="muted" style="padding: 3px 0">{{ h.meaning }}</td>
      </tr>
    </table>
    <div style="font-weight: 700; margin: 10px 0 4px">Operators</div>
    <table style="border-collapse: collapse; width: 100%">
      <tr v-for="h in QUERY_OPERATORS" :key="h.example">
        <td style="padding: 2px 10px 2px 0; white-space: nowrap"><code>{{ h.example }}</code></td>
        <td class="muted">{{ h.meaning }}</td>
      </tr>
    </table>
    <div class="muted small" style="margin-top: 6px">Keywords <code>AND</code>, <code>OR</code>, <code>NOT</code> are case-insensitive. Unknown fields are an error.</div>
    <div style="font-weight: 700; margin: 10px 0 4px">Fields</div>
    <div style="display: flex; flex-wrap: wrap; gap: 4px">
      <code v-for="f in QUERY_FIELDS[scope]" :key="f" class="tag-chip">{{ f }}</code>
    </div>
  </div>
</template>
