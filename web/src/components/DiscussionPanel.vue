<script setup lang="ts">
// The discussion of an item: threads of Markdown comments with replies (one
// level), @mentions with autocomplete, edit and remove (own comments;
// moderators remove anyone's), resolve and reopen, reactions. Resolved
// threads are folded away until "Show resolved" is on.
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import Menu from 'primevue/menu'
import ProgressSpinner from 'primevue/progressspinner'
import MentionInput from './MentionInput.vue'
import UserAvatar from './UserAvatar.vue'
import { api, asItems } from '@/lib/api'
import { formatDateTime, formatFullDateTime } from '@/lib/format'
import { P } from '@/lib/permissions'
import type { Comment, DiscussionKind } from '@/lib/types'
import { commentsPath, groupThreads, liveCount, REACTIONS, reactionList, wasEdited, type Thread } from '@/lib/discussions'
import { renderComment } from '@/lib/commentMarkdown'
import { uid } from '@/lib/a11y'
import { useAuthStore } from '@/stores/auth'
import { useProjectPeopleStore } from '@/stores/projectPeople'
import { usePrefsStore } from '@/stores/prefs'
import { rememberValues } from '@/lib/suggest'
import { useNotify } from '@/composables/notify'

const props = defineProps<{ entity: DiscussionKind; entityId: string; projectId?: string; compact?: boolean }>()
const emit = defineEmits<{ count: [n: number]; open: [n: number] }>()
const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const people = useProjectPeopleStore()
const notify = useNotify()

const MAX = 10000
const comments = ref<Comment[]>([])
const loading = ref(false)
const error = ref('')
const draft = ref('')
const posting = ref(false)
const showResolved = ref(false)
const replyTo = ref<string | null>(null)
const replyText = ref('')
const editing = ref<string | null>(null)
const editText = ref('')
const reactMenu = ref<InstanceType<typeof Menu> | null>(null)
const reactingTo = ref<Comment | null>(null)
const headingId = uid('discussion')
const resolvedId = uid('show-resolved')

const pid = computed(() => props.projectId ?? (typeof route.params.projectId === 'string' ? route.params.projectId : ''))
const path = computed(() => commentsPath(props.entity, props.entityId))
const threads = computed(() => groupThreads(comments.value).filter((th) => !th.removed))
const openThreads = computed(() => threads.value.filter((th) => th.open))
const resolvedThreads = computed(() => threads.value.filter((th) => !th.open))
const shown = computed(() => (showResolved.value ? threads.value : openThreads.value))
const canWrite = computed(() => auth.can(P.commentCreate))
const moderator = computed(() => auth.can(P.projectUpdate) || auth.can(P.memberManage))
const me = computed(() => auth.user?.id)

function report() {
  emit('count', liveCount(comments.value))
  emit('open', openThreads.value.length)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    comments.value = asItems(await api.get<Comment[]>(path.value))
    report()
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}
watch(() => [props.entity, props.entityId], load, { immediate: true })
watch(pid, (p) => p && void people.ensure(p), { immediate: true })

const html = (c: Comment) => renderComment(c.body, people.people, t('links.newTab'))
const mine = (c: Comment) => !!me.value && c.user_id === me.value
const canRemove = (c: Comment) => !c.deleted && ((mine(c) && canWrite.value) || moderator.value)
const author = (c: Comment) => c.user_name || c.user_email || t('common.unknown')

function upsert(c: Comment) {
  const i = comments.value.findIndex((x) => x.id === c.id)
  if (i >= 0) comments.value[i] = { ...comments.value[i], ...c }
  else comments.value.push(c)
  report()
}

