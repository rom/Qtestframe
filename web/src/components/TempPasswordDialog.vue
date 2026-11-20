<script setup lang="ts">
// Shows a temporary password once, with a copy button.
import { ref, watch } from 'vue'
import Dialog from 'primevue/dialog'
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useI18n } from 'vue-i18n'
import { uid } from '@/lib/a11y'

const visible = defineModel<boolean>('visible', { default: false })
defineProps<{ password: string; user: string }>()
const { t } = useI18n()
const id = uid('tmp-pw')
const copied = ref(false)
watch(visible, () => (copied.value = false))
async function copy(pw: string) {
  try {
    await navigator.clipboard.writeText(pw)
    copied.value = true
  } catch {
    // select it so it can be copied by hand
    const el = document.getElementById(id)
    if (el) window.getSelection()?.selectAllChildren(el)
  }
}
</script>

<template>
  <Dialog v-model:visible="visible" modal :header="t('admin.tempPasswordTitle')" :style="{ width: 'min(480px, calc(100vw - 16px))' }" :closable="true">
    <p>{{ t('admin.tempPasswordFor', { user }) }}</p>
    <div class="row nowrap" style="gap: 8px">
      <code :id="id" class="temp-pw" :aria-label="t('admin.tempPassword')">{{ password }}</code>
      <Button :label="copied ? t('common.copied') : t('common.copy')" :icon="copied ? 'pi pi-check' : 'pi pi-copy'" severity="secondary" @click="copy(password)" />
    </div>
    <div class="sr-only" role="status" aria-live="polite">{{ copied ? t('common.copied') : '' }}</div>
    <Message severity="warn" :closable="false" style="margin-top: 12px">{{ t('admin.tempPasswordHint') }}</Message>
    <template #footer><Button :label="t('admin.tempPasswordDone')" @click="visible = false" /></template>
  </Dialog>
</template>

<style scoped>
.temp-pw {
  flex: 1;
  min-width: 0;
  padding: 8px 10px;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  font-size: 16px;
  word-break: break-all;
  user-select: all;
  background: var(--qtf-bg);
}
</style>
