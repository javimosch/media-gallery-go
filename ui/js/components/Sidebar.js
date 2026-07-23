const Sidebar = {
    props: {
        currentView: String
    },
    emits: ['navigate'],
    template: `
    <aside class="sidebar-desktop">
        <div class="p-5 border-b border-neutral-800">
            <h1 class="text-lg font-bold text-white flex items-center gap-2">
                <i data-lucide="images" class="w-5 h-5 text-blue-400"></i>
                Media Gallery
            </h1>
            <p class="text-xs text-neutral-600 mt-1" v-if="stats">{{ stats.total_files?.toLocaleString() }} files · {{ formatSize(stats.total_size) }}</p>
            <p class="text-xs text-neutral-600 mt-1" v-else>Loading...</p>
        </div>
        <nav class="flex-1 p-3 space-y-1">
            <div v-for="item in navItems" :key="item.id"
                 class="sidebar-link"
                 :class="{ active: currentView === item.id }"
                 @click="$emit('navigate', item.id)">
                <i :data-lucide="item.icon" class="w-4 h-4"></i>
                {{ item.label }}
            </div>
        </nav>
    </aside>
    `,
    setup() {
        const stats = Vue.ref(null);
        const navItems = [
            { id: 'gallery', label: 'Gallery', icon: 'grid-3x3' },
            { id: 'faces', label: 'Faces', icon: 'scan-face' },
            { id: 'search', label: 'Search', icon: 'search' },
            { id: 'dupes', label: 'Duplicates', icon: 'copy' },
            { id: 'stats', label: 'Stats', icon: 'bar-chart-3' },
        ];

        async function loadStats() {
            try { stats.value = await fetchJSON('/api/stats'); } catch (e) {}
        }

        Vue.onMounted(() => { loadStats(); lucide.createIcons(); });
        Vue.onUpdated(() => lucide.createIcons());
        return { navItems, stats, formatSize };
    }
};
