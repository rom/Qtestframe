<script setup lang="ts">
// Read-only role × permission matrix (GET /rbac) with permission descriptions.
import { computed, onMounted, ref } from 'vue'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import { api, errorMessage } from '@/lib/api'
import { matrixPermissions, permissionKey } from '@/lib/permissions'
import type { RbacInfo } from '@/lib/types'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'

const { t, te } = useI18n()
const rbac = ref<RbacInfo | null>(null)
const error = ref('')
const loading = ref(true)
onMounted(async () => {
  try {
    rbac.value = await api.get<RbacInfo>('/rbac')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
})
const perms = computed(() => matrixPermissions(rbac.value?.matrix))
const desc = (p: string) => (te(`rbac.perm.${permissionKey(p)}`) ? t(`rbac.perm.${permissionKey(p)}`) : '')
const has = (role: string, p: string) => !!rbac.value?.matrix?.[role]?.includes(p)
</script>

<template>
  <div class="card">
    <h2 class="plain-heading" style="margin-bottom: 4px">{{ t('admin.rolesAndPermissions') }}</h2>
    <p class="small muted" style="margin-top: 0">{{ t('rbac.intro') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="loading" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else-if="rbac" class="table-scroll" tabindex="0" role="region" :aria-label="t('admin.rolesAndPermissions')">
      <table class="rbac-table">
        <caption class="sr-only">{{ t('admin.rolesAndPermissions') }}</caption>
        <thead>
          <tr>
            <th scope="col">{{ t('admin.permission') }}</th>
            <th v-for="r in rbac.roles" :key="r" scope="col" class="role-col">{{ enumLabel('role', r) }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in perms" :key="p">
            <th scope="row">
              <div class="mono small">{{ p }}</div>
              <div v-if="desc(p)" class="small muted" style="font-weight: normal">{{ desc(p) }}</div>
            </th>
            <td v-for="r in rbac.roles" :key="r" class="role-col">
              <i v-if="has(r, p)" class="pi pi-check yes" aria-hidden="true" />
              <i v-else class="pi pi-times no" aria-hidden="true" />
              <span class="sr-only">{{ has(r, p) ? t('rbac.granted') : t('rbac.notGranted') }}</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="small muted">{{ t('rbac.superAdminNote') }}</p>
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
}
.rbac-table .role-col {
  text-align: center;
  width: 90px;
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
