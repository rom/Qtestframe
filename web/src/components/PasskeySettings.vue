<script setup lang="ts">
// Profile: the user's passkeys — add (navigator.credentials.create), rename
// and delete. Adding and deleting need a recent sign-in; otherwise the
// user confirms their identity first (ReauthDialog).
import { computed, onMounted, ref } from 'vue'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ReauthDialog from './ReauthDialog.vue'
import { api, errorMessage } from '@/lib/api'
import { createPasskey, passkeyErrorKind, passkeySupport, suggestName, type CreationOptionsJSON, type Passkey, type PasskeyList } from '@/lib/passkeys'
import { formatDate, formatDateTime } from '@/lib/format'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = withDefaults(defineProps<{ /** inside the forced MFA set-up: no card, the add button only */ compact?: boolean }>(), { compact: false })
const emit = defineEmits<{ added: [Passkey] }>()
const auth = useAuthStore()
const notify = useNotify()
const { t } = useI18n()

const list = ref<PasskeyList | null>(null)
const loadError = ref('')
const support = passkeySupport()
async function load() {
  loadError.value = ''
  try {
    list.value = await api.get<PasskeyList>('/auth/passkeys')
  } catch (e) {
    loadError.value = errorMessage(e)
  }
}
onMounted(load)
const usable = computed(() => !!list.value?.available && support === 'ok')
const announce = ref('')

function passkeyError(e: unknown): string {
  const kind = passkeyErrorKind(e)
  return kind ? t(`passkeys.error.${kind}`) : errorMessage(e)
}

// ---------- re-authentication: retry the action once confirmed ----------
const reauthOpen = ref(false)
let pending: (() => Promise<void>) | null = null
async function guarded(action: () => Promise<void>, onError: (e: unknown) => void) {
  try {
    await action()
  } catch (e) {
    if (passkeyErrorKind(e) === 'reauth') {
      pending = action
      reauthOpen.value = true
      return
    }
    onError(e)
  }
}
async function reauthed() {
  const action = pending
  pending = null
  await load()
  if (action) await action().catch((e) => notify.fail(e, passkeyError(e)))
}

// ---------- add ----------
const addOpen = ref(false)
const name = ref('')
const addError = ref('')
const adding = ref(false)
function openAdd() {
  name.value = suggestName()
  addError.value = ''
  addOpen.value = true
}
async function register() {
  const opts = await api.post<CreationOptionsJSON>('/auth/passkeys/register/options')
  const credential = await createPasskey(opts)
  const pk = await api.post<Passkey>('/auth/passkeys/register', { name: name.value.trim(), credential })
  addOpen.value = false
  notify.ok(t('passkeys.added'), pk.name)
  announce.value = t('passkeys.added')
  emit('added', pk)
  await load()
  void auth.fetchMe()
}
async function add() {
  addError.value = ''
  adding.value = true
  try {
    await guarded(register, (e) => (addError.value = passkeyError(e)))
  } finally {
    adding.value = false
  }
}

// ---------- rename ----------
const renaming = ref<Passkey | null>(null)
const newName = ref('')
const renameError = ref('')
function openRename(pk: Passkey) {
  renaming.value = pk
  newName.value = pk.name
  renameError.value = ''
}
async function rename() {
  if (!renaming.value || !newName.value.trim()) return
  try {
    await api.patch(`/auth/passkeys/${renaming.value.id}`, { name: newName.value.trim() })
    renaming.value = null
    notify.ok(t('passkeys.renamed'))
    announce.value = t('passkeys.renamed')
    await load()
  } catch (e) {
    renameError.value = errorMessage(e)
  }
}

// ---------- delete ----------
function remove(pk: Passkey) {
  notify.confirmDanger(
    t('passkeys.deleteConfirm', { name: pk.name }),
    () =>
      guarded(
        async () => {
          await api.del(`/auth/passkeys/${pk.id}`)
          notify.ok(t('passkeys.deleted'), pk.name)
          announce.value = t('passkeys.deleted')
          await load()
          void auth.fetchMe()
        },
        (e) => notify.fail(e, passkeyError(e)),
      ),
    t('passkeys.deleteHeader'),
    t('passkeys.delete'),
  )
}
</script>

