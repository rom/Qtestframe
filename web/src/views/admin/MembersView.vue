<script setup lang="ts">
import HelpTip from '@/guide/HelpTip.vue'
import { computed, onMounted, ref, watch } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Password from 'primevue/password'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import UserAvatar from '@/components/UserAvatar.vue'
import TempPasswordDialog from '@/components/TempPasswordDialog.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate, formatDateTime } from '@/lib/format'
import { invitationState } from '@/lib/email'
import { P } from '@/lib/permissions'
import type { Membership } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useRolesStore } from '@/stores/roles'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { usePrefsStore } from '@/stores/prefs'
import { useI18n } from 'vue-i18n'
import { enumLabel, enumOptions } from '@/i18n/labels'
import { entry, recordUndo } from '@/lib/undoActions'
import { useUndoStore } from '@/stores/undo'
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
const rolesStore = useRolesStore()
const roles = computed(() => enumOptions('role', rolesStore.roles.length ? rolesStore.roles.map((r) => r.name) : (rbac.value?.roles ?? meta.meta.roles)))
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
  void meta.load()
  void rolesStore.ensure()
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

// ---------- several members at once ----------
interface BulkMember {
  user_id: string
  email?: string
  role?: string
  reason?: string
}
interface BulkResult {
  changed: BulkMember[]
  skipped: BulkMember[]
}
const selection = ref<Membership[]>([])
const bulkRole = ref<string | null>(null)
const bulkBusy = ref(false)
const undoStore = useUndoStore()
watch(
  () => undoStore.version,
  () => void load(),
)
watch(members, (l) => (selection.value = selection.value.filter((m) => l.some((x) => x.user_id === m.user_id))))

function skippedText(res: BulkResult): string {
  return res.skipped.map((x) => `${x.email || x.user_id}: ${t(`admin.bulk.reason.${x.reason}`)}`).join('\n')
}
function report(res: BulkResult, done: string) {
  if (res.skipped.length) notify.failText(t('admin.bulk.partly', { n: res.changed.length, skipped: res.skipped.length }), skippedText(res))
  else notify.ok(done)
}
const post = (body: Record<string, unknown>) => api.post<BulkResult>('/members/bulk', body)

function bulkSetRole(role: string | null) {
  if (!role) return
  const chosen = [...selection.value]
  const n = chosen.length
  notify.confirmDanger(
    t('admin.bulk.confirmRole', { n, role: enumLabel('role', role) }, n),
    async () => {
      bulkBusy.value = true
      try {
        const before = new Map(chosen.map((m) => [m.user_id, m.role]))
        const res = await post({ user_ids: chosen.map((m) => m.user_id), action: 'role', role })
        const label = t('admin.bulk.roleSet', { n: res.changed.length, role: enumLabel('role', role) }, res.changed.length)
        report(res, label)
        if (res.changed.length) {
          // back to the old roles, one request per old role
          const byOld = new Map<string, string[]>()
          for (const c of res.changed) {
            const old = before.get(c.user_id) ?? ''
            byOld.set(old, [...(byOld.get(old) ?? []), c.user_id])
          }
          const ids = res.changed.map((c) => c.user_id)
          recordUndo(
            entry(
              label,
              async () => {
                for (const [old, list] of byOld) await post({ user_ids: list, action: 'role', role: old })
              },
              () => post({ user_ids: ids, action: 'role', role }),
            ),
          )
        }
        selection.value = []
        await load()
      } catch (e) {
        notify.fail(e)
      } finally {
        bulkBusy.value = false
        bulkRole.value = null
      }
    },
    t('admin.bulk.roleHeader'),
    t('admin.bulk.roleAction'),
  )
}

