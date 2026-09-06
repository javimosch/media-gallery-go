const ThumbGrid = {
    props: {
        files: Array,
        loading: Boolean,
        hasMore: Boolean,
    },
    emits: ['load-more', 'open-file'],
    template: `
    <div>
        <div v-if="files.length === 0 && !loading" class="text-center text-neutral-600 py-20">
            <i data-lucide="image-off" class="w-12 h-12 mx-auto mb-4 opacity-40"></i>
            <p class="text-sm">No files found</p>
        </div>
        <div class="thumb-grid" v-if="files.length > 0">
            <div v-for="(file, i) in files" :key="file.sha256 + file.relpath"
                 class="thumb-item"
                 @click="$emit('open-file', file)">
                <img :src="'/api/thumb/' + file.sha256"
                     :alt="file.relpath"
                     loading="lazy"
                     decoding="async"
                     draggable="false"
                     @error="onImgError">
                <span v-if="isVideo(file.relpath)" class="video-badge">
                    <i data-lucide="play" class="w-3 h-3"></i>
                </span>
            </div>
        </div>
        <div v-if="loading" class="loading-spinner"></div>
        <div v-if="hasMore && !loading && files.length > 0"
             class="scroll-sentinel"
             ref="sentinel">
            <button v-if="!infiniteScroll" class="load-more-btn" @click="$emit('load-more')">Load more</button>
            <div v-else class="loading-spinner"></div>
        </div>
    </div>
    `,
    setup(props, { emit }) {
        const infiniteScroll = Vue.computed(() => galleryState.settings.infiniteScroll);
        let observer = null;

        function isVideo(path) {
            return ['mp4', 'mov', 'avi', 'mkv', 'wmv'].includes(path.toLowerCase().split('.').pop());
        }
        function onImgError(e) {
            e.target.style.opacity = '0.2';
        }

        function setupObserver() {
            if (observer) { observer.disconnect(); observer = null; }
            if (!infiniteScroll.value) return;
            const scrollRoot = document.querySelector('.scroll-container');
            observer = new IntersectionObserver((entries) => {
                if (entries[0].isIntersecting && props.hasMore && !props.loading) {
                    emit('load-more');
                }
            }, { root: scrollRoot || null, rootMargin: '400px' });
            Vue.nextTick(() => {
                const el = document.querySelector('.scroll-sentinel');
                if (el) observer.observe(el);
            });
        }

        Vue.onMounted(() => {
            lucide.createIcons();
            setupObserver();
        });

        Vue.onUpdated(() => {
            lucide.createIcons();
            // Re-observe sentinel after DOM updates (new content may have shifted it)
            if (infiniteScroll.value && observer) {
                Vue.nextTick(() => {
                    const el = document.querySelector('.scroll-sentinel');
                    if (el) { observer.disconnect(); observer.observe(el); }
                });
            }
        });

        Vue.onUnmounted(() => {
            if (observer) observer.disconnect();
        });

        Vue.watch(infiniteScroll, () => setupObserver());

        return { isVideo, onImgError, infiniteScroll };
    }
};
