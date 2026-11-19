<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Tree from 'primevue/tree'
import Button from 'primevue/button'
import { allKeys, buildTree, folderPath } from '@/lib/tree'
import type { Folder } from '@/lib/types'
import { useI18n } from 'vue-i18n'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ folders: Folder[]; header?: string; allowRoot?: boolean }>()
const emit = defineEmits<{ pick: [folderId: string | null] }>()
const { t } = useI18n()

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
  <Dialog v-model:visible="visible" :header="header ?? t('folders.chooseFolder')" modal style="width: min(460px, 95vw)">
    <button v-if="allowRoot !== false" type="button" class="tree-root" :class="{ active: choice === null }" :aria-pressed="choice === null" @click="choice = null">
      <i class="pi pi-inbox" aria-hidden="true" /> {{ t('folders.topLevelNoFolder') }}
    </button>
    <Tree
      v-model:expanded-keys="expanded"
      :value="nodes"
      selection-mode="single"
      :selection-keys="keys"
      :aria-label="header ?? t('folders.chooseFolder')"
      style="max-height: 50vh; overflow: auto; padding: 0; border: none"
      @node-select="(n) => (choice = String(n.key))"
    />
    <div class="small muted" style="margin-top: 8px" aria-live="polite">
      {{ t('folders.target') }} <b>{{ choice === undefined ? '—' : choice === null ? t('common.topLevel') : folderPath(folders, choice) }}</b>
    </div>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="visible = false" />
      <Button :label="t('folders.select')" icon="pi pi-check" :disabled="choice === undefined" @click="confirm" />
    </template>
  </Dialog>
</template>
