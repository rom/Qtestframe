<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Checkbox from 'primevue/checkbox'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import UserAvatar from '@/components/UserAvatar.vue'
import TempPasswordDialog from '@/components/TempPasswordDialog.vue'
import { generatePassword } from '@/lib/signin'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate } from '@/lib/format'
import type { User } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import { useI18n } from 'vue-i18n'
const prefs = usePrefsStore()

const auth = useAuthStore()
const notify = useNotify()
const { t } = useI18n()
const users = ref<User[]>([])
const loading = ref(false)
const filter = ref('')
const filtered = computed(() => {
  const f = filter.value.trim().toLowerCase()
  return f ? users.value.filter((u) => `${u.email} ${u.display_name}`.toLowerCase().includes(f)) : users.value
})

async function load() {
  loading.value = true
  try {
    users.value = asItems(await api.get<{ items: User[] }>('/users'))
  } catch (e) {
    notify.fail(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const dialog = ref(false)
const editing = ref<User | null>(null)
const form = ref({ email: '', display_name: '', password: '', super_admin: false, disabled: false })
const formError = ref('')
function openCreate() {
  editing.value = null
  form.value = { email: '', display_name: '', password: '', super_admin: false, disabled: false }
  formError.value = ''
  dialog.value = true
}
function openEdit(u: User) {
  editing.value = u
  form.value = { email: u.email, display_name: u.display_name, password: '', super_admin: u.super_admin, disabled: u.disabled }
  formError.value = ''
  dialog.value = true
}
async function save() {
  formError.value = ''
  try {
    if (editing.value) {
      await api.patch(`/users/${editing.value.id}`, {
        display_name: form.value.display_name,
        disabled: form.value.disabled,
        super_admin: form.value.super_admin,
        ...(form.value.password ? { password: form.value.password } : {}),
      })
    } else {
      await api.post('/users', { email: form.value.email, display_name: form.value.display_name, password: form.value.password, super_admin: form.value.super_admin })
    }
    dialog.value = false
    load()
  } catch (e) {
    formError.value = errorMessage(e)
  }
}

// ---------- password reset (generated temporary password) ----------
const tempOpen = ref(false)
const tempPassword = ref('')
const tempUser = ref('')
const isLocal = (u: User) => !u.auth_provider || u.auth_provider === 'local'
function resetPassword(u: User) {
  notify.confirmDanger(
    t('admin.confirmResetPassword', { user: u.email }),
    async () => {
      const pw = generatePassword(16)
      try {
        await api.patch(`/users/${u.id}`, { password: pw, must_change_password: true })
        u.must_change_password = true
        tempPassword.value = pw
        tempUser.value = u.email
        tempOpen.value = true
      } catch (e) {
        notify.fail(e, t('admin.resetFailed'))
      }
    },
    t('admin.resetPasswordHeader'),
    t('admin.resetPasswordAction'),
  )
}
</script>

<template>
  <div class="card">
    <div class="filterbar">
      <IconField><InputIcon class="pi pi-filter" /><InputText v-model="filter" :placeholder="t('admin.filterUsers')" :aria-label="t('admin.filterUsers')" /></IconField>
      <span class="grow" />
      <Button :label="t('admin.newUser')" icon="pi pi-user-plus" @click="openCreate" />
    </div>
    <DataTable v-stack :value="filtered" data-key="id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(50)" sort-field="email" :sort-order="1">
      <Column field="email" :header="t('common.email')" sortable body-class="stack-main">
        <template #body="{ data }"><span class="row nowrap" style="gap: 8px; min-width: 0"><UserAvatar :avatar="data.avatar" :email="data.email" :name="data.display_name || data.email" :size="28" /><span class="ellipsis">{{ data.email }}</span></span></template>
      </Column>
      <Column field="display_name" :header="t('common.name')" sortable />
      <Column field="auth_provider" :header="t('admin.provider')" sortable style="width: 130px" />
      <Column :header="t('admin.flags')" style="width: 220px">
        <template #body="{ data }">
          <div class="row" style="gap: 4px">
            <Tag v-if="data.super_admin" :value="t('admin.superAdmin')" severity="danger" />
            <Tag v-if="data.disabled" :value="t('admin.disabled')" severity="contrast" />
            <Tag v-if="data.must_change_password" :value="t('admin.mustChange')" severity="warn" />
          </div>
        </template>
      </Column>
      <Column field="created_at" :header="t('common.created')" sortable style="width: 130px"><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column style="width: 100px"><template #body="{ data }"><Button v-if="isLocal(data) && data.id !== auth.user?.id" icon="pi pi-key" text rounded severity="secondary" :aria-label="t('admin.resetPasswordOf', { user: data.email })" v-tooltip="t('admin.resetPasswordAction')" @click="resetPassword(data)" /><Button icon="pi pi-pencil" text rounded severity="secondary" :aria-label="t('admin.editNamed', { name: data.email })" v-tooltip="t('common.edit')" @click="openEdit(data)" /></template></Column>
    </DataTable>
  </div>
  <Dialog v-model:visible="dialog" :header="editing ? t('admin.editUser') : t('admin.newUser')" modal style="width: min(460px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div v-field class="field"><label>{{ t('common.email') }}</label><InputText v-model="form.email" type="email" :disabled="!!editing" required /></div>
      <div v-field class="field"><label>{{ t('admin.displayName') }}</label><InputText v-model="form.display_name" /></div>
      <div v-field class="field">
        <label>{{ editing ? t('admin.resetPassword') : t('admin.password') }}</label>
        <Password v-model="form.password" toggle-mask fluid autocomplete="new-password" />
      </div>
      <div class="row"><Checkbox v-model="form.super_admin" input-id="sa" binary :disabled="editing?.id === auth.user?.id" /><label for="sa">{{ t('admin.superAdministrator') }}</label></div>
      <div v-if="editing" class="row"><Checkbox v-model="form.disabled" input-id="dis" binary :disabled="editing.id === auth.user?.id" /><label for="dis">{{ t('admin.disabled') }}</label></div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :disabled="!form.email || (!editing && !form.password)" />
      </div>
    </form>
  </Dialog>
  <TempPasswordDialog v-model:visible="tempOpen" :password="tempPassword" :user="tempUser" />
</template>
