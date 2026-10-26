<script setup lang="ts">
/** Pages under a list (list.ts): "21–40 of 57" and Previous / Next; hidden while everything fits on one page. */
defineProps<{ pages: number; range: string }>();
const page = defineModel<number>({ required: true });
</script>

<template>
  <div v-if="pages > 1" class="flex items-center justify-end gap-2 text-sm">
    <span class="opacity-60">{{ range }}</span>
    <button type="button" class="btn btn-ghost btn-sm" :disabled="page === 0" @click="page--">Previous</button>
    <span class="text-xs opacity-50">Page {{ page + 1 }} of {{ pages }}</span>
    <button type="button" class="btn btn-ghost btn-sm" :disabled="page >= pages - 1" @click="page++">Next</button>
  </div>
</template>
