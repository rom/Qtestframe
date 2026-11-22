<script setup lang="ts">
// The in-app guides: the user guide (/help/:page?, docs/user-guide) and the
// administrator guide (/admin-guide/:page?, docs/admin-guide), rendered in the
// UI language (English when a page is missing), with contents, search and
// previous/next navigation.
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Drawer from 'primevue/drawer'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Message from 'primevue/message'
import { useBreakpoint } from '@/lib/breakpoint'
import { useAuthStore } from '@/stores/auth'
import { canSeeAdminGuide } from './access'
import { queryTerms, splitByTerms } from '@/lib/highlight'
import { ASSETS, guideContent } from './content'
import { helpPath as bookPath, type GuideBook } from './links'
import { neighbours, normalizePageId } from './pages'
import { renderGuide } from './render'
import { searchGuide } from './search'

const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()

const lang = computed(() => String(locale.value || 'en'))
/** the guide this route shows */
const book = computed<GuideBook>(() => (route.name === 'adminGuide' ? 'admin' : 'user'))
const content = computed(() => guideContent(book.value))
/** a page of the guide on screen */
const helpPath = (page: string, hash = '') => bookPath(page, hash, book.value)
const pageId = computed(() => normalizePageId(route.params.page))
// administrators can switch between the two guides
const auth = useAuthStore()
const otherGuide = computed<GuideBook | null>(() => (book.value === 'admin' ? 'user' : canSeeAdminGuide(auth.me) ? 'admin' : null))
const resolved = computed(() => content.value.getPage(lang.value, pageId.value))
const rendered = computed(() =>
  resolved.value
    ? renderGuide(resolved.value.markdown, {
        lang: resolved.value.lang,
        assets: ASSETS,
        book: book.value,
        tableLabel: t('guide.table'),
        missingImage: (alt) => t('guide.missingImage', { alt }),
      })
    : null,
)
const order = computed(() => content.value.getOrder(lang.value))
const titles = computed(() => content.value.getTitles(lang.value))
const pager = computed(() => neighbours(order.value, pageId.value))

const narrow = useBreakpoint('tablet')
const tocOpen = ref(false)

// ---------- search ----------
const query = ref('')
const hits = computed(() => (query.value.trim() ? searchGuide(content.value.getSections(lang.value), query.value, { order: order.value, limit: 20 }) : []))
const marks = computed(() => queryTerms(query.value))
function clearSearch() {
  query.value = ''
}

// ---------- navigation inside the guide ----------
const article = ref<HTMLElement>()

