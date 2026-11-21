<script setup lang="ts">
// A project's icon: preset or uploaded image, else the key's first letters.
import { computed, ref, watch } from 'vue'
import { imageUrl } from '@/lib/avatars'
import { ICON_PRESETS } from '@/assets/presets'
import { projectColorStyle } from '@/lib/projectColors'

const props = withDefaults(defineProps<{ icon?: string | null; projectKey?: string; size?: number; label?: string; color?: string | null }>(), {
  icon: '',
  projectKey: '',
  size: 40,
  label: '',
  color: '',
})
const src = computed(() => imageUrl(props.icon, 'icon', ICON_PRESETS))
const broken = ref(false)
watch(src, () => (broken.value = false))
const style = computed(() => ({
  ...projectColorStyle(props.color),
  width: `${props.size}px`,
  height: `${props.size}px`,
  borderRadius: `${Math.round(props.size / 4)}px`,
  fontSize: `${Math.max(9, Math.round(props.size * 0.3))}px`,
}))
</script>

<template>
  <span class="project-icon" :style="style" :role="label ? 'img' : undefined" :aria-label="label || undefined" :aria-hidden="label ? undefined : 'true'">
    <img v-if="src && !broken" :src="src" alt="" :width="size" :height="size" @error="broken = true" />
    <span v-else>{{ (projectKey || '?').slice(0, size < 28 ? 2 : 4) }}</span>
  </span>
</template>

<style scoped>
.project-icon {
  display: inline-grid;
  place-items: center;
  flex: none;
  overflow: hidden;
  background: color-mix(in srgb, var(--p-primary-color) 15%, transparent);
  /* a little text colour keeps the initials at 4.5:1 on the tint in every theme (themes.test.ts) */
  color: color-mix(in srgb, var(--p-primary-color) 75%, var(--qtf-text));
  font-weight: 700;
  line-height: 1;
}
.project-icon img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
</style>
