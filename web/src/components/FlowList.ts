// The list view of a test flow (FlowView): the main path as an ordered list
// with the "otherwise" branch of every check; the steps of expanded shared
// steps are nested lists.
import { defineComponent, h, type PropType, type VNode } from 'vue'
import { t as tr } from '@/i18n'
import type { FlowNode } from '@/lib/testFlow'

export interface FlowListItem {
  node: FlowNode
  fail?: FlowNode
  children: FlowListItem[]
}

function renderList(items: FlowListItem[], nested: boolean): VNode {
  return h(
    'ol',
    { class: nested ? 'flow-list nested' : 'flow-list' },
    items.map((it) => h('li', { key: it.node.id }, renderItem(it))),
  )
}

function renderItem(it: FlowListItem): VNode[] {
  const n = it.node
  const out: VNode[] = []
  const head = n.kind === 'call' ? `${tr('flow.calls')} ${n.text}` : n.text
  out.push(h('span', { class: 'flow-li-kind k-' + n.kind }, [n.number ? h('b', n.number + '. ') : null, head]))
  if (n.detail) out.push(h('span', { class: 'muted' }, ' — ' + n.detail))
  if (n.note && n.note !== 'missing') out.push(h('span', { class: 'muted' }, ` (${tr('flow.' + n.note)})`))
  if (it.fail) out.push(h('div', { class: 'small' }, [h('span', { class: 'muted' }, tr('flow.otherwise') + ' '), h('b', { class: 'flow-fail-text' }, it.fail.text)]))
  if (it.children.length) out.push(renderList(it.children, true))
  return out
}

export default defineComponent({
  name: 'FlowList',
  props: { items: { type: Array as PropType<FlowListItem[]>, required: true } },
  setup(props) {
    return () => renderList(props.items, false)
  },
})