async function post(body: string, parent = ''): Promise<boolean> {
  const text = body.trim()
  if (!text) return false
  posting.value = true
  try {
    const c = await api.post<Comment>(path.value, { body: text, parent_id: parent || undefined })
    if (c?.id) upsert(c)
    else await load()
    if (pid.value) void rememberValues(usePrefsStore().prefs, pid.value, [{ field: 'comment', value: text }])
    return true
  } catch (e) {
    notify.fail(e, t('comments.addFailed'))
    return false
  } finally {
    posting.value = false
  }
}
async function postThread() {
  if (await post(draft.value)) draft.value = ''
}
function startReply(th: Thread) {
  replyTo.value = th.root.id
  replyText.value = ''
}
async function postReply(th: Thread) {
  if (await post(replyText.value, th.root.id)) {
    replyTo.value = null
    replyText.value = ''
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
    upsert(await api.patch<Comment>(`/comments/${c.id}`, { body }))
    editing.value = null
  } catch (e) {
    notify.fail(e, t('comments.updateFailed'))
  }
}
function remove(c: Comment) {
  const msg = mine(c) ? t('comments.confirmDelete') : t('discussions.confirmModerate', { name: author(c) })
  notify.confirmDanger(msg, async () => {
    try {
      await api.del(`/comments/${c.id}`)
      upsert({ ...c, body: '', deleted: true, mentions: [], reactions: {} })
    } catch (e) {
      notify.fail(e)
    }
  })
}
async function setResolved(th: Thread, resolved: boolean) {
  try {
    upsert(await api.post<Comment>(`/comments/${th.root.id}/${resolved ? 'resolve' : 'reopen'}`, {}))
    notify.ok(resolved ? t('discussions.resolvedToast') : t('discussions.reopenedToast'))
  } catch (e) {
    notify.fail(e)
  }
}
async function react(c: Comment, key: string) {
  try {
    const res = await api.post<{ reactions: Record<string, string[]> }>(`/comments/${c.id}/reactions`, { reaction: key })
    upsert({ ...c, reactions: res?.reactions ?? {} })
  } catch (e) {
    notify.fail(e)
  }
}
const reactionItems = computed(() =>
  REACTIONS.map((r) => ({ label: `${r.emoji} ${t(`discussions.reaction.${r.key === '+1' ? 'up' : r.key === '-1' ? 'down' : r.key}`)}`, command: () => reactingTo.value && react(reactingTo.value, r.key) })),
)
function openReactions(e: Event, c: Comment) {
  reactingTo.value = c
  reactMenu.value?.toggle(e)
}
const reactionLabel = (key: string) => t(`discussions.reaction.${key === '+1' ? 'up' : key === '-1' ? 'down' : key}`)

function onKey(e: KeyboardEvent, send: () => void) {
  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
    e.preventDefault()
    send()
  }
}
// links into the tool (item keys) without reloading the page
function onBodyClick(e: MouseEvent) {
  const a = (e.target as HTMLElement | null)?.closest?.('a[data-route]') as HTMLAnchorElement | null
  if (!a || e.ctrlKey || e.metaKey || e.shiftKey || e.button !== 0) return
  e.preventDefault()
  void router.push(a.getAttribute('href') ?? '/')
}
defineExpose({ reload: load })
</script>

