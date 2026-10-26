<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { initials, session } from './api';
import icon from './assets/icon.png';
import ThemeSwitch from './ThemeSwitch.vue';

/**
 * The panel's sidebar: the logo and server, the pages with icons, the theme and the signed-in admin. It folds to an
 * icon rail (names as tooltips) and remembers that per browser. On narrow screens it starts folded and opens over the
 * page with a dimmed backdrop, so the page never gets squeezed.
 */
defineProps<{ page: string; nav: { id: string; label: string }[] }>();
const emit = defineEmits<{ go: [id: string]; signOut: [] }>();

const ICONS: Record<string, string[]> = {
  members: ['M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2', 'M5 7a4 4 0 1 0 8 0a4 4 0 1 0-8 0', 'M22 21v-2a4 4 0 0 0-3-3.87', 'M16 3.13a4 4 0 0 1 0 7.75'],
  workspaces: ['M12 2 2 7l10 5 10-5-10-5Z', 'm2 17 10 5 10-5', 'm2 12 10 5 10-5'],
  devices: ['M4 5h16a1 1 0 0 1 1 1v10H3V6a1 1 0 0 1 1-1Z', 'M1.5 19.5h21'],
  admins: ['M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z', 'm9 12 2 2 4-4'],
  activity: ['M22 12h-4l-3 9L9 3l-3 9H2'],
  backup: ['M12 3c4.4 0 8 1.3 8 3s-3.6 3-8 3-8-1.3-8-3 3.6-3 8-3Z', 'M4 6v6c0 1.7 3.6 3 8 3s8-1.3 8-3V6', 'M4 12v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6'],
  account: ['M2 12a10 10 0 1 0 20 0a10 10 0 1 0-20 0', 'M9 10a3 3 0 1 0 6 0a3 3 0 1 0-6 0', 'M6.2 18.8a7 7 0 0 1 11.6 0'],
};
const SIGN_OUT = ['M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4', 'm16 17 5-5-5-5', 'M21 12H9'];

const KEY = 'den.sidebar';
const narrow = ref(false);
const open = ref(true);
const media = window.matchMedia('(max-width: 767px)');
function sync() {
  narrow.value = media.matches;
  let saved: string | null = null;
  try {
    saved = localStorage.getItem(KEY);
  } catch {
    /* storage blocked: the default below */
  }
  open.value = narrow.value ? false : saved !== 'closed';
}
function toggle() {
  open.value = !open.value;
  if (narrow.value) return;
  try {
    localStorage.setItem(KEY, open.value ? 'open' : 'closed');
  } catch {
    /* not remembered: fine */
  }
}
function go(id: string) {
  emit('go', id);
  if (narrow.value) open.value = false;
}
onMounted(() => {
  sync();
  media.addEventListener('change', sync);
});
onBeforeUnmount(() => media.removeEventListener('change', sync));
const host = () => session.publicUrl.replace(/^https?:\/\//, '');
</script>

<template>
  <!-- On narrow screens the folded rail keeps its place; the open sidebar floats over the page. -->
  <div class="side-slot" :class="{ 'is-open': open && !narrow }">
    <div v-if="open && narrow" class="side-backdrop" @click="open = false" />
    <aside class="side" :class="{ 'is-open': open, 'is-floating': open && narrow }">
      <div class="side-top">
        <img :src="icon" alt="" class="side-logo" />
        <div v-if="open" class="min-w-0 flex-1">
          <p class="truncate text-[15px] font-semibold tracking-tight">DialCatz Den</p>
          <p class="truncate text-[11px] opacity-55">{{ host() }}</p>
        </div>
      </div>
      <!-- Fold / unfold: a round handle on the sidebar's edge, the usual place for it. -->
      <button class="side-handle tooltip tooltip-right" :data-tip="open ? 'Collapse sidebar' : 'Expand sidebar'" :aria-label="open ? 'Collapse sidebar' : 'Expand sidebar'" :aria-expanded="open" @click="toggle">
        <svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path :d="open ? 'm15 18-6-6 6-6' : 'm9 18 6-6-6-6'" /></svg>
      </button>

      <nav class="side-nav" aria-label="Pages">
        <button v-for="n in nav" :key="n.id" class="side-item" :class="{ 'is-active': page === n.id, 'tooltip tooltip-right': !open }" :data-tip="open ? undefined : n.label" :aria-current="page === n.id ? 'page' : undefined" @click="go(n.id)">
          <svg viewBox="0 0 24 24" class="size-[18px] shrink-0" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path v-for="d in ICONS[n.id] ?? ICONS.account" :key="d" :d="d" /></svg>
          <span v-if="open" class="truncate">{{ n.label }}</span>
        </button>
      </nav>

      <div class="side-bottom">
        <!-- AGPL-3.0 §13: everyone using this Den over the network can get its source -->
        <a v-if="open" class="side-source" href="https://github.com/catzoidhub/DialCatz-Den" target="_blank" rel="noopener">Source code · AGPL-3.0</a>
        <ThemeSwitch small :vertical="!open" :class="open ? 'self-start' : 'self-center'" />
        <div class="side-account" :class="{ 'is-folded': !open }">
          <span class="side-avatar" :class="{ 'tooltip tooltip-right': !open }" :data-tip="open ? undefined : session.admin?.email">{{ initials(session.admin?.name || session.admin?.email || '?') }}</span>
          <span v-if="open" class="min-w-0 flex-1">
            <span class="block truncate text-[13px] font-medium">{{ session.admin?.name }}</span>
            <span class="block truncate text-[11px] opacity-55">{{ session.admin?.email }}</span>
          </span>
          <button class="side-signout" :class="open ? '' : 'tooltip tooltip-right'" :title="open ? 'Sign out' : undefined" data-tip="Sign out" aria-label="Sign out" @click="emit('signOut')">
            <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path v-for="d in SIGN_OUT" :key="d" :d="d" /></svg>
          </button>
        </div>
      </div>
    </aside>
  </div>
</template>
