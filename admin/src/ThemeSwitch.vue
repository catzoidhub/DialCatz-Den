<script setup lang="ts">
import { setTheme, theme, type Theme } from './theme';

/** Auto / Light / Dark as three icons (monitor, sun, moon). `small` for corners and the sidebar. */
defineProps<{ small?: boolean; vertical?: boolean }>();
const OPTIONS: { id: Theme; label: string; paths: string[] }[] = [
  { id: 'system', label: 'Auto', paths: ['M4 4h16a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1Z', 'M8 20h8', 'M12 16v4'] },
  { id: 'light', label: 'Light', paths: ['M12 16a4 4 0 1 0 0-8 4 4 0 0 0 0 8Z', 'M12 2v2', 'M12 20v2', 'm4.93 4.93 1.41 1.41', 'm17.66 17.66 1.41 1.41', 'M2 12h2', 'M20 12h2', 'm6.34 17.66-1.41 1.41', 'm19.07 4.93-1.41 1.41'] },
  { id: 'dark', label: 'Dark', paths: ['M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z'] },
];
</script>

<template>
  <div class="seg seg-choice" :class="{ 'seg-xs': small, 'flex-col': vertical }" role="radiogroup" aria-label="Theme">
    <button v-for="o in OPTIONS" :key="o.id" type="button" role="radio" :aria-checked="theme === o.id" :aria-label="o.label" :title="o.label" class="seg-item" :class="{ 'is-active': theme === o.id }" @click="setTheme(o.id)">
      <svg viewBox="0 0 24 24" :class="small ? 'size-3.5' : 'size-4'" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path v-for="d in o.paths" :key="d" :d="d" /></svg>
    </button>
  </div>
</template>
