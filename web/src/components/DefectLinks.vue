<script setup lang="ts">
import { computed } from 'vue'
import { defectLinks } from '@/lib/defects'
import { useProjectStore } from '@/stores/project'

const props = defineProps<{ defects?: string[] | null; template?: string | null; empty?: string }>()
const proj = useProjectStore()
const links = computed(() => defectLinks(props.defects, props.template ?? proj.project?.defect_url_template))
</script>

<template>
  <span v-if="links.length" class="row" style="gap: 0">
    <template v-for="d in links" :key="d.value">
      <a v-if="d.url" :href="d.url" target="_blank" rel="noopener noreferrer" class="defect-link" :title="d.value" @click.stop>
        <i class="pi pi-exclamation-circle" style="font-size: 10px" />{{ d.label }}
      </a>
      <span v-else class="defect-link plain" :title="d.value"><i class="pi pi-exclamation-circle" style="font-size: 10px" />{{ d.label }}</span>
    </template>
  </span>
  <span v-else-if="empty !== undefined" class="muted small">{{ empty }}</span>
</template>
