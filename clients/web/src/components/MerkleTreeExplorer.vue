<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Button from './Button.vue'
import type { BatchTreeNode, GlobalLogNode } from '@/lib/api'
import { bytesToHex, shortHash } from '@/lib/format'

type TreeNode = BatchTreeNode | GlobalLogNode

type HitNode = {
  key: string
  node: TreeNode
  x: number
  y: number
  width: number
  height: number
}

const props = defineProps<{
  nodes: TreeNode[]
  loading?: boolean
  nextCursor?: string
  emptyLabel?: string
  treeSize?: number
}>()

const emit = defineEmits<{
  loadMore: []
  expand: [node: TreeNode]
}>()

const canvas = ref<HTMLCanvasElement | null>(null)
const frame = ref<HTMLDivElement | null>(null)
const viewportWidth = ref(760)
const selectedKey = ref('')
const hoveredKey = ref('')
const hits = ref<HitNode[]>([])
let resizeObserver: ResizeObserver | undefined

const sortedNodes = computed(() => {
  return [...props.nodes].sort((a, b) => {
    if (a.level !== b.level) return b.level - a.level
    return a.start_index - b.start_index
  })
})

const selectedNode = computed(() => {
  const hit = hits.value.find((item) => item.key === selectedKey.value)
  return hit?.node ?? sortedNodes.value[0]
})

const canvasHeight = computed(() => {
  if (!sortedNodes.value.length) return 320
  const levelCount = new Set(sortedNodes.value.map((node) => node.level)).size
  return Math.max(360, Math.min(860, 130 + levelCount * 112))
})

const canvasWidth = computed(() => {
  const nodeWidth = nodeBoxWidth()
  const rowCounts = new Map<number, number>()
  for (const node of sortedNodes.value) {
    rowCounts.set(node.level, (rowCounts.get(node.level) ?? 0) + 1)
  }
  const maxRowCount = Math.max(1, ...rowCounts.values())
  return Math.max(viewportWidth.value, 96 + maxRowCount * nodeWidth + (maxRowCount - 1) * 56)
})

function keyOf(node: TreeNode): string {
  return `${node.level}:${node.start_index}:${node.width}`
}

function shortNodeHash(node: TreeNode): string {
  return shortHash(bytesToHex(node.hash), 8, 8) || '—'
}

function nodeRangeLabel(node: TreeNode): string {
  return `L${node.level} / start ${node.start_index} / width ${node.width}`
}

function childRanges(node: TreeNode): Array<{ level: number; start: number; width: number }> {
  if (node.width <= 1) return []
  const leftWidth = largestPowerOfTwoLessThan(node.width)
  const rightWidth = node.width - leftWidth
  return [
    { level: rangeLevel(leftWidth), start: node.start_index, width: leftWidth },
    { level: rangeLevel(rightWidth), start: node.start_index + leftWidth, width: rightWidth },
  ]
}

function childKey(range: { level: number; start: number; width: number }): string {
  return `${range.level}:${range.start}:${range.width}`
}

function rangeLevel(size: number): number {
  if (size <= 1) return 0
  let level = 0
  let width = 1
  while (width < size) {
    width <<= 1
    level += 1
  }
  return level
}

function largestPowerOfTwoLessThan(size: number): number {
  let out = 1
  while (out << 1 < size) out <<= 1
  return out
}

function roundedRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  const radius = Math.min(r, w / 2, h / 2)
  ctx.beginPath()
  ctx.moveTo(x + radius, y)
  ctx.arcTo(x + w, y, x + w, y + h, radius)
  ctx.arcTo(x + w, y + h, x, y + h, radius)
  ctx.arcTo(x, y + h, x, y, radius)
  ctx.arcTo(x, y, x + w, y, radius)
  ctx.closePath()
}

function nodeBoxWidth(): number {
  return viewportWidth.value < 680 ? 150 : 178
}

function setCanvasSize() {
  const nextWidth = frame.value?.clientWidth || 760
  viewportWidth.value = Math.max(360, nextWidth)
  draw()
}