<template>
  <component :is="props.compact ? 'div' : 'section'" :id="props.compact ? undefined : 'passkeys'" :class="{ card: !props.compact }" :aria-labelledby="props.compact ? undefined : 'h-passkeys'" :style="props.compact ? undefined : 'grid-column: 1 / -1'">
    <div v-if="!props.compact" class="row" style="margin-bottom: 10px">
      <h2 id="h-passkeys" style="margin: 0"><i class="pi pi-key" aria-hidden="true" /> {{ t('passkeys.title') }}</h2>
      <Tag v-if="list && list.items.length" :value="t('passkeys.count', { n: list.items.length }, list.items.length)" severity="secondary" />
    </div>
    <Message v-if="loadError" severity="error" :closable="false">{{ loadError }}</Message>
    <template v-else-if="list">
      <p v-if="!props.compact" class="small muted" style="margin: 0 0 12px">{{ list.passwordless ? t('passkeys.intro') : t('passkeys.introSecondFactor') }}</p>
      <Message v-if="!list.enabled" severity="info" :closable="false" style="margin-bottom: 12px">{{ t('passkeys.disabled') }}</Message>
      <Message v-else-if="!list.available" severity="warn" :closable="false" style="margin-bottom: 12px">{{ t('passkeys.unavailable') }}</Message>
      <Message v-else-if="support !== 'ok'" id="passkey-support" severity="warn" :closable="false" style="margin-bottom: 12px">{{ t(`passkeys.error.${support}`) }}</Message>
      <template v-if="!props.compact">
        <ul v-if="list.items.length" class="plain-list passkey-list" :aria-label="t('passkeys.listLabel')">
          <li v-for="pk in list.items" :key="pk.id" class="passkey">
            <i class="pi pi-key muted" aria-hidden="true" />
            <div class="grow" style="min-width: 0">
              <div class="row nowrap" style="gap: 6px">
                <b class="ellipsis">{{ pk.name }}</b>
                <Tag v-if="pk.backed_up" :value="t('passkeys.synced')" severity="info" icon="pi pi-cloud" v-tooltip="t('passkeys.syncedHint')" />
              </div>
              <div class="small muted">
                {{ t('passkeys.created') }} {{ formatDate(pk.created_at) }} ·
                <template v-if="pk.last_used_at">{{ t('passkeys.lastUsed') }} {{ formatDateTime(pk.last_used_at) }}</template>
                <template v-else>{{ t('passkeys.never') }}</template>
              </div>
            </div>
            <Button icon="pi pi-pencil" text rounded severity="secondary" :aria-label="t('passkeys.renameOf', { name: pk.name })" v-tooltip="t('passkeys.rename')" @click="openRename(pk)" />
            <Button icon="pi pi-trash" text rounded severity="danger" :aria-label="t('passkeys.deleteOf', { name: pk.name })" v-tooltip="t('passkeys.delete')" @click="remove(pk)" />
          </li>
        </ul>
        <p v-else class="small muted" style="margin: 0 0 12px">{{ t('passkeys.none') }}</p>
      </template>
      <Button v-if="list.enabled" :label="t('passkeys.add')" icon="pi pi-plus" :severity="props.compact ? 'secondary' : undefined" :outlined="props.compact" :disabled="!usable" :aria-describedby="support !== 'ok' ? 'passkey-support' : undefined" @click="openAdd" />
    </template>

    <Dialog :visible="addOpen" modal :header="t('passkeys.addTitle')" style="width: min(440px, 96vw)" @update:visible="(v: boolean) => !v && (addOpen = false)">
      <form id="passkey-add" class="grid" style="gap: 12px" @submit.prevent="add">
        <p class="small" style="margin: 0">{{ t('passkeys.addText') }}</p>
        <Message v-if="addError" id="passkey-add-error" severity="error" :closable="false">{{ addError }}</Message>
        <div v-field class="field">
          <label for="passkey-name">{{ t('passkeys.name') }}</label>
          <InputText id="passkey-name" v-model="name" maxlength="100" :placeholder="t('passkeys.namePlaceholder')" autofocus :aria-describedby="addError ? 'passkey-add-error' : undefined" />
        </div>
      </form>
      <template #footer>
        <Button :label="t('common.cancel')" text severity="secondary" @click="addOpen = false" />
        <Button type="submit" form="passkey-add" :label="t('passkeys.add')" icon="pi pi-key" :loading="adding" />
      </template>
    </Dialog>

    <Dialog :visible="!!renaming" modal :header="t('passkeys.renameTitle')" style="width: min(440px, 96vw)" @update:visible="(v: boolean) => !v && (renaming = null)">
      <form id="passkey-rename" class="grid" style="gap: 12px" @submit.prevent="rename">
        <Message v-if="renameError" id="passkey-rename-error" severity="error" :closable="false">{{ renameError }}</Message>
        <div v-field class="field">
          <label for="passkey-new-name">{{ t('passkeys.name') }}</label>
          <InputText id="passkey-new-name" v-model="newName" maxlength="100" required autofocus :aria-describedby="renameError ? 'passkey-rename-error' : undefined" />
        </div>
      </form>
      <template #footer>
        <Button :label="t('common.cancel')" text severity="secondary" @click="renaming = null" />
        <Button type="submit" form="passkey-rename" :label="t('common.save')" icon="pi pi-check" :disabled="!newName.trim()" />
      </template>
    </Dialog>

    <ReauthDialog v-model:visible="reauthOpen" @confirmed="reauthed" />
    <div class="sr-only" role="status" aria-live="polite">{{ announce }}</div>
  </component>
</template>

<style scoped>
.passkey-list {
  display: grid;
  gap: 4px;
  margin: 0 0 12px;
}
.passkey {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 4px;
  border-bottom: 1px solid var(--p-content-border-color);
}
.passkey:last-child {
  border-bottom: 0;
}
</style>
