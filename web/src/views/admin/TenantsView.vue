<script setup lang="ts">
import { onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate } from '@/lib/format'
import type { Tenant } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'

const auth = useAuthStore()
const notify = useNotify()
const tenants = ref<Tenant[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    tenants.value = asItems(await api.get<{ items: Tenant[] }>('/tenants'))
  } catch (e) {
    notify.fail(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const dialog = ref(false)
const editing = ref<Tenant | null>(null)
const form = ref({ slug: '', name: '' })
const formError = ref('')
function openCreate() {
  editing.value = null
  form.value = { slug: '', name: '' }
  formError.value = ''
  dialog.value = true
}
function openEdit(t: Tenant) {
  editing.value = t
  form.value = { slug: t.slug, name: t.name }
  formError.value = ''
  dialog.value = true
}
async function save() {
  formError.value = ''
  try {
    if (editing.value) await api.patch(`/tenants/${editing.value.id}`, { name: form.value.name })
    else await api.post('/tenants', form.value)
    dialog.value = false
    load()
  } catch (e) {
    formError.value = errorMessage(e)
  }
}
function remove(t: Tenant) {
  notify.confirmDanger(`Delete tenant "${t.name}" (${t.slug}) with ALL projects and data?`, async () => {
    try {
      await api.del(`/tenants/${t.id}`)
      load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
async function switchTo(t: Tenant) {
  try {
    await auth.switchTenant(t.id)
    notify.ok('Switched tenant', t.name)
  } catch (e) {
    notify.fail(e)
  }
}
</script>

<template>
  <div class="card">
    <div class="filterbar">
      <span class="muted small">Tenants are isolated organisations. Only super administrators manage them.</span>
      <span class="grow" />
      <Button label="New tenant" icon="pi pi-plus" @click="openCreate" />
    </div>
    <DataTable :value="tenants" data-key="id" :loading="loading" sort-field="slug" :sort-order="1">
      <Column field="slug" header="Slug" sortable><template #body="{ data }"><span class="mono">{{ data.slug }}</span></template></Column>
      <Column field="name" header="Name" sortable />
      <Column field="created_at" header="Created" sortable><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column style="width: 170px">
        <template #body="{ data }">
          <div class="row nowrap" style="gap: 0">
            <Button v-if="auth.tenant?.id !== data.id" icon="pi pi-sign-in" text rounded v-tooltip="'Switch to tenant'" aria-label="Switch" @click="switchTo(data)" />
            <Button icon="pi pi-pencil" text rounded severity="secondary" aria-label="Edit" @click="openEdit(data)" />
            <Button icon="pi pi-trash" text rounded severity="danger" aria-label="Delete" @click="remove(data)" />
          </div>
        </template>
      </Column>
    </DataTable>
  </div>
  <Dialog v-model:visible="dialog" :header="editing ? 'Rename tenant' : 'New tenant'" modal style="width: min(440px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="field"><label>Slug</label><InputText v-model="form.slug" :disabled="!!editing" placeholder="acme" required /></div>
      <div class="field"><label>Name</label><InputText v-model="form.name" placeholder="ACME Corp." required /></div>
      <div class="row" style="justify-content: flex-end">
        <Button label="Cancel" text severity="secondary" @click="dialog = false" />
        <Button type="submit" label="Save" icon="pi pi-check" :disabled="!form.slug || !form.name" />
      </div>
    </form>
  </Dialog>
</template>