function bulkRemove() {
  const chosen = [...selection.value]
  const n = chosen.length
  notify.confirmDanger(
    t('admin.bulk.confirmRemove', { n, tenant: auth.tenant?.name ?? '' }, n),
    async () => {
      bulkBusy.value = true
      try {
        const res = await post({ user_ids: chosen.map((m) => m.user_id), action: 'remove' })
        const label = t('admin.bulk.removed', { n: res.changed.length }, res.changed.length)
        report(res, label)
        if (res.changed.length) {
          const gone = res.changed.map((c) => ({ email: c.email ?? '', role: c.role ?? 'viewer', user_id: c.user_id }))
          recordUndo(
            entry(
              label,
              async () => {
                for (const m of gone) await api.post('/members', { email: m.email, role: m.role })
              },
              () => post({ user_ids: gone.map((m) => m.user_id), action: 'remove' }),
            ),
          )
        }
        selection.value = []
        await load()
      } catch (e) {
        notify.fail(e)
      } finally {
        bulkBusy.value = false
      }
    },
    t('admin.bulk.removeHeader'),
    t('common.remove'),
  )
}

const dialog = ref(false)
const emailEnabled = computed(() => !!meta.meta.email_enabled)
const blankForm = () => ({ email: '', display_name: '', role: 'tester', password: '', invite: emailEnabled.value, sso: false })
const form = ref(blankForm())
const formError = ref('')
const busy = ref(false)
function openAdd() {
  form.value = blankForm()
  formError.value = ''
  dialog.value = true
}
async function add() {
  busy.value = true
  formError.value = ''
  const invite = emailEnabled.value && form.value.invite
  try {
    const res = await api.post<Membership>('/members', {
      email: form.value.email.trim(),
      role: form.value.role,
      ...(form.value.display_name ? { display_name: form.value.display_name } : {}),
      ...(!invite && form.value.password ? { password: form.value.password } : {}),
      ...(invite ? { invite: true, sso: form.value.sso } : {}),
    })
    dialog.value = false
    if (invite) notify.ok(res?.invitation ? t('email.invite.sent') : t('admin.memberAdded'), res?.invitation ? form.value.email : t('email.invite.noticeSent'))
    else notify.ok(t('admin.memberAdded'), form.value.email)
    load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
// ---------- invitations ----------
const canTestMail = computed(() => auth.can(P.tenantManage))
const testing = ref(false)
async function testMail() {
  testing.value = true
  try {
    const res = await api.post<{ ok: boolean; to: string; error?: string }>('/admin/email/test')
    if (res?.ok) notify.ok(t('email.test.sent', { to: res.to }))
    else notify.failText(t('email.test.failed'), res?.error ?? '')
  } catch (e) {
    notify.fail(e, t('email.test.failed'))
  } finally {
    testing.value = false
  }
}
function inviteTip(m: Membership): string {
  const inv = m.invitation
  if (!inv) return ''
  const p = { sent: formatDateTime(inv.sent_at), expires: formatDateTime(inv.expires_at), by: inv.invited_by }
  return invitationState(inv) === 'expired' ? t('email.invite.expiredTip', p) : t('email.invite.invitedTip', p)
}
async function resend(m: Membership) {
  try {
    await api.post(`/members/${m.user_id}/invitation`)
    notify.ok(t('email.invite.resent'), m.email)
    load()
  } catch (e) {
    notify.fail(e)
  }
}
function revokeInvite(m: Membership) {
  notify.confirmDanger(
    t('email.invite.confirmRevoke', { user: m.email ?? '' }),
    async () => {
      try {
        await api.del(`/members/${m.user_id}/invitation`)
        notify.ok(t('email.invite.revoked'), m.email)
        load()
      } catch (e) {
        notify.fail(e)
      }
    },
    t('email.invite.revokeHeader'),
    t('email.invite.revoke'),
  )
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
      <Button v-if="canTestMail && emailEnabled" :label="t('email.test.button')" icon="pi pi-envelope" severity="secondary" outlined :loading="testing" v-tooltip.bottom="t('email.test.hint')" @click="testMail" />
      <Button v-if="canManage" :label="t('admin.addMember')" icon="pi pi-user-plus" @click="openAdd" />
    </div>
    <Message v-if="canManage && meta.loaded && !emailEnabled" severity="secondary" :closable="false" class="small" style="margin-bottom: 10px">{{ t('email.notConfigured') }}</Message>
    <div v-if="canManage && selection.length" class="bulkbar" role="toolbar" :aria-label="t('admin.bulk.toolbar')">
      <b>{{ t('common.selected', { count: selection.length }) }}</b>
      <Select
        v-model="bulkRole"
        :options="roles"
        option-label="label"
        option-value="value"
        :placeholder="t('admin.bulk.setRole')"
        :aria-label="t('admin.bulk.setRole')"
        size="small"
        style="min-width: 160px"
        :disabled="bulkBusy"
        @update:model-value="bulkSetRole"
      />
      <Button :label="t('common.remove')" icon="pi pi-user-minus" size="small" severity="danger" outlined :disabled="bulkBusy" @click="bulkRemove" />
      <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
    </div>
    <DataTable v-model:selection="selection" v-stack :value="filtered" data-key="user_id" :loading="loading" :aria-busy="loading" paginator :rows="prefs.rowsFor(50)" sort-field="email" :sort-order="1">
      <Column v-if="canManage" selection-mode="multiple" style="width: 2.6rem" />
      <Column field="email" :header="t('common.email')" sortable body-class="stack-main">
        <template #body="{ data }"><span class="row nowrap" style="gap: 8px; min-width: 0"><UserAvatar :avatar="data.avatar" :user-id="data.user_id" :email="data.email" :name="data.display_name || data.email" :size="28" /><span class="ellipsis">{{ data.email }}</span>
            <Tag
              v-if="data.invitation"
              v-tooltip="inviteTip(data)"
              tabindex="0"
              :aria-label="`${invitationState(data.invitation) === 'expired' ? t('email.invite.expired') : t('email.invite.invited')}: ${inviteTip(data)}`"
              :value="invitationState(data.invitation) === 'expired' ? t('email.invite.expired') : t('email.invite.invited')"
              :severity="invitationState(data.invitation) === 'expired' ? 'warn' : 'info'"
              icon="pi pi-envelope"
              class="invite-tag"
            /></span></template>
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
      <Column v-if="canManage" style="width: 140px; white-space: nowrap">
        <template #body="{ data }">
          <template v-if="data.invitation">
            <Button v-if="emailEnabled" icon="pi pi-send" text rounded severity="secondary" :aria-label="t('email.invite.resendOf', { user: data.email })" v-tooltip="t('email.invite.resend')" @click="resend(data)" />
            <Button icon="pi pi-ban" text rounded severity="danger" :aria-label="t('email.invite.revokeOf', { user: data.email })" v-tooltip="t('email.invite.revoke')" @click="revokeInvite(data)" />
          </template>
          <Button v-if="data.user_id !== auth.user?.id && !data.invitation" icon="pi pi-key" text rounded severity="secondary" :aria-label="t('admin.resetPasswordOf', { user: data.email })" v-tooltip="t('admin.resetPasswordAction')" @click="resetPassword(data)" />
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
      <template v-if="emailEnabled">
        <label class="row nowrap small" for="mb-invite"><ToggleSwitch v-model="form.invite" input-id="mb-invite" aria-describedby="mb-invite-hint" /> {{ t('email.invite.toggle') }}</label>
        <small id="mb-invite-hint" class="muted" style="margin-top: -6px">{{ t('email.invite.hint') }}</small>
        <template v-if="form.invite">
          <label class="row nowrap small" for="mb-sso"><ToggleSwitch v-model="form.sso" input-id="mb-sso" aria-describedby="mb-sso-hint" /> {{ t('email.invite.sso') }}</label>
          <small id="mb-sso-hint" class="muted" style="margin-top: -6px">{{ t('email.invite.ssoHint') }}</small>
        </template>
      </template>
      <div v-if="!(emailEnabled && form.invite)" v-field class="field">
        <label>{{ t('admin.initialPassword') }}</label>
        <HelpTip id="member.initialPassword" :label="t('admin.initialPassword')" />
        <Password v-model="form.password" toggle-mask fluid autocomplete="new-password" />
        <small class="muted">{{ t('admin.addMemberHint') }}</small>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="emailEnabled && form.invite ? t('email.invite.submit') : t('common.add')" :icon="emailEnabled && form.invite ? 'pi pi-send' : 'pi pi-check'" :loading="busy" :disabled="!form.email" />
      </div>
    </form>
  </Dialog>

  <TempPasswordDialog v-model:visible="tempOpen" :password="tempPassword" :user="tempUser" />
</template>

<style scoped>
.invite-tag {
  flex: none;
  font-size: 11px;
}
</style>
