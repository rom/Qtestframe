<script setup lang="ts">
import HelpTip from '@/guide/HelpTip.vue'
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
import UserAvatar from '@/components/UserAvatar.vue'
import TempPasswordDialog from '@/components/TempPasswordDialog.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate } from '@/lib/format'
import { P } from '@/lib/permissions'
import type { Membership } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import { useI18n } from 'vue-i18n'
import { enumLabel, enumOptions } from '@/i18n/labels'
const prefs = usePrefsStore()

const auth = useAuthStore()
const meta = useMetaStore()
const notify = useNotify()
const { t } = useI18n()
const members = ref<Membership[]>([])
const rbac = ref<{ roles: string[] } | null>(null)
const loading = ref(false)
const filter = ref('')
const canManage = computed(() => auth.can(P.memberManage))
const roles = computed(() => enumOptions('role', rbac.value?.roles ?? meta.meta.roles))
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
    rbac.value = await api.get<{ roles: string[] }>('/rbac')
  } catch {
    /* optional */
  }
})

async function setRole(m: Membership, role: string) {
  try {
    await api.patch(`/members/${m.user_id}`, { role })
    m.role = role
    notify.ok(t('admin.roleChanged'), `${m.email}: ${enumLabel('role', role)}`)
  } catch (e) {
    notify.fail(e)
  }
}
function remove(m: Membership) {
  notify.confirmDanger(t('admin.confirmRemoveMember', { email: m.email, tenant: auth.tenant?.name ?? '' }), async () => {
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
    notify.ok(t('admin.memberAdded'), form.value.email)
    load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
// ---------- password reset ----------
const tempOpen = ref(false)
const tempPassword = ref('')
const tempUser = ref('')
function resetPassword(m: Membership) {
  notify.confirmDanger(
    t('admin.confirmResetPassword', { user: m.email ?? '' }),
    async () => {
      try {
        const res = await api.post<{ temporary_password: string }>(`/members/${m.user_id}/reset-password`)
        tempPassword.value = res?.temporary_password ?? ''
        tempUser.value = m.email ?? ''
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
      <IconField><InputIcon class="pi pi-filter" /><InputText v-model="filter" :placeholder="t('admin.filterMembers')" :aria-label="t('admin.filterMembers')" /></IconField>
      <span class="grow" />
      <RouterLink :to="{ name: 'rbac' }" class="p-button p-button-text p-button-secondary"><i class="pi pi-table" aria-hidden="true" />&nbsp;{{ t('admin.roleMatrix') }}</RouterLink>
      <Button v-if="canManage" :label="t('admin.addMember')" icon="pi pi-user-plus" @click="openAdd" />
    </div>
    <DataTable v-stack :value="filtered" data-key="user_id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(50)" sort-field="email" :sort-order="1">
      <Column field="email" :header="t('common.email')" sortable body-class="stack-main">
        <template #body="{ data }"><span class="row nowrap" style="gap: 8px; min-width: 0"><UserAvatar :avatar="data.avatar" :user-id="data.user_id" :email="data.email" :name="data.display_name || data.email" :size="28" /><span class="ellipsis">{{ data.email }}</span></span></template>
      </Column>
      <Column field="display_name" :header="t('common.name')" sortable />
      <Column field="role" :header="t('common.role')" sortable style="width: 200px">
        <template #body="{ data }">
          <Select
            v-if="canManage && data.user_id !== auth.user?.id"
            :model-value="data.role"
            :options="roles"
            option-label="label"
            option-value="value"
            size="small"
            class="w-full"
            :aria-label="t('admin.roleOf', { user: data.email })"
            @update:model-value="(r: string) => setRole(data, r)"
          />
          <Tag v-else :value="enumLabel('role', data.role)" severity="secondary" />
        </template>
      </Column>
      <Column field="created_at" :header="t('admin.memberSince')" sortable style="width: 150px"><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column v-if="canManage" style="width: 100px">
        <template #body="{ data }">
          <Button v-if="data.user_id !== auth.user?.id" icon="pi pi-key" text rounded severity="secondary" :aria-label="t('admin.resetPasswordOf', { user: data.email })" v-tooltip="t('admin.resetPasswordAction')" @click="resetPassword(data)" />
          <Button v-if="data.user_id !== auth.user?.id" icon="pi pi-user-minus" text rounded severity="danger" :aria-label="t('admin.removeMember', { user: data.email })" v-tooltip="t('common.remove')" @click="remove(data)" />
        </template>
      </Column>
    </DataTable>
  </div>

  <Dialog v-model:visible="dialog" :header="t('admin.addMember')" modal style="width: min(460px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="add">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div v-field class="field"><label>{{ t('common.email') }}</label><InputText v-model="form.email" type="email" required autofocus /></div>
      <div v-field class="field"><label>{{ t('common.role') }}</label><HelpTip id="member.role" :label="t('common.role')" /><Select v-model="form.role" :options="roles" option-label="label" option-value="value" /></div>
      <div v-field class="field"><label>{{ t('admin.displayNameNew') }}</label><InputText v-model="form.display_name" /></div>
      <div v-field class="field">
        <label>{{ t('admin.initialPassword') }}</label>
        <HelpTip id="member.initialPassword" :label="t('admin.initialPassword')" />
        <Password v-model="form.password" toggle-mask fluid autocomplete="new-password" />
        <small class="muted">{{ t('admin.addMemberHint') }}</small>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="t('common.add')" icon="pi pi-check" :loading="busy" :disabled="!form.email" />
      </div>
    </form>
  </Dialog>

  <TempPasswordDialog v-model:visible="tempOpen" :password="tempPassword" :user="tempUser" />
</template>