<template>
  <section class="discussion" :aria-labelledby="headingId">
    <h3 :id="headingId" :class="compact ? 'small discussion-title' : 'sr-only'">
      <i v-if="compact" class="pi pi-comments" aria-hidden="true" /> {{ t('discussions.title') }}
      <span v-if="compact && openThreads.length" class="count">{{ t('discussions.openCount', { n: openThreads.length }, openThreads.length) }}</span>
    </h3>
    <Message v-if="error" severity="error" :closable="false">{{ t('comments.unavailable', { error }) }}</Message>
    <div v-else-if="loading && !comments.length" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <template v-else>
      <div class="row discussion-bar">
        <span class="muted small">{{ t('discussions.summary', { open: openThreads.length, resolved: resolvedThreads.length }) }}</span>
        <span class="grow" />
        <template v-if="resolvedThreads.length">
          <ToggleSwitch v-model="showResolved" :input-id="resolvedId" />
          <label :for="resolvedId" class="small">{{ t('discussions.showResolved') }}</label>
        </template>
      </div>
      <div v-if="!shown.length" class="muted small" style="padding: 8px 0">{{ threads.length ? t('discussions.allResolved') : t('comments.none') }}</div>
      <article v-for="th in shown" :key="th.root.id" class="thread" :class="{ resolved: !th.open }" :aria-label="t('discussions.threadBy', { name: author(th.root) })">
        <div v-for="c in [th.root, ...th.replies]" :key="c.id" class="comment" :class="{ reply: c.id !== th.root.id }">
          <UserAvatar :avatar="c.user_avatar ?? ''" :user-id="c.user_id" :email="c.user_email" :name="author(c)" :size="c.id === th.root.id ? 32 : 26" />
          <div class="grow" style="min-width: 0">
            <div class="row" style="gap: 6px; flex-wrap: wrap">
              <b>{{ author(c) }}</b>
              <span class="muted small" :title="formatFullDateTime(c.created_at)">{{ formatDateTime(c.created_at) }}</span>
              <span v-if="wasEdited(c)" class="muted small" :title="t('comments.editedAt', { date: formatFullDateTime(c.updated_at) })">{{ t('comments.edited') }}</span>
              <Tag v-if="c.id === th.root.id && th.root.resolved_at" severity="success" :value="t('discussions.resolved')" :title="th.root.resolved_by_name ? t('discussions.resolvedBy', { name: th.root.resolved_by_name, date: formatFullDateTime(th.root.resolved_at) }) : ''" />
              <span class="grow" />
              <template v-if="!c.deleted && editing !== c.id">
                <Button v-if="canWrite" icon="pi pi-face-smile" text rounded size="small" severity="secondary" :aria-label="t('discussions.react')" v-tooltip="t('discussions.react')" aria-haspopup="true" @click="openReactions($event, c)" />
                <Button v-if="mine(c) && canWrite" icon="pi pi-pencil" text rounded size="small" severity="secondary" :aria-label="t('comments.edit')" v-tooltip="t('comments.edit')" @click="startEdit(c)" />
                <Button v-if="canRemove(c)" icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('comments.delete')" v-tooltip="t('comments.delete')" @click="remove(c)" />
              </template>
            </div>
            <p v-if="c.deleted" class="muted small removed"><i class="pi pi-ban" aria-hidden="true" /> {{ t('discussions.removed') }}</p>
            <div v-else-if="editing === c.id" class="grid" style="gap: 6px; margin-top: 4px">
              <MentionInput v-model="editText" :project-id="pid" multiline rows="2" auto-resize :maxlength="MAX" :aria-label="t('comments.edit')" @keydown="onKey($event, () => saveEdit(c))" />
              <div class="row" style="justify-content: flex-end">
                <Button :label="t('common.cancel')" text size="small" severity="secondary" @click="editing = null" />
                <Button :label="t('common.save')" icon="pi pi-check" size="small" :disabled="!editText.trim()" @click="saveEdit(c)" />
              </div>
            </div>
            <!-- sanitised by DOMPurify in lib/commentMarkdown.ts -->
            <div v-else class="comment-md" @click="onBodyClick" v-html="html(c)" />
            <div v-if="!c.deleted && reactionList(c, me).length" class="row reactions" style="gap: 4px; flex-wrap: wrap">
              <button
                v-for="r in reactionList(c, me)"
                :key="r.key"
                type="button"
                class="reaction"
                :class="{ mine: r.mine }"
                :aria-pressed="r.mine"
                :disabled="!canWrite"
                :aria-label="t('discussions.reactionCount', { reaction: reactionLabel(r.key), n: r.count }, r.count)"
                @click="react(c, r.key)"
              >
                <span aria-hidden="true">{{ r.emoji }} {{ r.count }}</span>
              </button>
            </div>
          </div>
        </div>
        <div class="row thread-actions">
          <Button v-if="canWrite && th.open && replyTo !== th.root.id" :label="t('discussions.reply')" icon="pi pi-reply" text size="small" @click="startReply(th)" />
          <span class="grow" />
          <Button v-if="canWrite && th.open" :label="t('discussions.resolve')" icon="pi pi-check-circle" text size="small" severity="success" @click="setResolved(th, true)" />
          <Button v-if="canWrite && !th.open" :label="t('discussions.reopen')" icon="pi pi-replay" text size="small" severity="secondary" @click="setResolved(th, false)" />
        </div>
        <div v-if="replyTo === th.root.id" class="grid reply-box" style="gap: 6px">
          <MentionInput v-model="replyText" :project-id="pid" multiline rows="2" auto-resize :maxlength="MAX" :placeholder="t('discussions.replyPlaceholder')" :aria-label="t('discussions.reply')" @keydown="onKey($event, () => postReply(th))" />
          <div class="row" style="justify-content: flex-end">
            <Button :label="t('common.cancel')" text size="small" severity="secondary" @click="replyTo = null" />
            <Button :label="t('discussions.reply')" icon="pi pi-send" size="small" :loading="posting" :disabled="!replyText.trim()" @click="postReply(th)" />
          </div>
        </div>
      </article>
      <div v-if="canWrite" class="grid" style="gap: 6px; margin-top: 12px">
        <MentionInput v-model="draft" :project-id="pid" suggest="comment" multiline rows="3" auto-resize :placeholder="t('discussions.placeholder')" :aria-label="t('discussions.newThread')" :maxlength="MAX" @keydown="onKey($event, postThread)" />
        <div class="row" style="justify-content: flex-end">
          <span class="small muted grow">{{ t('discussions.markdownHint') }}</span>
          <span v-if="draft.length > MAX - 500" class="small muted">{{ draft.length }}/{{ MAX }}</span>
          <Button :label="t('discussions.startThread')" icon="pi pi-send" size="small" :loading="posting" :disabled="!draft.trim()" @click="postThread" />
        </div>
      </div>
      <p v-else class="muted small">{{ t('discussions.readOnly') }}</p>
      <Menu ref="reactMenu" :model="reactionItems" popup />
    </template>
  </section>
