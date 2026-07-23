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
            <div v-for="file in files" :key="file.sha256 + file.relpath"
                 class="thumb-item"
                 @click="$emit('open-file', file)">
                <img :src="'/api/thumb/' + file.sha256"
                     :alt="file.relpath"
                     loading="lazy"
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
            <button class="load-more-btn" @click="$emit('load-more')">Load more</button>
        </div>
    </div>
    `,
    setup(props, { emit }) {
        function isVideo(path) {
            return ['mp4', 'mov', 'avi', 'mkv', 'wmv'].includes(path.toLowerCase().split('.').pop());
        }
        function onImgError(e) {
            e.target.style.opacity = '0.2';
        }

        // Infinite scroll via IntersectionObserver
        Vue.onMounted(() => {
            lucide.createIcons();
            const observer = new IntersectionObserver((entries) => {
                if (entries[0].isIntersecting && props.hasMore && !props.loading) {
                    emit('load-more');
                }
            }, { rootMargin: '200px' });
            Vue.nextTick(() => {
                const el = document.querySelector('.scroll-sentinel');
                if (el) observer.observe(el);
            });
            // Store for cleanup
            window._thumbObserver = observer;
        });

        Vue.onUpdated(() => lucide.createIcons());

        return { isVideo, onImgError };
    }
};