function layoutNodes(): HitNode[] {
  const nodes = sortedNodes.value
  if (!nodes.length) return []

  const paddingX = 48
  const paddingY = 54
  const nodeWidth = nodeBoxWidth()
  const nodeHeight = 68
  const gap = 56
  const drawWidth = canvasWidth.value
  const drawableWidth = Math.max(1, drawWidth - paddingX * 2)
  const maxLevel = Math.max(...nodes.map((node) => node.level))
  const totalWidth = Math.max(
    props.treeSize || 0,
    ...nodes.map((node) => node.start_index + Math.max(1, node.width)),
  )
  const levels = [...new Set(nodes.map((node) => node.level))].sort((a, b) => b - a)
  const levelIndex = new Map(levels.map((level, index) => [level, index]))
  const byLevel = new Map<number, HitNode[]>()

  for (const node of nodes) {
    const center = node.start_index + Math.max(1, node.width) / 2
    const x = paddingX + (center / totalWidth) * drawableWidth - nodeWidth / 2
    const row = levelIndex.get(node.level) ?? Math.max(0, maxLevel - node.level)
    const hit = {
      key: keyOf(node),
      node,
      x,
      y: paddingY + row * 108,
      width: nodeWidth,
      height: nodeHeight,
    }
    const rowHits = byLevel.get(node.level) ?? []
    rowHits.push(hit)
    byLevel.set(node.level, rowHits)
  }

  const out: HitNode[] = []
  for (const rowHits of byLevel.values()) {
    rowHits.sort((a, b) => a.node.start_index - b.node.start_index)
    for (let i = 0; i < rowHits.length; i += 1) {
      const previous = rowHits[i - 1]
      rowHits[i].x = Math.max(12, rowHits[i].x)
      if (previous) rowHits[i].x = Math.max(rowHits[i].x, previous.x + previous.width + gap)
    }
    const overflow = rowHits.length ? rowHits[rowHits.length - 1].x + nodeWidth + 12 - drawWidth : 0
    if (overflow > 0) {
      for (const hit of rowHits) hit.x -= overflow
    }
    if (rowHits.length && rowHits[0].x < 12) {
      const shift = 12 - rowHits[0].x
      for (const hit of rowHits) hit.x += shift
    }
    out.push(...rowHits)
  }
  return out
}

function draw() {
  const el = canvas.value
  if (!el) return
  const ctx = el.getContext('2d')
  if (!ctx) return

  const dpr = window.devicePixelRatio || 1
  const height = canvasHeight.value
  const drawWidth = canvasWidth.value
  el.width = Math.floor(drawWidth * dpr)
  el.height = Math.floor(height * dpr)
  el.style.width = `${drawWidth}px`
  el.style.height = `${height}px`
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, drawWidth, height)

  const gradient = ctx.createLinearGradient(0, 0, drawWidth, height)
  gradient.addColorStop(0, '#111a12')
  gradient.addColorStop(0.55, '#070907')
  gradient.addColorStop(1, '#0b1110')
  ctx.fillStyle = gradient
  ctx.fillRect(0, 0, drawWidth, height)

  ctx.strokeStyle = 'rgba(188, 255, 86, 0.07)'
  ctx.lineWidth = 1
  for (let x = 24; x < drawWidth; x += 42) {
    ctx.beginPath()
    ctx.moveTo(x, 0)
    ctx.lineTo(x, height)
    ctx.stroke()
  }
  for (let y = 28; y < height; y += 42) {
    ctx.beginPath()
    ctx.moveTo(0, y)
    ctx.lineTo(drawWidth, y)
    ctx.stroke()
  }

  const nextHits = layoutNodes()
  hits.value = nextHits
  const byKey = new Map(nextHits.map((hit) => [hit.key, hit]))

  ctx.lineCap = 'round'
  for (const hit of nextHits) {
    for (const range of childRanges(hit.node)) {
      const child = byKey.get(childKey(range))
      if (!child) continue
      const startX = hit.x + hit.width / 2
      const startY = hit.y + hit.height
      const endX = child.x + child.width / 2
      const endY = child.y
      const edge = ctx.createLinearGradient(startX, startY, endX, endY)
      edge.addColorStop(0, 'rgba(188, 255, 86, 0.55)')
      edge.addColorStop(1, 'rgba(77, 163, 255, 0.28)')
      ctx.strokeStyle = edge
      ctx.lineWidth = 2
      ctx.beginPath()
      ctx.moveTo(startX, startY)
      const midY = (startY + endY) / 2
      ctx.bezierCurveTo(startX, midY, endX, midY, endX, endY)
      ctx.stroke()
    }
  }

  for (const hit of nextHits) {
    const isSelected = hit.key === selectedKey.value
    const isHovered = hit.key === hoveredKey.value
    const expandable = hit.node.width > 1
    const fill = isSelected ? 'rgba(188, 255, 86, 0.16)' : expandable ? 'rgba(11, 18, 12, 0.92)' : 'rgba(8, 10, 9, 0.88)'
    const border = isSelected ? 'rgba(188, 255, 86, 0.95)' : isHovered ? 'rgba(188, 255, 86, 0.65)' : 'rgba(255, 255, 255, 0.16)'

    ctx.shadowColor = isSelected ? 'rgba(188, 255, 86, 0.28)' : 'rgba(0, 0, 0, 0.22)'
    ctx.shadowBlur = isSelected ? 18 : 10
    roundedRect(ctx, hit.x, hit.y, hit.width, hit.height, 16)
    ctx.fillStyle = fill
    ctx.fill()
    ctx.shadowBlur = 0
    ctx.strokeStyle = border
    ctx.lineWidth = isSelected ? 2 : 1
    ctx.stroke()

    ctx.fillStyle = expandable ? '#bcff56' : '#7dd3fc'
    ctx.beginPath()
    ctx.arc(hit.x + 18, hit.y + 18, 5, 0, Math.PI * 2)
    ctx.fill()

    ctx.font = '700 11px ui-monospace, SFMono-Regular, Menlo, monospace'
    ctx.fillStyle = '#f6fff0'
    ctx.fillText(`L${hit.node.level} / start ${hit.node.start_index}`, hit.x + 32, hit.y + 22)

    ctx.font = '10px ui-monospace, SFMono-Regular, Menlo, monospace'
    ctx.fillStyle = '#92a08e'
    ctx.fillText(`width ${hit.node.width}${expandable ? ' · click to expand' : ' · leaf'}`, hit.x + 14, hit.y + 42)

    ctx.font = '11px ui-monospace, SFMono-Regular, Menlo, monospace'
    ctx.fillStyle = '#bcff56'
    ctx.fillText(shortNodeHash(hit.node), hit.x + 14, hit.y + 60)
  }
}

