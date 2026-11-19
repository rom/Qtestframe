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
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate } from '@/lib/format'
import type { User } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'

const auth = useAuthStore()
const notify = useNotify()
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
</script>

<template>
  <div class="card">
    <div class="filterbar">
      <IconField><InputIcon class="pi pi-filter" /><InputText v-model="filter" placeholder="Filter users" /></IconField>
      <span class="grow" />
      <Button label="New user" icon="pi pi-user-plus" @click="openCreate" />
    </div>
    <DataTable :value="filtered" data-key="id" :loading="loading" paginator :rows="50" sort-field="email" :sort-order="1">
      <Column field="email" header="E-mail" sortable />
      <Column field="display_name" header="Name" sortable />
      <Column field="auth_provider" header="Provider" sortable style="width: 120px" />
      <Column header="Flags" style="width: 200px">
        <template #body="{ data }">
          <div class="row" style="gap: 4px">
            <Tag v-if="data.super_admin" value="Super admin" severity="danger" />
            <Tag v-if="data.disabled" value="Disabled" severity="contrast" />
          </div>
        </template>
      </Column>
      <Column field="created_at" header="Created" sortable style="width: 130px"><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column style="width: 60px"><template #body="{ data }"><Button icon="pi pi-pencil" text rounded severity="secondary" aria-label="Edit" @click="openEdit(data)" /></template></Column>
    </DataTable>
  </div>
  <Dialog v-model:visible="dialog" :header="editing ? 'Edit user' : 'New user'" modal style="width: min(460px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="field"><label>E-mail</label><InputText v-model="form.email" type="email" :disabled="!!editing" required /></div>
      <div class="field"><label>Display name</label><InputText v-model="form.display_name" /></div>
      <div class="field">
        <label>{{ editing ? 'Reset password (optional)' : 'Password' }}</label>
        <Password v-model="form.password" toggle-mask fluid autocomplete="new-password" />
      </div>
      <div class="row"><Checkbox v-model="form.super_admin" input-id="sa" binary :disabled="editing?.id === auth.user?.id" /><label for="sa">Super administrator</label></div>
      <div v-if="editing" class="row"><Checkbox v-model="form.disabled" input-id="dis" binary :disabled="editing.id === auth.user?.id" /><label for="dis">Disabled</label></div>
      <div class="row" style="justify-content: flex-end">
        <Button label="Cancel" text severity="secondary" @click="dialog = false" />
        <Button type="submit" label="Save" icon="pi pi-check" :disabled="!form.email || (!editing && !form.password)" />
      </div>
    </form>
  </Dialog>
</template>
