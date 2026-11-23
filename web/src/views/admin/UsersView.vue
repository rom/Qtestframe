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
import Select from 'primevue/select'
import UserAvatar from '@/components/UserAvatar.vue'
import TempPasswordDialog from '@/components/TempPasswordDialog.vue'
import { generatePassword } from '@/lib/signin'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate, formatDateTime } from '@/lib/format'
import { blankUserFilter, filterUsers, lastSuperAdmin, userProviders, type UserMemberships } from '@/lib/tenantAdmin'
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
const filter = ref(blankUserFilter())
const filtered = computed(() => filterUsers(users.value, filter.value))
const statusOptions = computed(() => (['all', 'active', 'disabled'] as const).map((v) => ({ value: v, label: t(`admin.userFilter.status.${v}`) })))
const mfaOptions = computed(() => (['all', 'on', 'off'] as const).map((v) => ({ value: v, label: t(`admin.userFilter.mfa.${v}`) })))
const providerOptions = computed(() => [{ value: '', label: t('admin.userFilter.anyProvider') }, ...userProviders(users.value).map((p) => ({ value: p, label: p }))])
const announce = ref('')
const isLast = (u: User) => lastSuperAdmin(users.value, u)

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

// ---------- memberships and sessions ----------
const memberOf = ref<User | null>(null)
const memberships = ref<UserMemberships | null>(null)
async function openMemberships(u: User) {
  memberOf.value = u
  memberships.value = null
  try {
    memberships.value = await api.get<UserMemberships>(`/users/${u.id}/memberships`)
  } catch (e) {
    notify.fail(e)
    memberOf.value = null
  }
}
function revokeSessions(u: User) {
  notify.confirmDanger(
    t('admin.revokeSessions.confirm', { user: u.email }),
    async () => {
      try {
        const r = await api.post<{ revoked: number }>(`/users/${u.id}/sessions/revoke`)
        notify.ok(t('admin.revokeSessions.done', { n: r.revoked }), u.email)
        announce.value = t('admin.revokeSessions.done', { n: r.revoked })
        if (memberships.value && memberOf.value?.id === u.id) memberships.value.sessions = 0
      } catch (e) {
        notify.fail(e)
      }
    },
    t('admin.revokeSessions.header'),
    t('admin.revokeSessions.action'),
  )
}
async function setDisabled(u: User, disabled: boolean) {
  try {
    const next = await api.patch<User>(`/users/${u.id}`, { disabled })
    Object.assign(u, next)
    const msg = disabled ? t('admin.userDisabled', { user: u.email }) : t('admin.userEnabled', { user: u.email })
    notify.ok(msg)
    announce.value = msg
  } catch (e) {
    notify.fail(e)
  }
}

