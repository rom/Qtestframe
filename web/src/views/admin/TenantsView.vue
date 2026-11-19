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
import { useI18n } from 'vue-i18n'

const auth = useAuthStore()
const notify = useNotify()
const { t } = useI18n()
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
function remove(tn: Tenant) {
  notify.confirmDanger(t('admin.confirmDeleteTenant', { name: tn.name, slug: tn.slug }), async () => {
    try {
      await api.del(`/tenants/${tn.id}`)
      load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
async function switchTo(tn: Tenant) {
  try {
    await auth.switchTenant(tn.id)
    notify.ok(t('admin.switchedTenant'), tn.name)
  } catch (e) {
    notify.fail(e)
  }
}
</script>

<template>
  <div class="card">
    <div class="filterbar">
      <span class="muted small">{{ t('admin.tenantsHint') }}</span>
      <span class="grow" />
      <Button :label="t('admin.newTenant')" icon="pi pi-plus" @click="openCreate" />
    </div>
    <DataTable :value="tenants" data-key="id" :loading="loading" sort-field="slug" :sort-order="1">
      <Column field="slug" :header="t('admin.slug')" sortable><template #body="{ data }"><span class="mono">{{ data.slug }}</span></template></Column>
      <Column field="name" :header="t('common.name')" sortable />
      <Column field="created_at" :header="t('common.created')" sortable><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column style="width: 170px">
        <template #body="{ data }">
          <div class="row nowrap" style="gap: 0">
            <Button v-if="auth.tenant?.id !== data.id" icon="pi pi-sign-in" text rounded v-tooltip="t('admin.switchToTenant')" :aria-label="t('admin.switchToTenant')" @click="switchTo(data)" />
            <Button icon="pi pi-pencil" text rounded severity="secondary" :aria-label="t('common.edit')" v-tooltip="t('common.edit')" @click="openEdit(data)" />
            <Button icon="pi pi-trash" text rounded severity="danger" :aria-label="t('common.delete')" v-tooltip="t('common.delete')" @click="remove(data)" />
          </div>
        </template>
      </Column>
    </DataTable>
  </div>
  <Dialog v-model:visible="dialog" :header="editing ? t('admin.renameTenant') : t('admin.newTenant')" modal style="width: min(440px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="field"><label>{{ t('admin.slug') }}</label><InputText v-model="form.slug" :disabled="!!editing" placeholder="acme" required /></div>
      <div class="field"><label>{{ t('common.name') }}</label><InputText v-model="form.name" :placeholder="t('admin.tenantNamePlaceholder')" required /></div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :disabled="!form.slug || !form.name" />
      </div>
    </form>
  </Dialog>
</template>
