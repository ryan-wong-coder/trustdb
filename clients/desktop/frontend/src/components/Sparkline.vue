<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  values: number[]
  width?: number
  height?: number
  stroke?: string
  fill?: string
}>(), {
  width: 120,
  height: 32,
  stroke: 'currentColor',
  fill: 'currentColor',
})

// A minimal dependency-free sparkline. We pre-compute the SVG path as
// a plain polyline and a matching area polygon so it renders crisply
// at any DPI without loading a chart library for four data points.
const geometry = computed(() => {
  const xs = props.values
  if (!xs.length) return { line: '', area: '', min: 0, max: 0 }
  const min = Math.min(...xs)
  const max = Math.max(...xs)
  const span = max - min || 1
  const w = props.width
  const h = props.height
  const step = xs.length > 1 ? w / (xs.length - 1) : 0
  const points = xs.map((v, i) => {
    const x = i * step
    const y = h - ((v - min) / span) * h
    return [x, y] as [number, number]
  })
  const line = points.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(' ')
  const area = `0,${h} ${line} ${w},${h}`
  return { line, area, min, max }
})
</script>

<template>
  <svg
    :width="width"
    :height="height"
    :viewBox="`0 0 ${width} ${height}`"
    class="block"
    preserveAspectRatio="none"
  >
    <polygon :points="geometry.area" :fill="fill" fill-opacity="0.10" />
    <polyline
      :points="geometry.line"
      fill="none"
      :stroke="stroke"
      stroke-width="1.4"
      stroke-linecap="round"
      stroke-linejoin="round"
    />
  </svg>
</template>
