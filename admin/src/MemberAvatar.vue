<script setup lang="ts">
import { ref, watch } from 'vue';
import { initials } from './api';

/** A member's picture (their own URL, or Gravatar's), with their initials when it does not load (Gravatar answers 404 for none). */
const props = defineProps<{ name: string; url: string }>();
const broken = ref(false);
watch(() => props.url, () => (broken.value = false));
</script>

<template>
  <img v-if="url && !broken" :src="url" alt="" class="size-9 shrink-0 rounded-full object-cover" referrerpolicy="no-referrer" @error="broken = true" />
  <span v-else class="avatar-circle">{{ initials(name) }}</span>
</template>
