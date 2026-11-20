<script setup lang="ts">
// Role × permission matrix (GET /roles). Holders of role.manage can tick and
// untick permissions of every role except admin, save or discard the
// changes, reset built-in roles to their defaults, and create, rename and
// delete their own roles.
import { computed, onMounted, reactive, ref } from 'vue'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import { api, errorMessage } from '@/lib/api'
import { P, matrixPermissions, permissionKey } from '@/lib/permissions'
import type { RoleInfo } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useRolesStore } from '@/stores/roles'
import { useNotify } from '@/composables/notify'
import { useConfirm } from 'primevue/useconfirm'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'

const { t, te } = useI18n()
const auth = useAuthStore()
const store = useRolesStore()
const notify = useNotify()
const confirmService = useConfirm()
const confirm = {
  ask(o: { header: string; message: string; acceptLabel: string; danger?: boolean; accept: () => Promise<void> }) {
    confirmService.require({
      header: o.header,
      message: o.message,
      icon: o.danger ? 'pi pi-exclamation-triangle' : 'pi pi-replay',
      rejectProps: { label: t('common.cancel'), severity: 'secondary', outlined: true },
      acceptProps: { label: o.acceptLabel, severity: o.danger ? 'danger' : undefined },
      accept: o.accept,
    })
  },
}
const error = ref('')
const loading = ref(true)
const saving = ref(false)
/** role name → edited permission list (only roles with unsaved changes) */
const edits = reactive<Record<string, string[]>>({})

