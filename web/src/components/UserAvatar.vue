<script setup lang="ts">
// A user's avatar: preset or uploaded image, else initials. Decorative by
// default (the name is shown next to it); pass `label` to announce it.
import { computed, ref, watch } from 'vue'
import { imageUrl } from '@/lib/avatars'
import { initials } from '@/lib/format'
import { AVATAR_PRESETS } from '@/assets/presets'
import { usePeopleStore } from '@/stores/people'

const props = withDefaults(
  defineProps<{
    /** avatar value; when undefined it is looked up by userId / email */
    avatar?: string | null
    name?: string | null
    userId?: string | null
    email?: string | null
    size?: number
    label?: string
  }>(),
  { avatar: undefined, name: '', userId: null, email: null, size: 32, label: '' },
)
const people = usePeopleStore()
const value = computed(() => (props.avatar !== undefined && props.avatar !== null ? props.avatar : people.avatarFor({ id: props.userId, email: props.email })))
const src = computed(() => imageUrl(value.value, 'avatar', AVATAR_PRESETS))
const broken = ref(false)
watch(src, () => (broken.value = false))
const text = computed(() => initials(props.name || props.email))
const style = computed(() => ({ width: `${props.size}px`, height: `${props.size}px`, fontSize: `${Math.max(10, Math.round(props.size * 0.38))}px` }))
</script>

<template>
  <span class="user-avatar" :style="style" :role="label ? 'img' : undefined" :aria-label="label || undefined" :aria-hidden="label ? undefined : 'true'">
    <img v-if="src && !broken" :src="src" alt="" :width="size" :height="size" @error="broken = true" />
    <span v-else class="initials">{{ text }}</span>
  </span>
</template>

<style scoped>
.user-avatar {
  display: inline-grid;
  place-items: center;
  flex: none;
  border-radius: 50%;
  overflow: hidden;
  background: var(--p-primary-color);
  color: var(--p-primary-contrast-color);
  font-weight: 600;
  line-height: 1;
  user-select: none;
}
.user-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  background: var(--qtf-card);
}
</style>
