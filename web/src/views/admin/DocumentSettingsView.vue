<script setup lang="ts">
// Administration → Documents: the organisation's logo on the first page of
// reports and generated documents (GET/PUT /settings/logo). Projects use it
// unless they upload their own or turn it off.
import { computed } from 'vue'
import LogoSettingsCard from '@/components/LogoSettingsCard.vue'
import { P } from '@/lib/permissions'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const canEdit = computed(() => auth.can(P.settingsManage))
</script>

<template>
  <LogoSettingsCard :can-edit="canEdit" style="margin-top: 0" />
</template>
