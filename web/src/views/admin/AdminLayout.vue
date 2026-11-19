<script setup lang="ts">
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { P } from '@/lib/permissions'

const auth = useAuthStore()
const tabs = computed(() =>
  [
    { to: 'members', label: 'Members', icon: 'pi pi-users', show: auth.can(P.memberRead) },
    { to: 'tokens', label: 'API tokens', icon: 'pi pi-key', show: auth.can(P.tokenManage) },
    { to: 'audit', label: 'Audit log', icon: 'pi pi-history', show: auth.can(P.auditRead) },
    { to: 'tenants', label: 'Tenants', icon: 'pi pi-building', show: auth.isSuperAdmin },
    { to: 'users', label: 'Users', icon: 'pi pi-id-card', show: auth.isSuperAdmin },
  ].filter((t) => t.show),
)
</script>

<template>
  <div class="page-header">
    <div>
      <h1>Administration</h1>
      <div class="sub">{{ auth.tenant?.name ?? 'No tenant selected' }}</div>
    </div>
  </div>
  <nav class="tabs-nav">
    <RouterLink v-for="t in tabs" :key="t.to" :to="{ name: t.to }"><i :class="t.icon" />{{ t.label }}</RouterLink>
  </nav>
  <RouterView />
</template>
