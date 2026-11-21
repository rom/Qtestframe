<script setup lang="ts">
// During the grace period of an MFA policy: set up a second factor before
// the deadline (afterwards it is required at sign-in).
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import Message from 'primevue/message'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { daysLeft } from '@/lib/mfa'
import { formatDate } from '@/lib/format'

const auth = useAuthStore()
const route = useRoute()
const { t } = useI18n()
const mfa = computed(() => auth.me?.mfa)
const days = computed(() => daysLeft(mfa.value?.enrolment_deadline))
const show = computed(() => !!mfa.value?.required && !mfa.value.enabled && !mfa.value.enrolment_required && !!days.value && route.name !== 'profile')
</script>

<template>
  <Message v-if="show" severity="warn" :closable="false" class="mfa-banner">
    {{ t('mfa.requiredBy', { date: formatDate(mfa?.enrolment_deadline), n: days ?? 0 }, days ?? 0) }}
    <RouterLink :to="{ name: 'profile', hash: '#mfa' }">{{ t('mfa.setUpNow') }}</RouterLink>
  </Message>
</template>

<style scoped>
.mfa-banner {
  margin-bottom: 16px;
}
</style>
