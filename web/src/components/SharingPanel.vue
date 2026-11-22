<script setup lang="ts">
// Project sharing: the access mode of a project and its members with their
// project roles (internal/api/sharing.go). Used in the Share dialog and in
// Project settings → Sharing.
import { computed, onMounted, ref, watch } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Select from 'primevue/select'
import RadioButton from 'primevue/radiobutton'
import AutoComplete from 'primevue/autocomplete'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import UserAvatar from '@/components/UserAvatar.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { filterMembers, includesSelf, lockedRoles, mayManageMember, memberName, sortMembers } from '@/lib/sharing'
import { diffRoles, rolesOf } from '@/lib/permissions'
import type { MemberCandidate, Project, ProjectAccessMode, ProjectMember, ProjectSharing } from '@/lib/types'
import { enumLabel, enumOptions, rolesLabel } from '@/i18n/labels'
import RolePicker from '@/components/RolePicker.vue'
import { useAuthStore } from '@/stores/auth'
import { useRolesStore } from '@/stores/roles'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const props = defineProps<{ project: Project; headingLevel?: 2 | 3 }>()
const emit = defineEmits<{ changed: [sharing: ProjectSharing] }>()
const auth = useAuthStore()
const roles = useRolesStore()
const notify = useNotify()
const { t } = useI18n()

const sharing = ref<ProjectSharing | null>(null)
const members = ref<ProjectMember[]>([])
const loading = ref(false)
const error = ref('')
const filter = ref('')
const canEdit = computed(() => !!sharing.value?.my_access?.can_share || !!auth.user?.super_admin)
const grantableNames = computed(() => sharing.value?.grantable_roles ?? [])
const grantable = computed(() => enumOptions('role', grantableNames.value))
const mayManage = (m: ProjectMember) => mayManageMember(m, grantableNames.value)
const shown = computed(() => sortMembers(filterMembers(members.value, filter.value, (r) => enumLabel('role', r))))
const uid = `share-${Math.random().toString(36).slice(2, 8)}`

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [s, m] = await Promise.all([
      api.get<ProjectSharing>(`/projects/${props.project.id}/access`),
      api.get<{ items: ProjectMember[] }>(`/projects/${props.project.id}/members`),
    ])
    sharing.value = s
    members.value = asItems(m)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
onMounted(() => {
  void roles.ensure()
  void load()
})
watch(
  () => props.project.id,
  () => void load(),
)

const modes = computed(() => [
  { value: 'organisation' as ProjectAccessMode, icon: 'pi pi-building', label: t('sharing.mode.organisation'), hint: t('sharing.mode.organisationHint', { org: auth.tenant?.name ?? '' }) },
  { value: 'restricted' as ProjectAccessMode, icon: 'pi pi-lock', label: t('sharing.mode.restricted'), hint: t('sharing.mode.restrictedHint') },
])
const savingMode = ref(false)
async function setMode(mode: ProjectAccessMode) {
  if (!sharing.value || mode === sharing.value.access) return
  const apply = async () => {
    savingMode.value = true
    try {
      sharing.value = await api.put<ProjectSharing>(`/projects/${props.project.id}/access`, { access: mode })
      members.value = asItems(await api.get<{ items: ProjectMember[] }>(`/projects/${props.project.id}/members`))
      notify.ok(mode === 'restricted' ? t('sharing.nowRestricted') : t('sharing.nowOrganisation'), props.project.name)
      emit('changed', sharing.value)
    } catch (e) {
      notify.fail(e)
    } finally {
      savingMode.value = false
    }
  }
  if (mode === 'organisation') notify.confirmDanger(t('sharing.confirmOrganisation', { org: auth.tenant?.name ?? '' }), apply, t('sharing.confirmOrganisationHeader'), t('sharing.openToAll'))
  else await apply()
}

