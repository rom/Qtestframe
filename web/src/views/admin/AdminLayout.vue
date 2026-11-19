<script setup lang="ts">
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { P } from '@/lib/permissions'
import { useI18n } from 'vue-i18n'

const auth = useAuthStore()
const { t } = useI18n()
const tabs = computed(() =>
  [
    { to: 'members', label: t('admin.members'), icon: 'pi pi-users', show: auth.can(P.memberRead) },
    { to: 'tokens', label: t('admin.tokens'), icon: 'pi pi-key', show: auth.can(P.tokenManage) },
    { to: 'audit', label: t('admin.audit'), icon: 'pi pi-history', show: auth.can(P.auditRead) },
    { to: 'tenants', label: t('admin.tenants'), icon: 'pi pi-building', show: auth.isSuperAdmin },
    { to: 'users', label: t('admin.users'), icon: 'pi pi-id-card', show: auth.isSuperAdmin },
  ].filter((t) => t.show),
)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>{{ t('admin.title') }}</h1>
      <div class="sub">{{ auth.tenant?.name ?? t('admin.noTenant') }}</div>
    </div>
  </div>
  <nav class="tabs-nav" :aria-label="t('admin.sections')">
    <RouterLink v-for="tab in tabs" :key="tab.to" :to="{ name: tab.to }"><i :class="tab.icon" aria-hidden="true" />{{ tab.label }}</RouterLink>
  </nav>
  <RouterView />
</template>
