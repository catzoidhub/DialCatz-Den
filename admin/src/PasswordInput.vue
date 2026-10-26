<script setup lang="ts">
import { ref } from 'vue';

/** A password field with a show / hide eye (signed-out pages). */
defineProps<{ autocomplete: string; minlength?: number; placeholder?: string }>();
const model = defineModel<string>({ required: true });
const shown = ref(false);
</script>

<template>
  <div class="relative">
    <input v-model="model" :type="shown ? 'text' : 'password'" class="input auth-input w-full pr-11" required :autocomplete="autocomplete" :minlength="minlength" :placeholder="placeholder" />
    <button type="button" class="absolute top-1/2 right-2 grid size-8 -translate-y-1/2 place-items-center rounded-lg opacity-55 hover:bg-base-content/10 hover:opacity-90" :title="shown ? 'Hide password' : 'Show password'" :aria-label="shown ? 'Hide password' : 'Show password'" @click="shown = !shown">
      <svg v-if="!shown" viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z" /><circle cx="12" cy="12" r="3" /></svg>
      <svg v-else viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9.9 5.1A9.8 9.8 0 0 1 12 5c6.5 0 10 7 10 7a17.6 17.6 0 0 1-2.2 3.1M6.6 6.6C3.9 8.4 2 12 2 12s3.5 7 10 7a9.7 9.7 0 0 0 5.4-1.6M3 3l18 18M9.9 9.9a3 3 0 0 0 4.2 4.2" /></svg>
    </button>
  </div>
</template>
