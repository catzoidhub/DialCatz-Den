<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { attempt, call } from './api';

/**
 * Activity → Dashboard: what happened on this server over the last 7, 30 or 90 days, drawn by hand (SVG and CSS, no chart
 * library): events per day by kind, the kinds' share, when in the week things happen, who and what is most frequent,
 * and a few plain-language findings. Counted on the server (GET /audit/stats) in the admin's own time zone.
 */
interface Day { day: string; total: number; kinds: Record<string, number> }
interface Count { name: string; count: number }
interface Stats {
  days: Day[];
  kinds: Record<string, number>;
  actors: Count[];
  actions: Count[];
  members: Count[];
  heat: number[][];
  total: number;
  previous: number;
  people: number;
  connections: number;
  warnings: number;
}

const RANGES = [7, 30, 90] as const;
const KINDS = [
  { id: 'members', label: 'Members' },
  { id: 'workspaces', label: 'Workspaces' },
  { id: 'devices', label: 'Devices & invites' },
  { id: 'admins', label: 'Admins' },
  { id: 'server', label: 'Server' },
  { id: 'other', label: 'Other' },
] as const;
const WEEK = [1, 2, 3, 4, 5, 6, 0]; // Monday first
const range = ref<(typeof RANGES)[number]>(30);
const stats = ref<Stats | null>(null);
const loading = ref(false);

async function load() {
  loading.value = true;
  const r = await attempt(() => call<Stats>('GET', `/audit/stats?days=${range.value}&tz=${-new Date().getTimezoneOffset()}`));
  loading.value = false;
  if (r) stats.value = r;
}
onMounted(load);
watch(range, load);

// ---- helpers
const kindColor = (k: string) => `var(--k-${k})`;
const kindLabel = (k: string) => KINDS.find((x) => x.id === k)?.label ?? k;
const dayOf = (s: string) => {
  const [y, m, d] = s.split('-').map(Number);
  return new Date(y, m - 1, d);
};
const dayShort = (s: string) => dayOf(s).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
const dayLong = (s: string) => dayOf(s).toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' });
const weekday = (d: number) => new Date(2024, 0, 7 + d).toLocaleDateString(undefined, { weekday: 'short' }); // 2024-01-07 is a Sunday
const weekdayLong = (d: number) => new Date(2024, 0, 7 + d).toLocaleDateString(undefined, { weekday: 'long' });
const hourRange = (h: number) => `${String(h).padStart(2, '0')}:00–${String((h + 1) % 24).padStart(2, '0')}:00`;
/** "admin:ana@team.dev" → who and what they are. */
function who(actor: string) {
  const i = actor.indexOf(':');
  const role = i > 0 ? actor.slice(0, i) : '';
  return { name: i > 0 ? actor.slice(i + 1) : actor, role: role === 'admin' || role === 'member' ? role : role ? 'sign-in' : '' };
}
/** A round top for the axis: 1, 2 or 5 × 10ⁿ. */
function nice(n: number) {
  if (n <= 4) return Math.max(1, n);
  const p = 10 ** Math.floor(Math.log10(n));
  return ([1, 2, 5, 10].find((m) => m * p >= n) ?? 10) * p;
}
const pct = (n: number, of: number) => (!of ? '0' : n && n / of < 0.005 ? '<1' : String(Math.round((n / of) * 100)));

// ---- per day
const top = computed(() => nice(Math.max(0, ...(stats.value?.days.map((d) => d.total) ?? [0]))));
const shownKinds = computed(() => KINDS.filter((k) => (stats.value?.kinds[k.id] ?? 0) > 0));
const hover = ref<number | null>(null);
const busiest = computed(() => {
  const ds = stats.value?.days ?? [];
  return ds.reduce<number | null>((b, d, i) => (d.total > 0 && (b === null || d.total > ds[b].total) ? i : b), null);
});
const focus = computed(() => {
  const i = hover.value ?? busiest.value;
  return i === null ? null : stats.value!.days[i];
});
const tickEvery = computed(() => Math.ceil((stats.value?.days.length ?? 1) / 6));

