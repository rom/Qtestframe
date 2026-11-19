<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Textarea from 'primevue/textarea'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Avatar from 'primevue/avatar'
import ProgressSpinner from 'primevue/progressspinner'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDateTime, formatFullDateTime, initials } from '@/lib/format'
import { P } from '@/lib/permissions'
import type { Comment } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ entity: 'requirement' | 'test'; entityId: string }>()
const emit = defineEmits<{ count: [n: number] }>()
const auth = useAuthStore()
const notify = useNotify()

const MAX = 10000
const comments = ref<Comment[]>([])
const loading = ref(false)
const error = ref('')
const draft = ref('')
const posting = ref(false)
const editing = ref<string | null>(null)
const editText = ref('')
const base = computed(() => `/${props.entity === 'requirement' ? 'requirements' : 'tests'}/${props.entityId}/comments`)

async function load() {
  loading.value = true
  error.value = ''
  try {
    comments.value = asItems(await api.get<Comment[]>(base.value)).sort((a, b) => a.created_at.localeCompare(b.created_at))
    emit('count', comments.value.length)
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
watch(() => props.entityId, load, { immediate: true })

const mine = (c: Comment) => !!auth.user && c.user_id === auth.user.id
const canDelete = (c: Comment) => mine(c) || auth.can(P.memberManage)
const edited = (c: Comment) => !!c.updated_at && c.updated_at !== c.created_at && new Date(c.updated_at).getTime() - new Date(c.created_at).getTime() > 1000

async function post() {
  const body = draft.value.trim()
  if (!body) return
  posting.value = true
  try {
    const c = await api.post<Comment>(base.value, { body })
    if (c?.id) comments.value.push(c)
    else await load()
    draft.value = ''
    emit('count', comments.value.length)
  } catch (e) {
    notify.fail(e, 'Could not add comment')
  } finally {
    posting.value = false
  }
}
function startEdit(c: Comment) {
  editing.value = c.id
  editText.value = c.body
}
async function saveEdit(c: Comment) {
  const body = editText.value.trim()
  if (!body) return
  try {
    const u = await api.patch<Comment>(`/comments/${c.id}`, { body })
    Object.assign(c, u ?? { body })
    editing.value = null
  } catch (e) {
    notify.fail(e, 'Could not update comment')
  }
}
function remove(c: Comment) {
  notify.confirmDanger('Delete this comment?', async () => {
    try {
      await api.del(`/comments/${c.id}`)
      comments.value = comments.value.filter((x) => x.id !== c.id)
      emit('count', comments.value.length)
    } catch (e) {
      notify.fail(e)
    }
  })
}
function onKey(e: KeyboardEvent) {
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault()
    post()
  }
}
</script>

<template>
  <div>
    <Message v-if="error" severity="error" :closable="false">Comments are unavailable: {{ error }}</Message>
    <div v-else-if="loading && !comments.length" class="empty"><ProgressSpinner style="width: 30px; height: 30px" /></div>
    <template v-else>
      <div v-if="!comments.length" class="muted small" style="padding: 8px 0">No comments yet. Start the discussion below.</div>
      <div v-for="c in comments" :key="c.id" class="comment">
        <Avatar :label="initials(c.user_name || c.user_email)" shape="circle" size="normal" style="flex: none" />
        <div class="grow">
          <div class="row" style="gap: 6px">
            <b>{{ c.user_name || c.user_email || 'Unknown' }}</b>
            <span class="muted small" :title="formatFullDateTime(c.created_at)">{{ formatDateTime(c.created_at) }}</span>
            <span v-if="edited(c)" class="muted small" :title="'Edited ' + formatFullDateTime(c.updated_at)">(edited)</span>
            <span class="grow" />
            <Button v-if="mine(c) && editing !== c.id" icon="pi pi-pencil" text rounded size="small" severity="secondary" aria-label="Edit comment" @click="startEdit(c)" />
            <Button v-if="canDelete(c)" icon="pi pi-trash" text rounded size="small" severity="danger" aria-label="Delete comment" @click="remove(c)" />
          </div>
          <div v-if="editing === c.id" class="grid" style="gap: 6px; margin-top: 4px">
            <Textarea v-model="editText" rows="2" auto-resize :maxlength="MAX" />
            <div class="row" style="justify-content: flex-end">
              <Button label="Cancel" text size="small" severity="secondary" @click="editing = null" />
              <Button label="Save" icon="pi pi-check" size="small" :disabled="!editText.trim()" @click="saveEdit(c)" />
            </div>
          </div>
          <div v-else class="body">{{ c.body }}</div>
        </div>
      </div>
      <div class="grid" style="gap: 6px; margin-top: 12px">
        <Textarea v-model="draft" rows="3" auto-resize placeholder="Write a comment… (Ctrl+Enter to post)" :maxlength="MAX" @keydown="onKey" />
        <div class="row" style="justify-content: flex-end">
          <span v-if="draft.length > MAX - 500" class="small muted">{{ draft.length }}/{{ MAX }}</span>
          <Button label="Comment" icon="pi pi-send" size="small" :loading="posting" :disabled="!draft.trim()" @click="post" />
        </div>
      </div>
    </template>
  </div>
</template>