function focusAnchor(hash: string) {
  const id = decodeURIComponent(hash.replace(/^#/, ''))
  if (!id || !article.value) return false
  const el = article.value.querySelector<HTMLElement>(`[id="${CSS.escape(id)}"]`)
  if (!el) return false
  if (!el.hasAttribute('tabindex')) el.setAttribute('tabindex', '-1')
  el.scrollIntoView({ block: 'start' })
  el.focus({ preventScroll: true })
  return true
}

/** Guide links are plain <a> in the rendered HTML; route them through the router (no inline handlers: CSP). */
function onArticleClick(e: MouseEvent) {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
  const a = (e.target as Element | null)?.closest?.('a')
  if (!a || !article.value?.contains(a)) return
  const page = a.getAttribute('data-guide-page')
  const hash = a.getAttribute('data-guide-hash') ?? ''
  if (page) {
    e.preventDefault()
    router.push(bookPath(page, hash, a.getAttribute('data-guide-book') === 'admin' ? 'admin' : 'user'))
  } else if (hash) {
    e.preventDefault()
    router.push({ hash: '#' + encodeURIComponent(hash) }).then(() => focusAnchor(hash))
  } else if (a.hasAttribute('data-guide-app')) {
    e.preventDefault()
    router.push(a.getAttribute('href') ?? '/')
  }
}

function goTo(page: string, anchor = '') {
  tocOpen.value = false
  query.value = ''
  router.push(helpPath(page, anchor))
}

watch(
  () => [book.value, pageId.value, route.hash] as const,
  async ([b, , hash], old) => {
    await nextTick()
    if (hash && focusAnchor(hash)) return
    if (old && (old[0] !== b || old[1] !== pageId.value)) window.scrollTo?.({ top: 0 })
  },
)
onMounted(async () => {
  await nextTick()
  if (route.hash) focusAnchor(route.hash)
})
</script>

<template>
  <div class="guide" :class="{ narrow }">
    <aside v-if="!narrow" class="guide-toc card">
      <nav :aria-label="t(book === 'admin' ? 'guide.adminContentsNav' : 'guide.contentsNav')">
        <h2 class="guide-toc-title">{{ t('guide.contents') }}</h2>
        <ol class="guide-pages">
          <li v-for="id in order" :key="id">
            <RouterLink :to="helpPath(id)" :aria-current="id === pageId ? 'page' : undefined" :class="{ active: id === pageId }">{{ titles[id] }}</RouterLink>
            <ul v-if="id === pageId && rendered?.headings.length" class="guide-headings" :aria-label="t('guide.onThisPage')">
              <li v-for="h in rendered.headings.filter((x) => x.level === 2)" :key="h.id">
                <RouterLink :to="helpPath(id, h.id)">{{ h.text }}</RouterLink>
              </li>
            </ul>
          </li>
        </ol>
        <RouterLink v-if="otherGuide" :to="bookPath('index', '', otherGuide)" class="guide-switch">
          <i class="pi pi-book" aria-hidden="true" /> {{ t(otherGuide === 'admin' ? 'guide.adminGuide' : 'guide.userGuide') }}
        </RouterLink>
      </nav>
    </aside>

    <div class="guide-main">
      <div class="guide-toolbar">
        <Button
          v-if="narrow"
          :label="t('guide.contents')"
          icon="pi pi-list"
          severity="secondary"
          outlined
          aria-haspopup="dialog"
          :aria-expanded="tocOpen"
          @click="tocOpen = true"
        />
        <form class="guide-search" role="search" @submit.prevent>
          <IconField>
            <InputIcon class="pi pi-search" aria-hidden="true" />
            <InputText v-model="query" type="search" class="w-full" :placeholder="t('guide.searchPlaceholder')" :aria-label="t('guide.search')" />
          </IconField>
        </form>
      </div>

      <section v-if="query.trim()" class="card guide-results" aria-labelledby="guide-results-h">
        <div class="row" style="justify-content: space-between">
          <h2 id="guide-results-h" class="plain-heading" aria-live="polite">{{ t('guide.searchResults', hits.length) }}</h2>
          <Button :label="t('guide.clearSearch')" icon="pi pi-times" text size="small" severity="secondary" @click="clearSearch" />
        </div>
        <p v-if="!hits.length" class="muted">{{ t('guide.searchNothing', { query: query.trim() }) }}</p>
        <ol v-else class="guide-hits">
          <li v-for="h in hits" :key="h.page + '#' + h.anchor">
            <a :href="helpPath(h.page, h.anchor)" @click.prevent="goTo(h.page, h.anchor)">
              {{ h.title }}<template v-if="h.heading"> › {{ h.heading }}</template>
            </a>
            <p class="small muted">
              <template v-for="(s, i) in splitByTerms(h.snippet, marks)" :key="i"><mark v-if="s.mark">{{ s.text }}</mark><template v-else>{{ s.text }}</template></template>
            </p>
          </li>
        </ol>
      </section>

      <template v-if="resolved && rendered">
        <Message v-if="resolved.fallback" severity="info" :closable="false" class="guide-fallback">{{ t('guide.fallback') }}</Message>
        <!-- sanitised by DOMPurify in render.ts -->
        <!-- eslint-disable-next-line vue/no-v-html -->
        <article ref="article" class="card guide-article" :lang="resolved.lang" @click="onArticleClick" v-html="rendered.html" />
        <nav v-if="pager.prev || pager.next" class="guide-pager" :aria-label="t('guide.pager')">
          <RouterLink v-if="pager.prev" :to="helpPath(pager.prev)" class="prev" rel="prev">
            <i class="pi pi-arrow-left" aria-hidden="true" /> <span>{{ t('guide.previous', { title: titles[pager.prev] }) }}</span>
          </RouterLink>
          <RouterLink v-if="pager.next" :to="helpPath(pager.next)" class="next" rel="next">
            <span>{{ t('guide.next', { title: titles[pager.next] }) }}</span> <i class="pi pi-arrow-right" aria-hidden="true" />
          </RouterLink>
        </nav>
      </template>
      <div v-else class="card">
        <h1 tabindex="-1">{{ t('guide.help') }}</h1>
        <p>{{ t('guide.notFound') }}</p>
        <RouterLink :to="helpPath('index')">{{ t('guide.backToIndex') }}</RouterLink>
      </div>
    </div>

    <Drawer v-if="narrow" v-model:visible="tocOpen" position="left" :header="t('guide.contents')" class="guide-toc-drawer" block-scroll>
      <nav :aria-label="t(book === 'admin' ? 'guide.adminContentsNav' : 'guide.contentsNav')">
        <ol class="guide-pages">
          <li v-for="id in order" :key="id">
            <a :href="helpPath(id)" :aria-current="id === pageId ? 'page' : undefined" :class="{ active: id === pageId }" @click.prevent="goTo(id)">{{ titles[id] }}</a>
            <ul v-if="id === pageId && rendered?.headings.length" class="guide-headings" :aria-label="t('guide.onThisPage')">
              <li v-for="h in rendered.headings.filter((x) => x.level === 2)" :key="h.id">
                <a :href="helpPath(id, h.id)" @click.prevent="goTo(id, h.id)">{{ h.text }}</a>
              </li>
            </ul>
          </li>
        </ol>
        <RouterLink v-if="otherGuide" :to="bookPath('index', '', otherGuide)" class="guide-switch" @click="tocOpen = false">
          <i class="pi pi-book" aria-hidden="true" /> {{ t(otherGuide === 'admin' ? 'guide.adminGuide' : 'guide.userGuide') }}
        </RouterLink>
      </nav>
    </Drawer>
  </div>
</template>

<style>
.guide {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}
.guide.narrow {
  grid-template-columns: minmax(0, 1fr);
}
.guide-toc {
  position: sticky;
  top: calc(var(--qtf-topbar-h, 56px) + 12px);
  max-height: calc(100vh - var(--qtf-topbar-h, 56px) - 32px);
  overflow: auto;
  padding: 12px;
}
.guide-toc-title {
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--qtf-muted);
  margin: 4px 8px 8px;
}
.guide-pages,
.guide-headings {
  list-style: none;
  margin: 0;
  padding: 0;
}
.guide-pages a {
  display: block;
  padding: 5px 8px;
  border-radius: 6px;
  color: var(--qtf-text);
  text-decoration: none;
  font-size: 14px;
  line-height: 1.3;
}
.guide-pages a:hover {
  background: var(--qtf-hover);
}
.guide-pages a.active {
  background: color-mix(in srgb, var(--p-primary-color) 14%, transparent);
  color: var(--qtf-text);
  font-weight: 600;
}
.guide-switch {
  display: flex;
  gap: 6px;
  align-items: center;
  margin: 12px 8px 4px;
  padding-top: 10px;
  border-top: 1px solid var(--qtf-border);
  font-size: 14px;
  color: var(--p-primary-color);
}
.guide-headings a {
  padding-left: 20px;
  font-size: 13px;
  color: var(--qtf-muted);
}
.guide-main {
  min-width: 0;
  display: grid;
  gap: 16px;
}
.guide-toolbar {
  display: flex;
  gap: 8px;
  align-items: center;
}
.guide-search {
  flex: 1;
  max-width: 520px;
}
.guide-hits {
  margin: 8px 0 0;
  padding-left: 20px;
}
.guide-hits li {
  margin-bottom: 10px;
}
.guide-hits p {
  margin: 2px 0 0;
}
.guide-hits mark {
  background: color-mix(in srgb, var(--p-primary-color) 25%, transparent);
  color: inherit;
  border-radius: 2px;
}
.guide-article {
  padding: 24px clamp(16px, 3vw, 40px) 32px;
  line-height: 1.6;
  color: var(--qtf-text);
  overflow-wrap: break-word;
  max-width: 980px;
}
.guide-article h1 {
  font-size: clamp(22px, 2vw + 12px, 28px);
  margin: 0 0 12px;
  line-height: 1.25;
}
.guide-article h2 {
  font-size: 20px;
  margin: 32px 0 10px;
  padding-top: 8px;
  border-top: 1px solid var(--qtf-border);
}
.guide-article h3 {
  font-size: 16px;
  margin: 24px 0 8px;
}
.guide-article h1,
.guide-article h2,
.guide-article h3,
.guide-article h4 {
  scroll-margin-top: calc(var(--qtf-topbar-h, 56px) + 12px);
}
.guide-article a {
  color: var(--p-primary-color);
  text-decoration: underline;
  text-underline-offset: 2px;
}
.guide-article img {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 12px 0;
  border: 1px solid var(--qtf-border);
  border-radius: var(--qtf-radius, 8px);
  box-shadow: 0 1px 3px rgb(0 0 0 / 0.08);
}
.guide-article code {
  font-family: var(--qtf-mono);
  font-size: 0.9em;
  background: var(--qtf-hover);
  padding: 1px 5px;
  border-radius: 4px;
}
.guide-article pre {
  background: var(--qtf-bg);
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 12px 14px;
  overflow-x: auto;
  line-height: 1.45;
}
.guide-article pre:focus-visible {
  outline: 2px solid var(--qtf-focus, var(--p-primary-color));
  outline-offset: 2px;
}
.guide-article pre code {
  background: none;
  padding: 0;
}
.guide-article blockquote {
  margin: 12px 0;
  padding: 8px 14px;
  border-left: 4px solid var(--p-primary-color);
  background: var(--qtf-hover);
  border-radius: 0 8px 8px 0;
}
.guide-article blockquote p {
  margin: 4px 0;
}
.guide-article kbd {
  font-family: var(--qtf-mono);
  font-size: 0.85em;
  border: 1px solid var(--qtf-border);
  border-bottom-width: 2px;
  border-radius: 4px;
  padding: 0 5px;
  background: var(--qtf-card);
}
.guide-table {
  overflow-x: auto;
  margin: 12px 0;
}
.guide-table:focus-visible {
  outline: 2px solid var(--qtf-focus, var(--p-primary-color));
}
.guide-article table {
  border-collapse: collapse;
  min-width: 100%;
  font-size: 14px;
}
.guide-article th,
.guide-article td {
  border: 1px solid var(--qtf-border);
  padding: 6px 10px;
  text-align: left;
  vertical-align: top;
}
.guide-article th {
  background: var(--qtf-hover);
}
.guide-image-missing {
  display: block;
  padding: 16px;
  margin: 12px 0;
  border: 1px dashed var(--qtf-border);
  border-radius: 8px;
  color: var(--qtf-muted);
  font-style: italic;
}
.guide-pager {
  display: flex;
  gap: 12px;
  justify-content: space-between;
  flex-wrap: wrap;
  max-width: 980px;
}
.guide-pager a {
  display: inline-flex;
  gap: 8px;
  align-items: center;
  padding: 10px 14px;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  background: var(--qtf-card);
  color: var(--qtf-text);
  text-decoration: none;
  max-width: 100%;
}
.guide-pager a:hover {
  border-color: var(--p-primary-color);
}
.guide-pager .next {
  margin-left: auto;
}
</style>
