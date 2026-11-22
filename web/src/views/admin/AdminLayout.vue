<script setup lang="ts">
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { P } from '@/lib/permissions'
import { useI18n } from 'vue-i18n'
import HelpLink from '@/guide/HelpLink.vue'

const auth = useAuthStore()
const { t } = useI18n()
const tabs = computed(() =>
  [
    { to: 'members', label: t('admin.members'), icon: 'pi pi-users', show: auth.can(P.memberRead) },
    { to: 'tokens', label: t('admin.tokens'), icon: 'pi pi-key', show: auth.can(P.tokenManage) },
    { to: 'provisioning', label: t('provisioning.tab'), icon: 'pi pi-sync', show: auth.can(P.tenantManage) },
    { to: 'audit', label: t('admin.audit'), icon: 'pi pi-history', show: auth.can(P.auditRead) },
    { to: 'rbac', label: t('admin.roleMatrix'), icon: 'pi pi-table', show: true },
    { to: 'fieldSets', label: t('fieldSets.tab'), icon: 'pi pi-th-large', show: auth.can(P.settingsManage) },
    { to: 'executionSettings', label: t('execSettings.tab'), icon: 'pi pi-sliders-h', show: auth.can(P.settingsManage) },
    { to: 'mfaPolicy', label: t('mfa.adminTab'), icon: 'pi pi-shield', show: auth.can(P.tenantManage) },
    { to: 'tenants', label: t('admin.tenants'), icon: 'pi pi-building', show: auth.isSuperAdmin },
    { to: 'users', label: t('admin.users'), icon: 'pi pi-id-card', show: auth.isSuperAdmin },
    { to: 'backup', label: t('admin.backup'), icon: 'pi pi-database', show: auth.isSuperAdmin },
    { to: 'systemSettings', label: t('sysSettings.tab'), icon: 'pi pi-cog', show: auth.isSuperAdmin },
  ].filter((t) => t.show),
)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('admin.title') }}</h1>
      <div class="sub">{{ auth.tenant?.name ?? t('admin.noTenant') }}</div>
    </div>
    <div class="actions"><HelpLink page="administration" /></div>
  </div>
  <nav class="tabs-nav" :aria-label="t('admin.sections')">
    <RouterLink v-for="tab in tabs" :key="tab.to" :to="{ name: tab.to }"><i :class="tab.icon" aria-hidden="true" />{{ tab.label }}</RouterLink>
  </nav>
  <RouterView />
</template>
