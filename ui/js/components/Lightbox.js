const Lightbox = {
    props: {
        file: Object,
        files: { type: Array, default: () => [] },
        index: { type: Number, default: -1 },
    },
    emits: ['close', 'navigate'],
    template: `
    <transition name="fade">
        <div v-if="file" class="lightbox-overlay" @click.self="$emit('close')"
             @touchstart="onTouchStart" @touchmove="onTouchMove" @touchend="onTouchEnd"
             tabindex="0" ref="overlay">
            <button class="lightbox-close" @click="$emit('close')">
                <i data-lucide="x" class="w-6 h-6"></i>
            </button>

            <button v-if="hasPrev" class="lightbox-nav lightbox-prev" @click.stop="prev">
                <i data-lucide="chevron-left" class="w-6 h-6"></i>
            </button>
            <button v-if="hasNext" class="lightbox-nav lightbox-next" @click.stop="next">
                <i data-lucide="chevron-right" class="w-6 h-6"></i>
            </button>

            <img v-if="isImage(file.relpath)"
                 :src="imgSrc"
                 :alt="file.relpath"
                 :style="{ transform: 'translateY(' + dragY + 'px)' }"
                 draggable="false"
                 @load="onImgLoad">
            <video v-else-if="isVideo(file.relpath)"
                   :src="'/api/stream/' + file.sha256"
                   controls autoplay playsinline
                   :style="{ transform: 'translateY(' + dragY + 'px)' }"
                   class="rounded-lg">
            </video>
            <div v-else class="text-white text-center">
                <p class="mb-4">Preview not available</p>
                <a :href="'/api/download/' + file.sha256"
                   class="px-4 py-2 bg-blue-600 rounded-lg">
                    Download
                </a>
            </div>

            <div class="lightbox-info">
                <div>{{ file.relpath }}</div>
                <div class="text-neutral-400 mt-1">
                    {{ formatSize(file.size) }}
                    <span v-if="index >= 0 && files.length > 0" class="lightbox-position">
                        &middot; {{ index + 1 }} / {{ files.length }}
                    </span>
                </div>
                <div v-if="fastGallery && isImage(file.relpath) && !fullRes" class="lightbox-hint">
                    Press <kbd>Space</kbd> for full resolution
                </div>
                <div v-if="fastGallery && fullRes" class="lightbox-hint">
                    Loading full resolution...
                </div>
            </div>
        </div>
    </transition>
    `,
    setup(props, { emit }) {
        const dragY = Vue.ref(0);
        const fullRes = Vue.ref(false);
        const imgSrc = Vue.ref('');
        let startY = 0;
        let dragging = false;

        const fastGallery = Vue.computed(() => galleryState.settings.fastGallery);

        const hasPrev = Vue.computed(() => props.index > 0);
        const hasNext = Vue.computed(() => props.index >= 0 && props.index < props.files.length - 1);

        function isImage(path) {
            return ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'tif', 'tiff'].includes(path.toLowerCase().split('.').pop());
        }
        function isVideo(path) {
            return ['mp4', 'mov', 'avi', 'mkv', 'wmv'].includes(path.toLowerCase().split('.').pop());
        }
        function formatSize(bytes) {
            if (bytes < 1073741824) return (bytes / 1048576).toFixed(1) + ' MB';
            return (bytes / 1073741824).toFixed(2) + ' GB';
        }

        function updateImgSrc() {
            if (!props.file) return;
            if (fastGallery.value && isImage(props.file.relpath)) {
                imgSrc.value = fullRes.value
                    ? '/api/download/' + props.file.sha256
                    : '/api/thumb/' + props.file.sha256;
            } else {
                imgSrc.value = '/api/download/' + props.file.sha256;
            }
        }

        function onImgLoad() {
            // img loaded
        }

        function prev() {
            if (hasPrev.value) {
                fullRes.value = false;
                emit('navigate', props.index - 1);
            }
        }

        function next() {
            if (hasNext.value) {
                fullRes.value = false;
                emit('navigate', props.index + 1);
            }
        }

        function toggleFullRes() {
            if (!fastGallery.value || !isImage(props.file?.relpath || '')) return;
            fullRes.value = !fullRes.value;
            updateImgSrc();
        }

        function onKeydown(e) {
            if (!props.file) return;
            switch (e.key) {
                case 'ArrowLeft':
                case 'a':
                case 'A':
                    e.preventDefault(); prev(); break;
                case 'ArrowRight':
                case 'd':
                case 'D':
                    e.preventDefault(); next(); break;
                case ' ':
                    e.preventDefault(); toggleFullRes(); break;
                case 'Escape':
                    e.preventDefault(); emit('close'); break;
            }
        }

        function onTouchStart(e) {
            if (e.touches.length === 1) {
                startY = e.touches[0].clientY;
                dragging = true;
            }
        }
        function onTouchMove(e) {
            if (dragging && e.touches.length === 1) {
                dragY.value = Math.max(0, e.touches[0].clientY - startY);
            }
        }
        function onTouchEnd() {
            if (dragY.value > 100) {
                emit('close');
            }
            dragY.value = 0;
            dragging = false;
        }

        function preloadAdjacent() {
            if (props.index < 0) return;
            if (props.index > 0) {
                const f = props.files[props.index - 1];
                if (f && isImage(f.relpath)) {
                    const img = new Image();
                    img.src = '/api/thumb/' + f.sha256;
                }
            }
            if (props.index < props.files.length - 1) {
                const f = props.files[props.index + 1];
                if (f && isImage(f.relpath)) {
                    const img = new Image();
                    img.src = '/api/thumb/' + f.sha256;
                }
            }
        }

        // Reset state and update src when file changes
        Vue.watch(() => props.file, () => {
            fullRes.value = false;
            dragY.value = 0;
            dragging = false;
            updateImgSrc();
            preloadAdjacent();
            Vue.nextTick(() => {
                if (props.file) {
                    document.addEventListener('keydown', onKeydown);
                }
            });
        });

        Vue.watch(fastGallery, () => updateImgSrc());

        Vue.onMounted(() => {
            if (props.file) {
                document.addEventListener('keydown', onKeydown);
                updateImgSrc();
                preloadAdjacent();
            }
        });

        Vue.onUnmounted(() => {
            document.removeEventListener('keydown', onKeydown);
        });

        Vue.onUpdated(() => lucide.createIcons());

        return { isImage, isVideo, formatSize, dragY, imgSrc, fastGallery, fullRes,
                 hasPrev, hasNext, prev, next, onImgLoad,
                 onTouchStart, onTouchMove, onTouchEnd };
    }
};
