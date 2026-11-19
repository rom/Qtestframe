<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate, humanize, options } from '@/lib/format'
import { P } from '@/lib/permissions'
import type { Membership, RbacInfo } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
const prefs = usePrefsStore()

const auth = useAuthStore()
const meta = useMetaStore()
const notify = useNotify()
const members = ref<Membership[]>([])
const rbac = ref<RbacInfo | null>(null)
const loading = ref(false)
const filter = ref('')
const canManage = computed(() => auth.can(P.memberManage))
const roles = computed(() => options(rbac.value?.roles ?? meta.meta.roles))
const filtered = computed(() => {
  const f = filter.value.trim().toLowerCase()
  return f ? members.value.filter((m) => `${m.email} ${m.display_name} ${m.role}`.toLowerCase().includes(f)) : members.value
})

async function load() {
  loading.value = true
  try {
    members.value = asItems(await api.get<{ items: Membership[] }>('/members'))
  } catch (e) {
    notify.fail(e)
  } finally {
    loading.value = false
  }
}
onMounted(async () => {
  load()
  try {
    rbac.value = await api.get<RbacInfo>('/rbac')
  } catch {
    /* optional */
  }
})

async function setRole(m: Membership, role: string) {
  try {
    await api.patch(`/members/${m.user_id}`, { role })
    m.role = role
    notify.ok('Role changed', `${m.email}: ${humanize(role)}`)
  } catch (e) {
    notify.fail(e)
  }
}
function remove(m: Membership) {
  notify.confirmDanger(`Remove ${m.email} from ${auth.tenant?.name}?`, async () => {
    try {
      await api.del(`/members/${m.user_id}`)
      load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

const dialog = ref(false)
const form = ref({ email: '', display_name: '', role: 'tester', password: '' })
const formError = ref('')
const busy = ref(false)
function openAdd() {
  form.value = { email: '', display_name: '', role: 'tester', password: '' }
  formError.value = ''
  dialog.value = true
}
async function add() {
  busy.value = true
  formError.value = ''
  try {
    await api.post('/members', {
      email: form.value.email.trim(),
      role: form.value.role,
      ...(form.value.display_name ? { display_name: form.value.display_name } : {}),
      ...(form.value.password ? { password: form.value.password } : {}),
    })
    dialog.value = false
    notify.ok('Member added', form.value.email)
    load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
const showMatrix = ref(false)
const allPerms = computed(() => [...new Set(Object.values(rbac.value?.matrix ?? {}).flat())].sort())
</script>

<template>
  <div class="card">
    <div class="filterbar">
      <IconField><InputIcon class="pi pi-filter" /><InputText v-model="filter" placeholder="Filter members" /></IconField>
      <span class="grow" />
      <Button v-if="rbac" label="Role matrix" icon="pi pi-table" severity="secondary" text @click="showMatrix = true" />
      <Button v-if="canManage" label="Add member" icon="pi pi-user-plus" @click="openAdd" />
    </div>
    <DataTable :value="filtered" data-key="user_id" :loading="loading" paginator :rows="prefs.rowsFor(50)" sort-field="email" :sort-order="1">
      <Column field="email" header="E-mail" sortable />
      <Column field="display_name" header="Name" sortable />
      <Column field="role" header="Role" sortable style="width: 200px">
        <template #body="{ data }">
          <Select
            v-if="canManage && data.user_id !== auth.user?.id"
            :model-value="data.role"
            :options="roles"
            option-label="label"
            option-value="value"
            size="small"
            class="w-full"
            @update:model-value="(r: string) => setRole(data, r)"
          />
          <Tag v-else :value="humanize(data.role)" severity="secondary" />
        </template>
      </Column>
      <Column field="created_at" header="Member since" sortable style="width: 140px"><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column v-if="canManage" style="width: 60px">
        <template #body="{ data }">
          <Button v-if="data.user_id !== auth.user?.id" icon="pi pi-user-minus" text rounded severity="danger" aria-label="Remove" @click="remove(data)" />
        </template>
      </Column>
    </DataTable>
  </div>

  <Dialog v-model:visible="dialog" header="Add member" modal style="width: min(460px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="add">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="field"><label>E-mail</label><InputText v-model="form.email" type="email" required autofocus /></div>
      <div class="field"><label>Role</label><Select v-model="form.role" :options="roles" option-label="label" option-value="value" /></div>
      <div class="field"><label>Display name (new users)</label><InputText v-model="form.display_name" /></div>
      <div class="field">
        <label>Initial password (new local users)</label>
        <Password v-model="form.password" toggle-mask fluid autocomplete="new-password" />
        <small class="muted">Existing users are added with the chosen role; a local account is created if the e-mail is unknown.</small>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button label="Cancel" text severity="secondary" @click="dialog = false" />
        <Button type="submit" label="Add" icon="pi pi-check" :loading="busy" :disabled="!form.email" />
      </div>
    </form>
  </Dialog>

  <Dialog v-model:visible="showMatrix" header="Roles and permissions" modal style="width: min(720px, 96vw)">
    <table v-if="rbac" style="width: 100%; border-collapse: collapse; font-size: 13px">
      <thead>
        <tr>
          <th style="text-align: left; padding: 6px">Permission</th>
          <th v-for="r in rbac.roles" :key="r" style="padding: 6px">{{ humanize(r) }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="p in allPerms" :key="p" style="border-top: 1px solid var(--qtf-border)">
          <td style="padding: 5px 6px" class="mono">{{ p }}</td>
          <td v-for="r in rbac.roles" :key="r" style="text-align: center">
            <i v-if="rbac.matrix[r]?.includes(p)" class="pi pi-check" style="color: var(--p-green-500)" />
          </td>
        </tr>
      </tbody>
    </table>
  </Dialog>
</template>
