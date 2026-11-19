<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Tree from 'primevue/tree'
import Button from 'primevue/button'
import { allKeys, buildTree, folderPath } from '@/lib/tree'
import type { Folder } from '@/lib/types'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ folders: Folder[]; header?: string; allowRoot?: boolean }>()
const emit = defineEmits<{ pick: [folderId: string | null] }>()

const nodes = computed(() => buildTree(props.folders))
const expanded = ref<Record<string, boolean>>({})
const choice = ref<string | null | undefined>(undefined)
watch(visible, (v) => {
  if (v) {
    expanded.value = allKeys(nodes.value)
    choice.value = undefined
  }
})
const keys = computed(() => (choice.value ? { [choice.value]: true } : {}))
function confirm() {
  if (choice.value === undefined) return
  emit('pick', choice.value)
  visible.value = false
}
</script>

<template>
  <Dialog v-model:visible="visible" :header="header ?? 'Choose folder'" modal style="width: min(460px, 95vw)">
    <div v-if="allowRoot !== false" class="tree-root" :class="{ active: choice === null }" @click="choice = null">
      <i class="pi pi-inbox" /> Top level (no folder)
    </div>
    <Tree
      v-model:expanded-keys="expanded"
      :value="nodes"
      selection-mode="single"
      :selection-keys="keys"
      style="max-height: 50vh; overflow: auto; padding: 0; border: none"
      @node-select="(n) => (choice = String(n.key))"
    />
    <div class="small muted" style="margin-top: 8px">
      Target: <b>{{ choice === undefined ? '—' : choice === null ? 'Top level' : folderPath(folders, choice) }}</b>
    </div>
    <template #footer>
      <Button label="Cancel" text severity="secondary" @click="visible = false" />
      <Button label="Select" icon="pi pi-check" :disabled="choice === undefined" @click="confirm" />
    </template>
  </Dialog>
</template>