async function load() {
  loading.value = true
  error.value = ''
  try {
    await store.ensure(true)
    if (!store.roles.length && !auth.tenant) error.value = t('admin.noTenant')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const canManage = computed(() => auth.can(P.roleManage))
const roles = computed(() => store.roles)
const perms = computed(() => {
  const all = store.permissions.length ? store.permissions : matrixPermissions(Object.fromEntries(roles.value.map((r) => [r.name, r.permissions])))
  return matrixPermissions(Object.fromEntries([['_', all]]))
})
const mine = computed(() => new Set(auth.me?.permissions ?? []))
const desc = (p: string) => (te(`rbac.perm.${permissionKey(p)}`) ? t(`rbac.perm.${permissionKey(p)}`) : '')
const label = (r: RoleInfo) => (r.builtin ? enumLabel('role', r.name) : r.label || r.name)
const current = (r: RoleInfo) => edits[r.name] ?? r.permissions
const has = (r: RoleInfo, p: string) => current(r).includes(p)
const changed = computed(() => Object.keys(edits))
/** a permission can be ticked only by someone who holds it */
const grantable = (p: string) => auth.isSuperAdmin || mine.value.has(p)

function toggle(r: RoleInfo, p: string, on: boolean) {
  const next = new Set(current(r))
  if (on) next.add(p)
  else next.delete(p)
  const list = [...next].sort()
  if (list.join() === [...r.permissions].sort().join()) delete edits[r.name]
  else edits[r.name] = list
}
function discard() {
  for (const k of Object.keys(edits)) delete edits[k]
}
async function save() {
  saving.value = true
  try {
    for (const name of changed.value) {
      await api.put(`/roles/${name}`, { permissions: edits[name] })
      delete edits[name]
    }
    notify.ok(t('rbac.saved'))
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    saving.value = false
  }
}
function reset(r: RoleInfo) {
  confirm.ask({
    header: t('rbac.resetTitle', { role: label(r) }),
    message: t('rbac.resetText'),
    acceptLabel: t('rbac.reset'),
    accept: async () => {
      try {
        await api.post(`/roles/${r.name}/reset`)
        delete edits[r.name]
        notify.ok(t('rbac.resetDone', { role: label(r) }))
        await load()
      } catch (e) {
        notify.fail(e)
      }
    },
  })
}
function remove(r: RoleInfo) {
  confirm.ask({
    header: t('rbac.deleteTitle', { role: label(r) }),
    message: t('rbac.deleteText'),
    acceptLabel: t('common.delete'),
    danger: true,
    accept: async () => {
      try {
        await api.del(`/roles/${r.name}`)
        delete edits[r.name]
        notify.ok(t('rbac.deleted', { role: label(r) }))
        await load()
      } catch (e) {
        notify.fail(e)
      }
    },
  })
}

// ---------- create / edit dialog ----------
const dialog = ref(false)
const editing = ref<RoleInfo | null>(null)
const form = reactive({ name: '', label: '', description: '', copyFrom: 'viewer' })
const formError = ref('')
function openCreate() {
  editing.value = null
  Object.assign(form, { name: '', label: '', description: '', copyFrom: 'viewer' })
  formError.value = ''
  dialog.value = true
}
function openEdit(r: RoleInfo) {
  editing.value = r
  Object.assign(form, { name: r.name, label: r.label ?? '', description: r.description ?? '', copyFrom: '' })
  formError.value = ''
  dialog.value = true
}
const copyOptions = computed(() => roles.value.map((r) => ({ label: label(r), value: r.name })))
const nameValid = computed(() => /^[a-z][a-z0-9_]{1,31}$/.test(form.name))
function slugFromLabel() {
  if (editing.value || form.name) return
  form.name = form.label
    .toLowerCase()
    .normalize('NFD')
    .replace(/\p{M}/gu, '')
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^[^a-z]+|_+$/g, '')
    .slice(0, 32)
}
async function submit() {
  formError.value = ''
  try {
    if (editing.value) {
      await api.put(`/roles/${editing.value.name}`, { label: form.label, description: form.description })
    } else {
      const base = roles.value.find((r) => r.name === form.copyFrom)
      const permissions = (base ? current(base) : []).filter(grantable)
      await api.post('/roles', { name: form.name, label: form.label, description: form.description, permissions })
    }
    dialog.value = false
    notify.ok(editing.value ? t('rbac.updated') : t('rbac.created', { role: form.label || form.name }))
    await load()
  } catch (e) {
    formError.value = errorMessage(e)
  }
}
</script>

<template>
  <div class="card">
    <div class="row" style="justify-content: space-between; align-items: flex-start; gap: 12px; flex-wrap: wrap">
      <div>
        <h2 class="plain-heading" style="margin-bottom: 4px">{{ t('admin.rolesAndPermissions') }}</h2>
        <p class="small muted" style="margin-top: 0; max-width: 70ch">{{ canManage ? t('rbac.introManage') : t('rbac.intro') }}</p>
      </div>
      <Button v-if="canManage" :label="t('rbac.newRole')" icon="pi pi-plus" @click="openCreate" />
    </div>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="loading && !roles.length" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <template v-else>
      <div v-if="changed.length" class="unsaved row" role="status">
        <i class="pi pi-pencil" aria-hidden="true" />
        <span class="grow">{{ t('rbac.unsaved', { count: changed.length }) }}</span>
        <Button :label="t('common.cancel')" text severity="secondary" size="small" :disabled="saving" @click="discard" />
        <Button :label="t('common.save')" icon="pi pi-check" size="small" :loading="saving" @click="save" />
      </div>
      <div class="table-scroll" tabindex="0" role="region" :aria-label="t('admin.rolesAndPermissions')">
        <table class="rbac-table">
          <caption class="sr-only">{{ t('admin.rolesAndPermissions') }}</caption>
          <thead>
            <tr>
              <th scope="col">{{ t('admin.permission') }}</th>
              <th v-for="r in roles" :key="r.name" scope="col" class="role-col">
                <div class="role-head">
                  <span class="role-name">{{ label(r) }}</span>
                  <span class="small muted">{{ t('rbac.memberCount', { count: r.members }) }}</span>
                  <Tag v-if="!r.builtin" :value="t('rbac.custom')" severity="info" class="small" />
                  <Tag v-else-if="r.customized" :value="t('rbac.changed')" severity="warn" class="small" />
                  <Tag v-if="edits[r.name]" :value="t('rbac.unsavedTag')" severity="secondary" class="small" />
                  <span v-if="canManage" class="row nowrap" style="gap: 0; justify-content: center">
                    <Button v-if="!r.builtin" icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('rbac.editRole', { role: label(r) })" v-tooltip.bottom="t('rbac.editRole', { role: label(r) })" @click="openEdit(r)" />
                    <Button v-if="r.builtin && r.customized" icon="pi pi-replay" text rounded size="small" severity="secondary" :aria-label="t('rbac.resetTitle', { role: label(r) })" v-tooltip.bottom="t('rbac.resetTitle', { role: label(r) })" @click="reset(r)" />
                    <Button
                      v-if="r.deletable"
                      icon="pi pi-trash"
                      text
                      rounded
                      size="small"
                      severity="danger"
                      :disabled="r.members + r.tokens > 0"
                      :aria-label="t('rbac.deleteTitle', { role: label(r) })"
                      v-tooltip.bottom="r.members + r.tokens > 0 ? t('rbac.inUse') : t('rbac.deleteTitle', { role: label(r) })"
                      @click="remove(r)"
                    />
                  </span>
                </div>
                <div v-if="r.description" class="small muted role-desc">{{ r.description }}</div>
              </th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in perms" :key="p">
              <th scope="row">
                <div class="mono small">{{ p }}</div>
                <div v-if="desc(p)" class="small muted" style="font-weight: normal">{{ desc(p) }}</div>
              </th>
              <td v-for="r in roles" :key="r.name" class="role-col" :class="{ dirty: edits[r.name] && has(r, p) !== r.permissions.includes(p) }">
                <template v-if="canManage && r.editable">
                  <Checkbox
                    :model-value="has(r, p)"
                    binary
                    :disabled="!grantable(p) && !has(r, p)"
                    :aria-label="t('rbac.toggle', { permission: p, role: label(r) })"
                    @update:model-value="(v: boolean) => toggle(r, p, v)"
                  />
                </template>
                <template v-else>
                  <i v-if="has(r, p)" class="pi pi-check yes" aria-hidden="true" />
                  <i v-else class="pi pi-times no" aria-hidden="true" />
                  <span class="sr-only">{{ has(r, p) ? t('rbac.granted') : t('rbac.notGranted') }}</span>
                </template>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="small muted">{{ t('rbac.adminNote') }} {{ t('rbac.superAdminNote') }}</p>
    </template>

    <Dialog v-model:visible="dialog" modal :header="editing ? t('rbac.editRole', { role: editing.label || editing.name }) : t('rbac.newRole')" :style="{ width: 'min(480px, 96vw)' }">
      <form class="grid" style="gap: 12px" @submit.prevent="submit">
        <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
        <div v-field class="field"><label>{{ t('rbac.label') }}</label><InputText v-model="form.label" :maxlength="64" autofocus required @blur="slugFromLabel" /></div>
        <div v-if="!editing" v-field class="field">
          <label>{{ t('rbac.name') }}</label>
          <InputText v-model="form.name" class="mono" :maxlength="32" required :invalid="!!form.name && !nameValid" aria-describedby="role-name-hint" />
          <small id="role-name-hint" class="muted">{{ t('rbac.nameHint') }}</small>
        </div>
        <div v-field class="field"><label>{{ t('common.description') }}</label><Textarea v-model="form.description" rows="2" auto-resize :maxlength="500" /></div>
        <div v-if="!editing" v-field class="field">
          <label>{{ t('rbac.copyFrom') }}</label>
          <Select v-model="form.copyFrom" :options="copyOptions" option-label="label" option-value="value" />
          <small class="muted">{{ t('rbac.copyHint') }}</small>
        </div>
        <div class="row" style="justify-content: flex-end">
          <Button :label="t('common.cancel')" text severity="secondary" type="button" @click="dialog = false" />
          <Button :label="editing ? t('common.save') : t('common.create')" icon="pi pi-check" type="submit" :disabled="!editing && !nameValid" />
        </div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
.rbac-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.rbac-table th,
.rbac-table td {
  padding: 6px 8px;
  border-top: 1px solid var(--qtf-border);
  text-align: left;
  vertical-align: top;
}
.rbac-table thead th {
  border-top: none;
  vertical-align: bottom;
}
.rbac-table .role-col {
  text-align: center;
  min-width: 104px;
}
.role-head {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}
.role-name {
  font-weight: 700;
}
.role-desc {
  font-weight: normal;
  max-width: 160px;
  margin: 2px auto 0;
}
td.dirty {
  background: var(--p-highlight-background, var(--qtf-hover));
}
.unsaved {
  gap: 8px;
  padding: 8px 12px;
  margin: 8px 0;
  border: 1px solid var(--p-primary-color);
  border-radius: 8px;
  position: sticky;
  top: 0;
  z-index: 2;
  background: var(--qtf-card);
}
.yes {
  color: var(--p-green-600);
  font-weight: 700;
}
.app-dark .yes {
  color: var(--p-green-400);
}
.no {
  color: var(--qtf-muted);
}
</style>
