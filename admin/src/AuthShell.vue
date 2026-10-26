<script setup lang="ts">
import icon from './assets/icon.png';
import ThemeSwitch from './ThemeSwitch.vue';

/**
 * The frame of every signed-out page (sign in, first start, two-step code, admin invite): on wide screens a brand
 * panel on the left (the logo, the line, what the Den does) and the form on the right; on narrow ones the form alone
 * with the logo on top. Colours come from the theme tokens, so light and dark follow the panel.
 */
defineProps<{ title: string; subtitle?: string }>();
const POINTS = [
  { title: 'No accounts for members', text: 'Invite each device with a one-time link; revoke one and only it is locked out.' },
  { title: 'Live for the whole team', text: 'Collections and environments sync as people work, with who is looking at what.' },
  { title: 'Secrets stay home', text: 'Variable values never reach this server — only their names.' },
];
</script>

<template>
  <div class="auth-page">
    <ThemeSwitch small class="absolute top-4 right-4 z-10" />
    <section class="auth-brand">
      <div class="auth-glow" aria-hidden="true" />
      <div class="relative flex h-full flex-col">
        <div class="flex items-center gap-5">
          <img :src="icon" alt="" class="auth-logo size-20 rounded-[1.4rem]" />
          <span>
            <span class="block text-3xl font-semibold tracking-tight">DialCatz Den</span>
            <span class="block text-sm opacity-70">Team server for DialCatz</span>
          </span>
        </div>
        <div class="my-auto max-w-md space-y-8 py-12">
          <h2 class="text-4xl leading-tight font-semibold tracking-tight">Where the team’s cats keep their calls.</h2>
          <ul class="space-y-5">
            <li v-for="p in POINTS" :key="p.title" class="flex gap-3">
              <span class="auth-tick" aria-hidden="true"><svg viewBox="0 0 20 20" class="size-3.5" fill="currentColor"><path fill-rule="evenodd" d="M16.7 5.3a1 1 0 0 1 0 1.4l-7.5 7.5a1 1 0 0 1-1.4 0L3.3 9.7a1 1 0 1 1 1.4-1.4l3.8 3.8 6.8-6.8a1 1 0 0 1 1.4 0Z" clip-rule="evenodd" /></svg></span>
              <span><span class="block font-medium">{{ p.title }}</span><span class="block text-sm opacity-70">{{ p.text }}</span></span>
            </li>
          </ul>
        </div>
        <p class="text-xs opacity-55">The self-hosted team server for DialCatz · part of the Catzoid family</p>
      </div>
    </section>
    <section class="auth-form-side">
      <div class="w-full max-w-sm space-y-7">
        <div class="space-y-3">
          <div class="flex items-center gap-3.5 pb-2 lg:hidden">
            <img :src="icon" alt="" class="auth-logo size-14 rounded-2xl" />
            <span><span class="block text-xl font-semibold tracking-tight">DialCatz Den</span><span class="block text-xs opacity-60">Team server for DialCatz</span></span>
          </div>
          <div>
            <h1 class="text-2xl font-semibold tracking-tight">{{ title }}</h1>
            <p v-if="subtitle" class="mt-1 text-sm opacity-60">{{ subtitle }}</p>
          </div>
        </div>
        <slot />
      </div>
    </section>
  </div>
</template>