// ---------- add a member ----------
const pick = ref<MemberCandidate | string | null>(null)
const suggestions = ref<MemberCandidate[]>([])
const addRoles = ref<string[]>(['viewer'])
const adding = ref(false)
async function complete(ev: { query: string }) {
  try {
    suggestions.value = asItems(await api.get<{ items: MemberCandidate[] }>(`/projects/${props.project.id}/members/candidates`, { q: ev.query }))
  } catch (e) {
    suggestions.value = []
    notify.fail(e)
  }
}
const candidateLabel = (c: MemberCandidate) => (c.type === 'group' ? `${c.display_name} (${t('sharing.group')})` : c.display_name ? `${c.display_name} <${c.email}>` : (c.email ?? c.id))
watch(grantableNames, (g) => {
  const keep = addRoles.value.filter((r) => g.includes(r))
  if (g.length && keep.length !== addRoles.value.length) addRoles.value = keep.length ? keep : [g[g.length - 1]]
})
async function add() {
  const c = pick.value
  if (!c) return
  const body: Record<string, string | string[]> = { roles: addRoles.value }
  if (typeof c === 'string') body.email = c.trim()
  else if (c.type === 'group') body.group_id = c.id
  else body.user_id = c.id
  adding.value = true
  try {
    const m = await api.post<ProjectMember>(`/projects/${props.project.id}/members`, body)
    notify.ok(t('sharing.added'), `${memberName(m)}: ${rolesLabel(rolesOf(m))}`)
    pick.value = null
    await reloadMembers()
  } catch (e) {
    notify.fail(e)
  } finally {
    adding.value = false
  }
}
async function reloadMembers() {
  members.value = asItems(await api.get<{ items: ProjectMember[] }>(`/projects/${props.project.id}/members`))
  if (sharing.value) {
    sharing.value.members = members.value.length
    emit('changed', sharing.value)
  }
}

// ---------- change and remove ----------
async function setRoles(m: ProjectMember, next: string[]) {
  const before = rolesOf(m)
  try {
    const res = await api.patch<ProjectMember>(`/projects/${props.project.id}/members/${m.id}`, { roles: next })
    m.roles = res?.roles ?? next
    m.role = res?.role ?? next[0]
    const { added, removed } = diffRoles(before, next)
    notify.ok(t('sharing.rolesChanged'), `${memberName(m)}: ${[...added.map((r) => '+' + enumLabel('role', r)), ...removed.map((r) => '−' + enumLabel('role', r))].join(' ')}`)
  } catch (e) {
    notify.fail(e)
    m.roles = before
  }
}
function remove(m: ProjectMember) {
  const self = m.id === auth.user?.id
  notify.confirmDanger(
    self ? t('sharing.confirmLeave', { project: props.project.name }) : t('sharing.confirmRemove', { member: memberName(m), project: props.project.name }),
    async () => {
      try {
        await api.del(`/projects/${props.project.id}/members/${m.id}`)
        notify.ok(t('sharing.removed'), memberName(m))
        await reloadMembers()
      } catch (e) {
        notify.fail(e)
      }
    },
    t('sharing.removeHeader'),
    t('common.remove'),
  )
}

// ---------- several members at once ----------
interface BulkItem {
  id: string
  label: string
  reason?: string
}
const selection = ref<ProjectMember[]>([])
const bulkRole = ref<string | null>(null)
const bulkAddRole = ref<string | null>(null)
const bulkRemoveRole = ref<string | null>(null)
const bulkBusy = ref(false)
watch(members, (l) => (selection.value = selection.value.filter((m) => l.some((x) => x.id === m.id))))
const reasonText = (r?: string) =>
  r === 'unchanged' ? t('sharing.reason.unchanged') : r === 'forbidden' ? t('sharing.reason.forbidden') : r === 'last_role' ? t('sharing.reason.lastRole') : t('sharing.reason.notFound')
async function bulk(body: Record<string, unknown>, done: (n: number) => string) {
  bulkBusy.value = true
  try {
    const res = await api.post<{ changed: BulkItem[]; skipped: BulkItem[] }>(`/projects/${props.project.id}/members/bulk`, { ids: selection.value.map((m) => m.id), ...body })
    if (res.skipped.length) notify.failText(t('sharing.partly', { n: res.changed.length, skipped: res.skipped.length }), res.skipped.map((x) => `${x.label || x.id}: ${reasonText(x.reason)}`).join('\n'))
    else notify.ok(done(res.changed.length))
    selection.value = []
    await reloadMembers()
  } catch (e) {
    notify.fail(e)
  } finally {
    bulkBusy.value = false
    bulkRole.value = null
    bulkAddRole.value = null
    bulkRemoveRole.value = null
  }
}
function bulkSetRole(role: string | null, action: 'role' | 'add_role' | 'remove_role' = 'role') {
  if (!role) return
  const n = selection.value.length
  const label = enumLabel('role', role)
  const confirmKey = { role: 'sharing.confirmBulkRole', add_role: 'sharing.confirmBulkAddRole', remove_role: 'sharing.confirmBulkRemoveRole' }[action]
  const doneKey = { role: 'sharing.bulkRoleSet', add_role: 'sharing.bulkRoleAdded', remove_role: 'sharing.bulkRoleRemoved' }[action]
  notify.confirmDanger(
    t(confirmKey, { n, role: label }, n),
    () => bulk({ action, roles: [role] }, (k) => t(doneKey, { n: k, role: label }, k)),
    t('sharing.bulkRoleHeader'),
    t('sharing.bulkRoleAction'),
  )
}
function bulkRemove() {
  const n = selection.value.length
  const self = includesSelf(
    selection.value.map((m) => m.id),
    auth.user?.id,
  )
  notify.confirmDanger(
    (self ? t('sharing.bulkIncludesYou') + ' ' : '') + t('sharing.confirmBulkRemove', { n, project: props.project.name }, n),
    () => bulk({ action: 'remove' }, (k) => t('sharing.bulkRemoved', { n: k }, k)),
    t('sharing.removeHeader'),
    t('common.remove'),
  )
}
const sourceText = computed(() => {
  const a = sharing.value?.my_access
  if (!a) return ''
  if (a.source === 'admin') return t('sharing.youAdmin')
  if (a.source === 'organisation') return t('sharing.youOrganisation')
  if (a.source === 'group') return t('sharing.youGroup', { role: rolesLabel(rolesOf(a)) })
  return t('sharing.youMember', { role: rolesLabel(rolesOf(a)) })
})
</script>