// ---- kinds donut
const R = 42;
const C = 2 * Math.PI * R;
const slices = computed(() => {
  const s = stats.value;
  if (!s?.total) return [];
  let at = 0;
  return shownKinds.value.map((k) => {
    const n = s.kinds[k.id] ?? 0;
    const len = (n / s.total) * C;
    const slice = { ...k, n, dash: `${Math.max(0, len - 1.2)} ${C}`, offset: -at };
    at += len;
    return slice;
  });
});

// ---- week × hour
const heatMax = computed(() => Math.max(1, ...(stats.value?.heat.flat() ?? [1])));
const heatCell = (n: number) => (n ? `color-mix(in oklch, var(--color-primary) ${Math.round(18 + (82 * n) / heatMax.value)}%, transparent)` : undefined);
const peak = computed(() => {
  const h = stats.value?.heat;
  if (!h) return null;
  let best: { day: number; hour: number; n: number } | null = null;
  for (let d = 0; d < 7; d++) for (let hr = 0; hr < 24; hr++) if (h[d][hr] && (!best || h[d][hr] > best.n)) best = { day: d, hour: hr, n: h[d][hr] };
  return best;
});

// ---- findings
const trend = computed(() => {
  const s = stats.value;
  if (!s) return null;
  if (!s.previous) return s.total ? { text: 'new', up: true } : null;
  const ch = Math.round(((s.total - s.previous) / s.previous) * 100);
  return { text: `${ch > 0 ? '+' : ''}${ch}%`, up: ch >= 0 };
});
const findings = computed(() => {
  const s = stats.value;
  if (!s?.total) return [];
  const out: string[] = [];
  const perDay = s.total / s.days.length;
  out.push(`${s.total.toLocaleString()} events in the last ${s.days.length} days, about ${perDay < 10 ? perDay.toFixed(1) : Math.round(perDay)} a day${s.previous ? ` — ${s.total >= s.previous ? 'up' : 'down'} from ${s.previous.toLocaleString()} in the ${s.days.length} days before` : ''}.`);
  if (busiest.value !== null) out.push(`The busiest day was ${dayLong(s.days[busiest.value].day)}, with ${s.days[busiest.value].total} events.`);
  const quiet = s.days.filter((d) => !d.total).length;
  if (quiet) out.push(`${quiet} of the ${s.days.length} days had no activity at all.`);
  if (peak.value) out.push(`Most activity falls on ${weekdayLong(peak.value.day)}s around ${hourRange(peak.value.hour)}.`);
  const k = [...shownKinds.value].sort((a, b) => (s.kinds[b.id] ?? 0) - (s.kinds[a.id] ?? 0))[0];
  if (k) out.push(`${kindLabel(k.id)} make up ${pct(s.kinds[k.id], s.total)}% of everything.`);
  if (s.actors[0]) out.push(`${who(s.actors[0].name).name} is the most active, with ${s.actors[0].count} events (${pct(s.actors[0].count, s.total)}%).`);
  if (s.connections) out.push(`Devices connected ${s.connections} times${s.members.length ? `, most often for ${s.members[0].name}` : ''}.`);
  return out;
});
</script>

