<script setup lang="ts">
withDefaults(defineProps<{
  size?: number
  animated?: boolean
}>(), {
  size: 52,
  animated: true,
})
</script>

<template>
  <svg
    class="trustdb-logo-mark"
    :class="{ 'is-animated': animated }"
    :width="size"
    :height="size"
    viewBox="0 0 512 512"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
    role="img"
    aria-label="TrustDB"
  >
    <defs>
      <clipPath id="trustdb-logo-clip">
        <rect x="32" y="32" width="448" height="448" rx="112" />
      </clipPath>
    </defs>

    <rect class="mark-bg" x="32" y="32" width="448" height="448" rx="112" />
    <rect class="mark-border" x="39" y="39" width="434" height="434" rx="105" />

    <g class="motion-layer" clip-path="url(#trustdb-logo-clip)">
      <path class="scan-beam" d="M-68 80H576L520 150H-124Z" />
      <path class="scan-edge" d="M80 424L424 80" />
    </g>

    <g class="mark-detail-lines">
      <path d="M102 154H184" style="--line-length: 82" />
      <path d="M102 188H152" style="--line-length: 50" />
      <path d="M328 350H410" style="--line-length: 82" />
      <path d="M360 384H410" style="--line-length: 50" />
    </g>

    <g class="proof-dots">
      <circle cx="102" cy="154" r="7" />
      <circle cx="410" cy="350" r="7" />
      <circle cx="410" cy="384" r="7" />
    </g>

    <g class="orbit-dot">
      <circle cx="428" cy="104" r="6" />
    </g>

    <g class="check-piece check-short" transform="rotate(45 185 252)">
      <rect class="check-base" x="140" y="230" width="90" height="44" rx="12" />
      <rect class="check-glint" x="148" y="234" width="18" height="36" rx="9" />
    </g>
    <g class="check-piece check-long" transform="rotate(-45 291 256)">
      <rect class="check-base" x="196" y="234" width="190" height="44" rx="12" />
      <rect class="check-glint" x="210" y="238" width="24" height="36" rx="12" />
    </g>
  </svg>
</template>

<style scoped>
.trustdb-logo-mark {
  display: block;
  overflow: visible;
  transform-box: fill-box;
  transform-origin: center;
}

.mark-bg {
  fill: #050805;
}

.mark-border {
  fill: none;
  stroke: #00ff22;
  stroke-opacity: 0.2;
  stroke-width: 6;
}

.mark-detail-lines {
  stroke: #00ff22;
  stroke-linecap: round;
  stroke-opacity: 0.13;
  stroke-width: 8;
}

.mark-detail-lines path {
  stroke-dasharray: calc(var(--line-length) * 1px);
  stroke-dashoffset: 0;
}

.proof-dots {
  fill: #00ff22;
  opacity: 0.34;
}

.motion-layer {
  pointer-events: none;
}

.scan-beam {
  fill: #00ff22;
  opacity: 0;
}

.scan-edge {
  fill: none;
  stroke: #c9ffd1;
  stroke-linecap: round;
  stroke-opacity: 0;
  stroke-width: 5;
}

.orbit-dot {
  fill: #00ff22;
  opacity: 0;
  transform-box: view-box;
  transform-origin: 256px 256px;
}

.check-piece .check-base {
  fill: #00ff22;
  transform-box: fill-box;
  transform-origin: center;
}

.check-piece .check-glint {
  fill: #d9ffe0;
  opacity: 0;
  transform-box: fill-box;
  transform-origin: center;
}

.is-animated {
  animation: trustdb-logo-breathe 4600ms ease-in-out infinite;
}

.is-animated .mark-border {
  stroke-dasharray: 1370;
  stroke-dashoffset: 1370;
  animation:
    trustdb-border-draw 900ms cubic-bezier(.2, .9, .2, 1) 40ms both,
    trustdb-border-breathe 3200ms ease-in-out 1100ms infinite;
}

.is-animated .check-short rect {
  animation: trustdb-check-in 680ms cubic-bezier(.2, .9, .2, 1) 90ms both;
}

.is-animated .check-long rect {
  animation: trustdb-check-in 760ms cubic-bezier(.2, .9, .2, 1) 210ms both;
}

.is-animated .check-short .check-base {
  animation:
    trustdb-check-in 680ms cubic-bezier(.2, .9, .2, 1) 90ms both,
    trustdb-check-pulse 3400ms ease-in-out 1450ms infinite;
}

.is-animated .check-long .check-base {
  animation:
    trustdb-check-in 760ms cubic-bezier(.2, .9, .2, 1) 210ms both,
    trustdb-check-pulse 3400ms ease-in-out 1530ms infinite;
}