// ---------- MFA reset (lost authenticator) ----------
function resetMfa(u: User) {
  notify.confirmDanger(
    t('mfa.reset.confirm', { user: u.email }),
    async () => {
      try {
        await api.post(`/users/${u.id}/mfa/reset`)
        u.mfa_enabled = false
        notify.ok(t('mfa.reset.done'), u.email)
      } catch (e) {
        notify.fail(e, t('mfa.reset.failed'))
      }
    },
    t('mfa.reset.header'),
    t('mfa.reset.action'),
  )
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
      <IconField><InputIcon class="pi pi-filter" /><InputText v-model="filter.text" :placeholder="t('admin.filterUsers')" :aria-label="t('admin.filterUsers')" /></IconField>
      <Select v-model="filter.status" :options="statusOptions" option-label="label" option-value="value" :aria-label="t('admin.userFilter.statusLabel')" />
      <Select v-model="filter.provider" :options="providerOptions" option-label="label" option-value="value" :aria-label="t('admin.userFilter.providerLabel')" />
      <Select v-model="filter.mfa" :options="mfaOptions" option-label="label" option-value="value" :aria-label="t('admin.userFilter.mfaLabel')" />
      <div class="row nowrap" style="gap: 6px"><Checkbox v-model="filter.superAdmin" input-id="uf-sa" binary /><label for="uf-sa" class="small">{{ t('admin.userFilter.superAdmins') }}</label></div>
      <span class="small muted" role="status">{{ t('admin.userFilter.count', { n: filtered.length, total: users.length }) }}</span>
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
            <Tag v-if="data.mfa_enabled" :value="t('mfa.tag')" severity="success" icon="pi pi-shield" />
          </div>
        </template>
      </Column>
      <Column field="created_at" :header="t('common.created')" sortable style="width: 130px"><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column style="width: 250px; white-space: nowrap">
        <template #body="{ data }">
          <Button icon="pi pi-building" text rounded severity="secondary" :aria-label="t('admin.membershipsOf', { user: data.email })" v-tooltip="t('admin.memberships')" @click="openMemberships(data)" />
          <Button v-if="data.id !== auth.user?.id" icon="pi pi-sign-out" text rounded severity="secondary" :aria-label="t('admin.revokeSessions.of', { user: data.email })" v-tooltip="t('admin.revokeSessions.action')" @click="revokeSessions(data)" />
          <Button v-if="data.mfa_enabled && data.id !== auth.user?.id" icon="pi pi-shield" text rounded severity="secondary" :aria-label="t('mfa.reset.of', { user: data.email })" v-tooltip="t('mfa.reset.action')" @click="resetMfa(data)" />
          <Button v-if="isLocal(data) && data.id !== auth.user?.id" icon="pi pi-key" text rounded severity="secondary" :aria-label="t('admin.resetPasswordOf', { user: data.email })" v-tooltip="t('admin.resetPasswordAction')" @click="resetPassword(data)" />
          <Button v-if="data.id !== auth.user?.id && !isLast(data)" :icon="data.disabled ? 'pi pi-lock-open' : 'pi pi-ban'" text rounded :severity="data.disabled ? 'success' : 'warn'" :aria-label="data.disabled ? t('admin.enableNamed', { user: data.email }) : t('admin.disableNamed', { user: data.email })" v-tooltip="data.disabled ? t('admin.enableUser') : t('admin.disableUser')" @click="setDisabled(data, !data.disabled)" />
          <Button icon="pi pi-pencil" text rounded severity="secondary" :aria-label="t('admin.editNamed', { name: data.email })" v-tooltip="t('common.edit')" @click="openEdit(data)" />
        </template>
      </Column>
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
      <div class="row"><Checkbox v-model="form.super_admin" input-id="sa" binary :disabled="editing?.id === auth.user?.id || (!!editing && isLast(editing))" /><label for="sa">{{ t('admin.superAdministrator') }}</label></div>
      <div v-if="editing" class="row"><Checkbox v-model="form.disabled" input-id="dis" binary :disabled="editing.id === auth.user?.id || isLast(editing)" /><label for="dis">{{ t('admin.disabled') }}</label></div>
      <p v-if="editing && isLast(editing)" class="small muted" style="margin: 0"><i class="pi pi-lock" aria-hidden="true" /> {{ t('admin.lastSuperAdmin') }}</p>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :disabled="!form.email || (!editing && !form.password)" />
      </div>
    </form>
  </Dialog>
  <TempPasswordDialog v-model:visible="tempOpen" :password="tempPassword" :user="tempUser" />
  <Dialog :visible="!!memberOf" :header="memberOf ? t('admin.membershipsOf', { user: memberOf.email }) : ''" modal style="width: min(560px, 95vw)" @update:visible="(v: boolean) => !v && (memberOf = null)">
    <template v-if="memberOf && memberships">
      <p class="small muted">{{ t('admin.membershipsIntro', { provider: memberOf.auth_provider || 'local', sessions: memberships.sessions }) }}</p>
      <p v-if="memberOf.password_changed_at" class="small muted">{{ t('admin.passwordChanged', { date: formatDateTime(memberOf.password_changed_at) }) }}</p>
      <ul v-if="memberships.items.length" class="plain-list membership-list">
        <li v-for="m in memberships.items" :key="m.tenant_id">
          <b>{{ m.tenant_name }}</b> <span class="mono small muted">{{ m.tenant_slug }}</span>
          <span class="row" style="gap: 4px; flex-wrap: wrap; margin-top: 2px"><Tag v-for="r in m.roles ?? [m.role]" :key="r" :value="r" severity="secondary" /></span>
        </li>
      </ul>
      <p v-else class="muted">{{ t('admin.noMemberships') }}</p>
      <div class="row" style="justify-content: flex-end; gap: 8px">
        <Button v-if="memberOf.id !== auth.user?.id && memberships.sessions > 0" :label="t('admin.revokeSessions.action')" icon="pi pi-sign-out" severity="secondary" outlined @click="revokeSessions(memberOf)" />
        <Button :label="t('common.close')" text @click="memberOf = null" />
      </div>
    </template>
  </Dialog>
  <div class="sr-only" role="status" aria-live="polite">{{ announce }}</div>
</template>