<template>
  <div class="space-y-5">
    <div class="flex flex-wrap items-center gap-3">
      <div class="seg seg-choice" role="tablist" aria-label="Period">
        <button v-for="r in RANGES" :key="r" role="tab" class="seg-item normal-case whitespace-nowrap" :class="{ 'is-active': range === r }" :aria-selected="range === r" @click="range = r">{{ r }} days</button>
      </div>
      <span v-if="loading" class="loading loading-spinner loading-sm opacity-50" />
    </div>

    <template v-if="stats">
      <!-- tiles -->
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <div class="stat-tile">
          <span class="label-caps">Events</span>
          <span class="stat-num">{{ stats.total.toLocaleString() }}</span>
          <span v-if="trend" class="text-xs" :class="trend.up ? 'text-success' : 'opacity-60'">{{ trend.text }} <span class="opacity-70">vs. the {{ range }} days before</span></span>
        </div>
        <div class="stat-tile">
          <span class="label-caps">People active</span>
          <span class="stat-num">{{ stats.people }}</span>
          <span class="text-xs opacity-60">admins and members</span>
        </div>
        <div class="stat-tile">
          <span class="label-caps">Device connections</span>
          <span class="stat-num">{{ stats.connections.toLocaleString() }}</span>
          <span class="text-xs opacity-60">from DialCatz apps</span>
        </div>
        <div class="stat-tile">
          <span class="label-caps">Sign-in warnings</span>
          <span class="stat-num" :class="{ 'text-warning': stats.warnings }">{{ stats.warnings }}</span>
          <span class="text-xs opacity-60">refused sign-ins, cloned passkeys</span>
        </div>
      </div>

      <div v-if="!stats.total" class="card"><div class="card-body py-10 text-center text-sm opacity-50">No activity in the last {{ range }} days.</div></div>

      <template v-else>
        <!-- per day -->
        <div class="card">
          <div class="card-body space-y-3">
            <div class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <h2 class="text-base font-semibold">Events per day</h2>
              <p v-if="focus" class="text-xs opacity-70">
                <b>{{ dayShort(focus.day) }}</b> · {{ focus.total }} {{ focus.total === 1 ? 'event' : 'events' }}<template v-for="k in shownKinds" :key="k.id"><template v-if="focus.kinds[k.id]"> · {{ k.label }} {{ focus.kinds[k.id] }}</template></template>
                <span v-if="hover === null && busiest !== null" class="opacity-60"> (busiest)</span>
              </p>
            </div>
            <div class="bars" @mouseleave="hover = null">
              <div class="bars-axis" aria-hidden="true"><span>{{ top }}</span><span>{{ top / 2 }}</span><span>0</span></div>
              <div class="bars-plot" role="img" :aria-label="`Events per day over the last ${range} days`">
                <div class="bars-grid" aria-hidden="true"><i /><i /><i /></div>
                <button
                  v-for="(d, i) in stats.days"
                  :key="d.day"
                  type="button"
                  class="bar"
                  :class="{ 'is-on': hover === i, 'is-dim': hover !== null && hover !== i }"
                  :aria-label="`${dayShort(d.day)}: ${d.total} events`"
                  @mouseenter="hover = i"
                  @focus="hover = i"
                  @click="hover = i"
                >
                  <span class="bar-stack" :style="{ height: `${(d.total / top) * 100}%` }">
                    <i v-for="k in shownKinds" v-show="d.kinds[k.id]" :key="k.id" :style="{ flexGrow: d.kinds[k.id] ?? 0, background: kindColor(k.id) }" />
                  </span>
                </button>
              </div>
              <div />
              <div class="bars-x" aria-hidden="true">
                <span v-for="(d, i) in stats.days" :key="d.day">{{ i % tickEvery === 0 || i === stats.days.length - 1 ? dayShort(d.day) : '' }}</span>
              </div>
            </div>
            <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs opacity-80">
              <span v-for="k in shownKinds" :key="k.id" class="flex items-center gap-1.5"><i class="legend-dot" :style="{ background: kindColor(k.id) }" />{{ k.label }}</span>
            </div>
          </div>
        </div>

        <div class="grid gap-5 lg:grid-cols-2">
          <!-- kinds -->
          <div class="card">
            <div class="card-body space-y-3">
              <h2 class="text-base font-semibold">What kind of activity</h2>
              <div class="flex flex-wrap items-center gap-6">
                <svg viewBox="0 0 100 100" class="size-36 shrink-0 -rotate-90" role="img" aria-label="Share of each kind">
                  <circle cx="50" cy="50" :r="R" fill="none" stroke="var(--color-base-300)" stroke-width="12" opacity="0.5" />
                  <circle v-for="s in slices" :key="s.id" cx="50" cy="50" :r="R" fill="none" :stroke="kindColor(s.id)" stroke-width="12" :stroke-dasharray="s.dash" :stroke-dashoffset="s.offset"><title>{{ s.label }}: {{ s.n }}</title></circle>
                  <text x="50" y="50" class="rotate-90 origin-center" text-anchor="middle" dominant-baseline="central" fill="currentColor" font-size="15" font-weight="600">{{ stats.total }}</text>
                </svg>
                <ul class="min-w-40 flex-1 space-y-1.5 text-sm">
                  <li v-for="s in slices" :key="s.id" class="flex items-center gap-2">
                    <i class="legend-dot" :style="{ background: kindColor(s.id) }" />
                    <span class="flex-1">{{ s.label }}</span>
                    <span class="tabular-nums opacity-60">{{ s.n }}</span>
                    <span class="w-10 text-right tabular-nums font-medium">{{ pct(s.n, stats.total) }}%</span>
                  </li>
                </ul>
              </div>
            </div>
          </div>

          <!-- findings -->
          <div class="card">
            <div class="card-body space-y-3">
              <h2 class="text-base font-semibold">What stands out</h2>
              <ul class="space-y-2 text-sm">
                <li v-for="(f, i) in findings" :key="i" class="flex gap-2"><span class="mt-1.5 size-1.5 shrink-0 rounded-full bg-primary" />{{ f }}</li>
                <li v-if="stats.warnings" class="flex gap-2 text-warning"><span class="mt-1.5 size-1.5 shrink-0 rounded-full bg-warning" />{{ stats.warnings }} sign-in {{ stats.warnings === 1 ? 'warning' : 'warnings' }}: look for admin.oauth.rejected and passkey.clone-warning in the list.</li>
              </ul>
            </div>
          </div>
        </div>

        <!-- week × hour -->
        <div class="card">
          <div class="card-body space-y-3">
            <h2 class="text-base font-semibold">When in the week</h2>
            <div>
              <div class="heat">
                <template v-for="d in WEEK" :key="d">
                  <span class="heat-day">{{ weekday(d) }}</span>
                  <i v-for="h in 24" :key="h" class="heat-cell" :style="{ background: heatCell(stats.heat[d][h - 1]) }" :title="`${weekday(d)} ${hourRange(h - 1)}: ${stats.heat[d][h - 1]}`" />
                </template>
                <span />
                <span v-for="h in 24" :key="h" class="heat-hour">{{ (h - 1) % 6 === 0 ? String(h - 1).padStart(2, '0') : '' }}</span>
              </div>
            </div>
            <div class="flex items-center gap-2 text-xs opacity-70">
              Fewer <i v-for="n in 5" :key="n" class="heat-cell size-3" :style="{ background: `color-mix(in oklch, var(--color-primary) ${18 + (n - 1) * 20}%, transparent)` }" /> More
            </div>
          </div>
        </div>

        <div class="grid gap-5 lg:grid-cols-2">
          <!-- people -->
          <div class="card">
            <div class="card-body space-y-3">
              <h2 class="text-base font-semibold">Most active</h2>
              <ul class="space-y-2.5">
                <li v-for="a in stats.actors" :key="a.name" class="space-y-1">
                  <div class="flex items-center gap-2 text-sm">
                    <span class="min-w-0 flex-1 truncate">{{ who(a.name).name }}</span>
                    <span v-if="who(a.name).role" class="badge badge-ghost badge-xs">{{ who(a.name).role }}</span>
                    <span class="tabular-nums opacity-70">{{ a.count }}</span>
                  </div>
                  <div class="hbar"><i :style="{ width: `${(a.count / stats.actors[0].count) * 100}%` }" /></div>
                </li>
              </ul>
            </div>
          </div>
          <!-- actions -->
          <div class="card">
            <div class="card-body space-y-3">
              <h2 class="text-base font-semibold">Most frequent actions</h2>
              <ul class="space-y-2.5">
                <li v-for="a in stats.actions" :key="a.name" class="space-y-1">
                  <div class="flex items-center gap-2 text-sm">
                    <span class="min-w-0 flex-1 truncate font-mono text-xs">{{ a.name }}</span>
                    <span class="tabular-nums opacity-70">{{ a.count }}</span>
                  </div>
                  <div class="hbar"><i :style="{ width: `${(a.count / stats.actions[0].count) * 100}%`, background: kindColor(a.name.startsWith('member.') ? 'members' : a.name.startsWith('workspace.') ? 'workspaces' : /^(token|invite|device)\./.test(a.name) ? 'devices' : /^(admin|passkey)\./.test(a.name) ? 'admins' : a.name.startsWith('server.') ? 'server' : 'other') }" /></div>
                </li>
              </ul>
            </div>
          </div>
        </div>
      </template>
    </template>
    <div v-else-if="loading" class="flex justify-center py-16"><span class="loading loading-spinner" /></div>
  </div>
</template>