.is-animated .check-short .check-glint {
  animation: trustdb-glint-short 3200ms cubic-bezier(.25, .9, .3, 1) 1200ms infinite;
}

.is-animated .check-long .check-glint {
  animation: trustdb-glint-long 3200ms cubic-bezier(.25, .9, .3, 1) 1320ms infinite;
}

.is-animated .proof-dots circle {
  animation: trustdb-dot-pulse 2400ms ease-in-out infinite;
}

.is-animated .proof-dots circle:nth-child(2) {
  animation-delay: 380ms;
}

.is-animated .proof-dots circle:nth-child(3) {
  animation-delay: 760ms;
}

.is-animated .mark-detail-lines path {
  animation: trustdb-line-stream 2800ms ease-in-out infinite;
}

.is-animated .mark-detail-lines path:nth-child(2) {
  animation-delay: 180ms;
}

.is-animated .mark-detail-lines path:nth-child(3) {
  animation-delay: 360ms;
}

.is-animated .mark-detail-lines path:nth-child(4) {
  animation-delay: 540ms;
}

.is-animated .scan-beam {
  animation: trustdb-scan-beam 3600ms ease-in-out infinite 900ms;
}

.is-animated .scan-edge {
  animation: trustdb-scan-edge 3600ms ease-in-out infinite 900ms;
}

.is-animated .orbit-dot {
  animation: trustdb-orbit 5200ms linear infinite 1000ms;
}

@keyframes trustdb-logo-breathe {
  0%, 100% {
    filter: drop-shadow(0 0 0 rgba(0, 255, 34, 0));
    transform: scale(1);
  }
  50% {
    filter: drop-shadow(0 0 14px rgba(0, 255, 34, .34));
    transform: scale(1.018);
  }
}

@keyframes trustdb-border-draw {
  to {
    stroke-dashoffset: 0;
  }
}

@keyframes trustdb-border-breathe {
  0%, 100% {
    stroke-opacity: .18;
  }
  50% {
    stroke-opacity: .48;
  }
}

@keyframes trustdb-check-in {
  from {
    opacity: 0;
    transform: translateY(10px) scaleX(.62);
  }
  to {
    opacity: 1;
    transform: translateY(0) scaleX(1);
  }
}

@keyframes trustdb-check-pulse {
  0%, 100% {
    filter: drop-shadow(0 0 0 rgba(0, 255, 34, 0));
    transform: scale(1);
  }
  46% {
    filter: drop-shadow(0 0 10px rgba(0, 255, 34, .42));
    transform: scale(1.035);
  }
}

@keyframes trustdb-glint-short {
  0%, 22%, 100% {
    opacity: 0;
    transform: translateX(-34px) scaleX(.35);
  }
  34% {
    opacity: .38;
  }
  48% {
    opacity: 0;
    transform: translateX(78px) scaleX(.75);
  }
}

@keyframes trustdb-glint-long {
  0%, 20%, 100% {
    opacity: 0;
    transform: translateX(-60px) scaleX(.35);
  }
  34% {
    opacity: .34;
  }
  50% {
    opacity: 0;
    transform: translateX(168px) scaleX(.82);
  }
}

@keyframes trustdb-dot-pulse {
  0%, 100% {
    opacity: .22;
    transform: scale(.9);
  }
  45% {
    opacity: .55;
    transform: scale(1.18);
  }
}

@keyframes trustdb-line-stream {
  0%, 100% {
    stroke-opacity: .1;
    stroke-dashoffset: 0;
  }
  42% {
    stroke-opacity: .34;
    stroke-dashoffset: calc(var(--line-length) * -1px);
  }
}

@keyframes trustdb-scan-beam {
  0% {
    opacity: 0;
    transform: translateY(-170px);
  }
  18% {
    opacity: .08;
  }
  42% {
    opacity: .18;
  }
  66% {
    opacity: 0;
    transform: translateY(420px);
  }
  100% {
    opacity: 0;
    transform: translateY(420px);
  }
}

@keyframes trustdb-scan-edge {
  0%, 15% {
    opacity: 0;
    transform: translate(-170px, 170px);
  }
  34% {
    opacity: .26;
  }
  60%, 100% {
    opacity: 0;
    transform: translate(170px, -170px);
  }
}

@keyframes trustdb-orbit {
  0% {
    opacity: 0;
    transform: rotate(0deg);
  }
  12%, 72% {
    opacity: .5;
  }
  100% {
    opacity: 0;
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .is-animated,
  .is-animated .mark-border,
  .is-animated .check-short rect,
  .is-animated .check-long rect,
  .is-animated .proof-dots circle,
  .is-animated .mark-detail-lines path,
  .is-animated .scan-beam,
  .is-animated .scan-edge,
  .is-animated .orbit-dot {
    animation: none;
  }
}
</style>
