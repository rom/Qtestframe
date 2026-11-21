<script setup lang="ts">
// Recovery codes, shown once after they were created: copy, download.
import Button from 'primevue/button'
import Message from 'primevue/message'
import { useI18n } from 'vue-i18n'
import { saveBlob } from '@/lib/download'
import { recoveryCodesFile } from '@/lib/mfa'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ codes: string[]; account: string }>()
const { t } = useI18n()
const notify = useNotify()

async function copy() {
  try {
    await navigator.clipboard.writeText(props.codes.join('\n'))
    notify.ok(t('mfa.codesCopied'))
  } catch (e) {
    notify.fail(e)
  }
}
function download() {
  const text = recoveryCodesFile(props.codes, props.account, t('mfa.codesFileHeading'), new Date())
  saveBlob(new Blob([text], { type: 'text/plain;charset=utf-8' }), 'qtestframe-recovery-codes.txt')
}
</script>

<template>
  <div class="recovery-codes">
    <Message severity="warn" :closable="false">{{ t('mfa.codesOnce') }}</Message>
    <p class="small muted" style="margin: 10px 0 6px">{{ t('mfa.codesExplain') }}</p>
    <ol class="code-list mono" :aria-label="t('mfa.recoveryCodes')">
      <li v-for="c in codes" :key="c">{{ c }}</li>
    </ol>
    <div class="row" style="gap: 8px">
      <Button :label="t('common.copy')" icon="pi pi-copy" size="small" severity="secondary" outlined @click="copy" />
      <Button :label="t('mfa.download')" icon="pi pi-download" size="small" severity="secondary" outlined @click="download" />
    </div>
  </div>
</template>

<style scoped>
.code-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(16ch, 1fr));
  gap: 6px 18px;
  padding: 10px 14px 10px 34px;
  margin: 0 0 12px;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  font-size: 1.05em;
}
</style>
