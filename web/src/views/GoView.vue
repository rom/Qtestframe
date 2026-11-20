<script setup lang="ts">
// /go/:key — opens a requirement or test by its key (links in texts, in
// e-mails or chats). Looks the key up with the search API and replaces the
// route with the item's editor.
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import { api, asItems, errorMessage } from '@/lib/api'
import { useI18n } from 'vue-i18n'

interface Hit {
  type: string
  id: string
  project_id: string
  key: string
}
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const error = ref('')
const notFound = ref(false)

async function resolve() {
  const key = String(route.params.key ?? '').toUpperCase()
  error.value = ''
  notFound.value = false
  try {
    const hits = asItems(await api.get<{ items: Hit[] }>('/search', { q: `key:"${key}"`, types: 'requirement,test', limit: 20 }))
    const hit = hits.find((h) => h.key.toUpperCase() === key)
    if (!hit) {
      notFound.value = true
      return
    }
    await router.replace({ name: hit.type === 'requirement' ? 'requirements' : 'tests', params: { projectId: hit.project_id }, query: { item: hit.id } })
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(resolve)
watch(() => route.params.key, resolve)
</script>

<template>
  <div class="card" style="max-width: 560px">
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <Message v-else-if="notFound" severity="warn" :closable="false">
      {{ t('links.notFound', { key: String(route.params.key ?? '') }) }}
      <RouterLink :to="{ name: 'search', query: { q: String(route.params.key ?? '') } }">{{ t('links.searchInstead') }}</RouterLink>
    </Message>
    <div v-else class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
  </div>
</template>