<template>
  <div class="sharing-panel">
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <template v-else-if="sharing">
      <fieldset class="plain-fieldset access-modes" :disabled="!canEdit || savingMode">
        <legend :id="`${uid}-mode`" class="field-label">{{ t('sharing.access') }}</legend>
        <label v-for="m in modes" :key="m.value" class="access-mode" :class="{ active: sharing.access === m.value }" :for="`${uid}-${m.value}`">
          <RadioButton :model-value="sharing.access" :value="m.value" :input-id="`${uid}-${m.value}`" :name="`${uid}-mode`" :aria-describedby="`${uid}-${m.value}-hint`" @update:model-value="setMode" />
          <span class="grow">
            <span class="row nowrap" style="gap: 6px"><i :class="m.icon" aria-hidden="true" /><b>{{ m.label }}</b></span>
            <small :id="`${uid}-${m.value}-hint`" class="muted">{{ m.hint }}</small>
          </span>
        </label>
      </fieldset>
      <p class="small muted" style="margin: 4px 0 12px">{{ sourceText }} {{ t('sharing.rolesHint') }}</p>

      <component :is="headingLevel === 3 ? 'h3' : 'h2'" :id="`${uid}-members`" class="plain-heading members-heading">{{ t('sharing.members') }} ({{ members.length }})</component>
      <form v-if="canEdit" class="add-member" :aria-label="t('sharing.addMember')" @submit.prevent="add">
        <AutoComplete
          v-model="pick"
          :suggestions="suggestions"
          :option-label="candidateLabel"
          complete-on-focus
          :delay="200"
          :placeholder="t('sharing.searchPeople')"
          :aria-label="t('sharing.searchPeople')"
          class="grow"
          fluid
          @complete="complete"
        >
          <template #option="{ option }">
            <span class="row nowrap" style="gap: 8px; min-width: 0">
              <i v-if="option.type === 'group'" class="pi pi-users" aria-hidden="true" />
              <UserAvatar v-else :avatar="option.avatar" :user-id="option.id" :email="option.email" :name="option.display_name || option.email" :size="22" />
              <span class="ellipsis">{{ candidateLabel(option) }}</span>
            </span>
          </template>
        </AutoComplete>
        <div class="add-roles"><RolePicker v-model="addRoles" :options="grantableNames" :aria-label="t('sharing.rolesToGive')" /></div>
        <Button type="submit" :label="t('sharing.add')" icon="pi pi-user-plus" :loading="adding" :disabled="!pick" />
      </form>
      <div class="filterbar" style="margin-top: 8px">
        <IconField><InputIcon class="pi pi-filter" /><InputText v-model="filter" :placeholder="t('sharing.filter')" :aria-label="t('sharing.filter')" /></IconField>
      </div>
      <div v-if="canEdit && selection.length" class="bulkbar" role="toolbar" :aria-label="t('sharing.bulkToolbar')">
        <b>{{ t('common.selected', { count: selection.length }) }}</b>
        <Select v-model="bulkRole" :options="grantable" option-label="label" option-value="value" :placeholder="t('sharing.bulkSetRole')" :aria-label="t('sharing.bulkSetRole')" size="small" style="min-width: 150px" :disabled="bulkBusy" @update:model-value="(r: string | null) => bulkSetRole(r, 'role')" />
        <Select v-model="bulkAddRole" :options="grantable" option-label="label" option-value="value" :placeholder="t('sharing.bulkAddRole')" :aria-label="t('sharing.bulkAddRole')" size="small" style="min-width: 150px" :disabled="bulkBusy" @update:model-value="(r: string | null) => bulkSetRole(r, 'add_role')" />
        <Select v-model="bulkRemoveRole" :options="grantable" option-label="label" option-value="value" :placeholder="t('sharing.bulkRemoveRole')" :aria-label="t('sharing.bulkRemoveRole')" size="small" style="min-width: 150px" :disabled="bulkBusy" @update:model-value="(r: string | null) => bulkSetRole(r, 'remove_role')" />
        <Button :label="t('common.remove')" icon="pi pi-user-minus" size="small" severity="danger" outlined :disabled="bulkBusy" @click="bulkRemove" />
        <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
      </div>
      <DataTable v-model:selection="selection" v-stack :value="shown" data-key="id" :loading="loading" :aria-busy="loading" :aria-labelledby="`${uid}-members`" size="small" class="members-table">
        <template #empty>
          <span class="muted">{{ sharing.access === 'restricted' ? t('sharing.noMembersRestricted') : t('sharing.noMembers') }}</span>
        </template>
        <Column v-if="canEdit" selection-mode="multiple" style="width: 2.6rem" />
        <Column :header="t('sharing.member')" body-class="stack-main">
          <template #body="{ data }">
            <span class="row nowrap" style="gap: 8px; min-width: 0">
              <span v-if="data.type === 'group'" class="group-icon" aria-hidden="true"><i class="pi pi-users" /></span>
              <UserAvatar v-else :avatar="data.avatar" :user-id="data.id" :email="data.email" :name="data.display_name || data.email" :size="26" />
              <span class="ellipsis">
                {{ memberName(data) }}
                <span v-if="data.type === 'user' && data.display_name" class="small muted">{{ data.email }}</span>
              </span>
              <Tag v-if="data.type === 'group'" :value="t('sharing.group')" severity="info" class="flat-tag" />
              <Tag v-if="data.id === auth.user?.id" :value="t('sharing.you')" severity="secondary" class="flat-tag" />
            </span>
          </template>
        </Column>
        <Column :header="t('sharing.orgRoles')" style="width: 160px">
          <template #body="{ data }">{{ data.type === 'user' ? rolesLabel(data.org_roles?.length ? data.org_roles : data.org_role ? [data.org_role] : []) : '–' }}</template>
        </Column>
        <Column :header="t('sharing.projectRoles')" style="width: 240px">
          <template #body="{ data }">
            <RolePicker
              v-if="canEdit"
              :model-value="rolesOf(data)"
              :options="grantableNames"
              :locked="lockedRoles(data, grantableNames)"
              size="small"
              :aria-label="t('sharing.rolesOf', { member: memberName(data) })"
              @change="(r: string[]) => setRoles(data, r)"
            />
            <span v-else class="row wrap" style="gap: 4px"><Tag v-for="r in rolesOf(data)" :key="r" :value="enumLabel('role', r)" severity="secondary" /></span>
          </template>
        </Column>
        <Column style="width: 56px">
          <template #body="{ data }">
            <Button
              v-if="(canEdit && mayManage(data)) || data.id === auth.user?.id"
              icon="pi pi-user-minus"
              text
              rounded
              severity="danger"
              :aria-label="data.id === auth.user?.id ? t('sharing.leave') : t('sharing.removeOf', { member: memberName(data) })"
              v-tooltip="data.id === auth.user?.id ? t('sharing.leave') : t('common.remove')"
              @click="remove(data)"
            />
          </template>
        </Column>
      </DataTable>
    </template>
    <div v-else class="muted small" aria-busy="true">{{ t('common.loading') }}</div>
  </div>
</template>

<style scoped>
.access-modes {
  display: grid;
  gap: 8px;
}
.access-mode {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 12px;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  cursor: pointer;
}
.access-mode.active {
  border-color: var(--p-primary-color);
}
.access-mode small {
  display: block;
}
.members-heading {
  font-size: 1rem;
  margin: 8px 0;
}
.add-member {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
}
.add-member .grow {
  min-width: 220px;
}
.add-roles {
  min-width: 180px;
  max-width: 100%;
}
.group-icon {
  display: inline-flex;
  width: 26px;
  height: 26px;
  border-radius: 50%;
  align-items: center;
  justify-content: center;
  background: var(--qtf-border);
  flex: none;
}
.flat-tag {
  flex: none;
  font-size: 11px;
}
</style>
