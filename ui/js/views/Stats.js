const StatsView = {
    template: `
    <div>
        <div class="top-bar">
            <h1>Stats</h1>
        </div>
        <div class="p-4 max-w-2xl mx-auto">
            <div v-if="loading" class="loading-spinner"></div>
            <div v-else-if="stats">
                <div class="grid grid-cols-2 gap-3 mb-6">
                    <div class="stat-card">
                        <div class="stat-value">{{ stats.total_files?.toLocaleString() }}</div>
                        <div class="stat-label">Total files</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value">{{ formatSize(stats.total_size) }}</div>
                        <div class="stat-label">Total size</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value text-amber-400">{{ stats.dupe_groups?.toLocaleString() }}</div>
                        <div class="stat-label">Dupe groups</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-value text-amber-400">{{ formatSize(stats.dupe_size) }}</div>
                        <div class="stat-label">Dupe size</div>
                    </div>
                </div>
                <div v-if="stats.misfiled_count > 0" class="stat-card mb-6" style="border-color: #92400e;">
                    <div class="flex items-center gap-2 mb-2">
                        <i data-lucide="alert-triangle" class="w-5 h-5 text-amber-400"></i>
                        <span class="font-semibold text-amber-400">Misfiled</span>
                    </div>
                    <div class="text-sm text-neutral-300">
                        {{ stats.misfiled_count?.toLocaleString() }} files ({{ formatSize(stats.misfiled_size) }}) in wrong folders
                    </div>
                </div>
                <h3 class="text-sm font-semibold text-neutral-400 mb-3 uppercase tracking-wide">Categories</h3>
                <div class="space-y-2">
                    <div v-for="cat in stats.categories" :key="cat.category"
                         class="stat-card">
                        <div class="flex items-center justify-between">
                            <div>
                                <div class="font-medium text-sm">{{ shortName(cat.category) }}</div>
                                <div class="text-xs text-neutral-500 mt-1">{{ cat.count?.toLocaleString() }} files</div>
                            </div>
                            <div class="text-lg font-bold">{{ formatSize(cat.size) }}</div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
    `,
    setup() {
        const stats = Vue.ref(null);
        const loading = Vue.ref(true);

        function shortName(c) { return c.replace('family-media-', '').replace('-sorted', ''); }
        function formatSize(b) {
            if (!b) return '0 B';
            if (b < 1048576) return (b / 1024).toFixed(0) + ' KB';
            if (b < 1073741824) return (b / 1048576).toFixed(1) + ' MB';
            return (b / 1073741824).toFixed(2) + ' GB';
        }

        Vue.onMounted(async () => {
            try { stats.value = await fetchJSON('/api/stats'); } catch (e) { console.error(e); }
            loading.value = false;
            lucide.createIcons();
        });

        return { stats, loading, formatSize, shortName };
    }
};
