const AppLayout = {
    template: `
    <div class="flex h-screen overflow-hidden bg-neutral-950">
        <sidebar
            :current-view="galleryState.currentView"
            @navigate="navigate"
            class="desktop-only"
        ></sidebar>

        <main class="flex-1 scroll-container" ref="mainScroll">
            <transition name="fade" mode="out-in">
                <gallery-view v-if="galleryState.currentView === 'gallery'" :key="'gallery'"></gallery-view>
                <faces-view v-else-if="galleryState.currentView === 'faces'" :key="'faces'"></faces-view>
                <search-view v-else-if="galleryState.currentView === 'search'" :key="'search'"></search-view>
                <dupes-view v-else-if="galleryState.currentView === 'dupes'" :key="'dupes'"></dupes-view>
                <stats-view v-else-if="galleryState.currentView === 'stats'" :key="'stats'"></stats-view>
            </transition>
            <div class="h-20 md:hidden"></div>
        </main>

        <nav class="bottom-nav md:hidden">
            <div v-for="item in navItems" :key="item.id"
                 class="bottom-nav-item"
                 :class="{ active: galleryState.currentView === item.id }"
                 @click="navigate(item.id)">
                <i :data-lucide="item.icon"></i>
                <span>{{ item.label }}</span>
            </div>
        </nav>

        <lightbox
            :file="galleryState.lightboxFile"
            :files="galleryState.lightboxFiles"
            :index="galleryState.lightboxIndex"
            @close="closeLightbox"
            @navigate="navigateLightbox"
        ></lightbox>
    </div>
    `,
    setup() {
        const mainScroll = Vue.ref(null);
        const navItems = [
            { id: 'gallery', label: 'Gallery', icon: 'grid-3x3' },
            { id: 'faces', label: 'Faces', icon: 'scan-face' },
            { id: 'search', label: 'Search', icon: 'search' },
            { id: 'dupes', label: 'Dupes', icon: 'copy' },
            { id: 'stats', label: 'Stats', icon: 'bar-chart-3' },
        ];

        function navigate(view) {
            galleryState.currentView = view;
            Vue.nextTick(() => {
                if (mainScroll.value) mainScroll.value.scrollTop = 0;
            });
        }

        function closeLightbox() {
            galleryState.lightboxFile = null;
            galleryState.lightboxFiles = [];
            galleryState.lightboxIndex = -1;
        }

        function navigateLightbox(newIndex) {
            if (newIndex < 0 || newIndex >= galleryState.lightboxFiles.length) return;
            galleryState.lightboxIndex = newIndex;
            galleryState.lightboxFile = galleryState.lightboxFiles[newIndex];
        }

        Vue.onMounted(() => lucide.createIcons());
        Vue.onUpdated(() => lucide.createIcons());

        return { galleryState, navigate, navItems, mainScroll, closeLightbox, navigateLightbox };
    }
};
