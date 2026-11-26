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
        <div class="sidebar-settings">
            <div class="sidebar-settings-header" @click="showSettings = !showSettings">
                <i data-lucide="settings" class="w-4 h-4"></i>
                <span>Settings</span>
                <span class="sidebar-settings-chevron" :class="{ open: showSettings }">&#8250;</span>
            </div>
            <div v-if="showSettings" class="sidebar-settings-body">
                <label class="setting-toggle">
                    <input type="checkbox" v-model="galleryState.settings.infiniteScroll" />
                    <span>Infinite scroll</span>
                </label>
                <label class="setting-toggle">
                    <input type="checkbox" v-model="galleryState.settings.fastGallery" />
                    <span>Fast gallery mode</span>
                </label>
                <div class="setting-hint">
                    <kbd>A</kbd>/<kbd>D</kbd> or arrows to navigate<br>
                    <kbd>Space</kbd> for full resolution
                </div>
            </div>
        </div>
        <div class="sidebar-hidden" v-if="stats && stats.hidden_files > 0">
            {{ stats.hidden_files.toLocaleString() }} files hidden: replica offline
        </div>
        <div class="sidebar-replicas" v-if="replicas.length > 0">
            <div class="sidebar-replicas-header">
                <i data-lucide="server" class="w-3.5 h-3.5"></i>
                <span>Replicas</span>
            </div>
            <div v-for="rep in replicas" :key="rep.name" class="sidebar-replica">
                <span class="replica-dot" :class="rep.status"></span>
                <span class="sidebar-replica-name">{{ rep.name }}</span>
                <span class="sidebar-replica-files">{{ rep.files?.toLocaleString() }}</span>
            </div>
        </div>
    </aside>
    `,
    setup() {
        const stats = Vue.ref(null);
        const replicas = Vue.ref([]);
        const showSettings = Vue.ref(false);
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

        async function loadReplicas() {
            try {
                const res = await fetchJSON('/api/replicas');
                replicas.value = res.replicas || [];
            } catch (e) { replicas.value = []; }
        }

        Vue.onMounted(() => {
            loadStats();
            loadReplicas();
            setInterval(loadReplicas, 60000);
            lucide.createIcons();
        });
        Vue.onUpdated(() => lucide.createIcons());
        return { navItems, stats, replicas, showSettings, galleryState, formatSize };
    }
};
