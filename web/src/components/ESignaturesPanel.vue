<script setup lang="ts">
// The electronic signatures of a record (its signature manifestation):
// printed name, date and time in UTC and in the signer's time zone, the
// meaning, what was signed and whether the record still has the signed
// content. Shown only when there is a signature (or always, with `always`).
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Tag from 'primevue/tag'
import { api, asItems } from '@/lib/api'
import { meaningLabel, short, utcStamp, zoneStamp, type ESignature } from '@/lib/erecords'

const props = withDefaults(defineProps<{ entityType: string; entityId: string; always?: boolean; stamp?: string | number }>(), { always: false, stamp: '' })
const emit = defineEmits<{ count: [n: number] }>()
const { t, te } = useI18n()
const list = ref<ESignature[]>([])
const loaded = ref(false)

async function load() {
  try {
    list.value = asItems(await api.get<{ items: ESignature[] }>('/esignatures', { entity_type: props.entityType, entity_id: props.entityId }))
  } catch {
    list.value = []
  }
  loaded.value = true
  emit('count', list.value.length)
}
onMounted(load)
watch(() => [props.entityId, props.stamp], load)
defineExpose({ load })

const actionName = (a: string) => (te(`erecords.actionName.${a}`) ? t(`erecords.actionName.${a}`) : a)
const methodName = (m: string) => (te(`erecords.method.${m.replace('+', '_')}`) ? t(`erecords.method.${m.replace('+', '_')}`) : m)
</script>

<template>
  <section v-if="loaded && (always || list.length)" class="esigs" :aria-label="t('erecords.signatures.title')">
    <h3 class="small esigs-title"><i class="pi pi-pen-to-square" aria-hidden="true" /> {{ t('erecords.signatures.title') }}</h3>
    <p v-if="!list.length" class="small muted">{{ t('erecords.signatures.none') }}</p>
    <ul v-else class="esig-list">
      <li v-for="s in list" :key="s.id" class="esig">
        <div class="row" style="gap: 8px; flex-wrap: wrap">
          <Tag :value="meaningLabel(t, s.meaning)" severity="info" />
          <b>{{ s.user_name || s.user_email }}</b>
          <span class="small muted">{{ s.user_email }}</span>
          <span class="grow" />
          <Tag v-if="s.valid" :value="t('erecords.signatures.valid')" severity="success" icon="pi pi-check" />
          <Tag v-else :value="t('erecords.signatures.changed')" severity="danger" icon="pi pi-exclamation-triangle" />
        </div>
        <dl class="esig-kv small">
          <dt>{{ t('erecords.signatures.when') }}</dt>
          <dd>
            <time :datetime="s.signed_at">{{ utcStamp(s.signed_at) }}</time>
            <span v-if="zoneStamp(s.signed_at, s.time_zone)" class="muted"> · {{ zoneStamp(s.signed_at, s.time_zone) }}</span>
          </dd>
          <dt>{{ t('erecords.signatures.what') }}</dt>
          <dd>{{ actionName(s.action) }} · {{ methodName(s.method) }}</dd>
          <template v-if="s.comment">
            <dt>{{ t('esign.comment') }}</dt>
            <dd class="esig-comment">{{ s.comment }}</dd>
          </template>
          <dt>{{ t('erecords.signatures.hash') }}</dt>
          <dd class="mono" :title="s.content_hash">{{ short(s.content_hash) }}<span v-if="!s.valid && s.current_hash" class="muted"> → {{ short(s.current_hash) }}</span></dd>
        </dl>
      </li>
    </ul>
  </section>
</template>

<style scoped>
.esigs {
  margin: 8px 0 12px;
  padding: 8px 12px;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
}
.esigs-title {
  margin: 0 0 6px;
  font-weight: 600;
}
.esig-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.esig {
  padding: 6px 0;
  border-top: 1px solid var(--qtf-border);
}
.esig:first-child {
  border-top: 0;
}
.esig-kv {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 2px 12px;
  margin: 4px 0 0;
}
.esig-kv dt {
  color: var(--qtf-muted);
}
.esig-kv dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.esig-comment {
  white-space: pre-wrap;
}
</style>
