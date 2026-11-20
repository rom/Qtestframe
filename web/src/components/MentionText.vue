<script setup lang="ts">
// Text with "@handle" mentions shown as name chips (avatar + name) for
// people the project knows; unknown handles stay as written.
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import UserAvatar from './UserAvatar.vue'
import { splitMentions } from '@/lib/mentions'
import { useProjectPeopleStore } from '@/stores/projectPeople'

const props = defineProps<{ text: string | null | undefined; projectId?: string }>()
const route = useRoute()
const store = useProjectPeopleStore()
const pid = computed(() => props.projectId ?? (typeof route.params.projectId === 'string' ? route.params.projectId : ''))
const hasMention = computed(() => (props.text ?? '').includes('@'))
const segments = computed(() => (hasMention.value ? splitMentions(props.text ?? '', store.people) : [{ text: props.text ?? '' }]))
onMounted(() => {
  if (hasMention.value && pid.value) void store.ensure(pid.value)
})
</script>

<template>
  <span class="mention-text"
    ><template v-for="(s, i) in segments" :key="i"
      ><span v-if="s.person" class="mention-chip" :title="s.person.email ? `@${s.person.handle} · ${s.person.email}` : `@${s.person.handle}`"
        ><UserAvatar :avatar="s.person.avatar ?? ''" :user-id="s.person.user_id" :email="s.person.email" :name="s.person.name" :size="16" /><span>{{ s.person.name }}</span></span
      ><span v-else-if="s.handle" class="mention-chip unknown">{{ s.text }}</span
      ><template v-else>{{ s.text }}</template></template
    ></span
  >
</template>

<style>
.mention-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 0 6px 0 2px;
  border-radius: 999px;
  background: var(--qtf-hover);
  border: 1px solid var(--qtf-border);
  font-weight: 600;
  white-space: nowrap;
  vertical-align: baseline;
  line-height: 1.5;
}
.mention-chip.unknown {
  padding: 0 6px;
  font-weight: normal;
}
</style>