</template>

<style>
.dialog-discussion {
  margin-top: 16px;
  padding-top: 8px;
  border-top: 1px solid var(--qtf-border);
}
.discussion .thread {
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 4px 10px;
  margin: 8px 0;
}
.discussion .thread.resolved {
  opacity: 0.85;
  border-style: dashed;
}
.discussion .thread .comment:last-of-type {
  border-bottom: none;
}
.discussion .comment.reply {
  margin-left: 24px;
}
.discussion .removed {
  font-style: italic;
  margin: 4px 0;
}
.discussion-title {
  margin: 0 0 4px;
  display: flex;
  align-items: center;
  gap: 6px;
}
.comment-md {
  margin-top: 2px;
  overflow-wrap: anywhere;
}
.comment-md p {
  margin: 0 0 6px;
}
.comment-md pre {
  overflow-x: auto;
  background: var(--qtf-hover);
  padding: 6px 8px;
  border-radius: 6px;
}
.comment-md blockquote {
  margin: 4px 0;
  padding-left: 10px;
  border-left: 3px solid var(--qtf-border);
  color: var(--qtf-muted);
}
.discussion .reaction {
  border: 1px solid var(--qtf-border);
  background: var(--qtf-card);
  color: var(--qtf-text);
  border-radius: 999px;
  padding: 0 8px;
  font: inherit;
  font-size: 12px;
  line-height: 22px;
  cursor: pointer;
}
.discussion .reaction.mine {
  border-color: var(--p-primary-color);
  background: color-mix(in srgb, var(--p-primary-color) 12%, transparent);
}
.discussion .reaction:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 1px;
}
.discussion .thread-actions {
  gap: 4px;
  padding-bottom: 4px;
}
.discussion .reply-box {
  margin: 0 0 8px 24px;
}
</style>