function eventPoint(event: MouseEvent): { x: number; y: number } {
  const rect = canvas.value?.getBoundingClientRect()
  if (!rect) return { x: 0, y: 0 }
  return {
    x: event.clientX - rect.left,
    y: event.clientY - rect.top,
  }
}

function hitAt(event: MouseEvent): HitNode | undefined {
  const point = eventPoint(event)
  for (let i = hits.value.length - 1; i >= 0; i -= 1) {
    const hit = hits.value[i]
    if (point.x >= hit.x && point.x <= hit.x + hit.width && point.y >= hit.y && point.y <= hit.y + hit.height) return hit
  }
  return undefined
}

function handleMove(event: MouseEvent) {
  const hit = hitAt(event)
  hoveredKey.value = hit?.key ?? ''
  if (canvas.value) canvas.value.style.cursor = hit ? 'pointer' : 'default'
}

function handleLeave() {
  hoveredKey.value = ''
  if (canvas.value) canvas.value.style.cursor = 'default'
}

function handleClick(event: MouseEvent) {
  const hit = hitAt(event)
  if (!hit) return
  selectedKey.value = hit.key
  if (hit.node.width > 1) emit('expand', hit.node)
}

watch([sortedNodes, viewportWidth, canvasHeight, canvasWidth, selectedKey, hoveredKey], () => nextTick(draw), { deep: true })

onMounted(() => {
  if (typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver(setCanvasSize)
    if (frame.value) resizeObserver.observe(frame.value)
  }
  setCanvasSize()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
})
</script>

<template>
  <div>
    <div v-if="nodes.length" ref="frame" class="overflow-x-auto overflow-y-hidden rounded-[18px] border border-white/10 bg-[#050605]">
      <canvas
        ref="canvas"
        class="block"
        role="img"
        aria-label="Merkle tree canvas explorer"
        @mousemove="handleMove"
        @mouseleave="handleLeave"
        @click="handleClick"
      />
      <div class="flex flex-wrap items-center justify-between gap-3 border-t border-white/10 px-4 py-3 text-[11px] text-ink-400">
        <div>
          <span class="text-ink-200">当前节点：</span>
          <span v-if="selectedNode" class="font-mono text-accent">{{ nodeRangeLabel(selectedNode) }}</span>
          <span v-else>暂无</span>
        </div>
        <div class="flex gap-3">
          <span>可见 {{ nodes.length }} 个节点</span>
          <span>点击非叶子节点按需展开</span>
        </div>
      </div>
    </div>
    <p v-else class="text-[12px] text-ink-500">{{ emptyLabel || '暂无节点，调整 level/start 后重试。' }}</p>
    <div v-if="nextCursor" class="mt-4">
      <Button size="sm" variant="subtle" :loading="loading" @click="emit('loadMore')">加载更多节点</Button>
    </div>
  </div>
</template>
