<template>
    <div class="toast-container">
        <TransitionGroup name="toast">
            <div
                v-for="t in toasts"
                :key="t.id"
                class="toast"
                :class="`toast-${t.type}`"
                @click="dismiss(t.id)"
            >
                <div class="toast-icon">{{ typeIcon(t.type) }}</div>
                <div class="toast-message">{{ t.message }}</div>
                <button class="toast-close" @click.stop="dismiss(t.id)">
                    ×
                </button>

                <!-- Progress bar for auto-dismiss -->
                <div
                    v-if="t.duration > 0"
                    class="toast-progress"
                    :style="{
                        width: toastProgress(t) + '%',
                        transition: `width ${t.duration}ms linear`,
                    }"
                ></div>
            </div>
        </TransitionGroup>
    </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { toast, type Toast } from "../lib/toast";

const toasts = ref<Toast[]>([]);

let unsubscribe: (() => void) | null = null;

onMounted(() => {
    toasts.value = toast.getToasts();
    unsubscribe = toast.subscribe((updated: Toast[]) => {
        toasts.value = updated;
    });
});

onUnmounted(() => {
    unsubscribe?.();
});

function dismiss(id: string) {
    toast.dismiss(id);
}

function typeIcon(type: string): string {
    switch (type) {
        case "success":
            return "✅";
        case "error":
            return "❌";
        case "warning":
            return "⚠️";
        case "info":
            return "ℹ️";
        default:
            return "📢";
    }
}

function toastProgress(t: Toast): number {
    const elapsed = Date.now() - t.createdAt;
    const remaining = Math.max(0, 100 - (elapsed / t.duration) * 100);
    return remaining;
}
</script>

<style scoped>
.toast-container {
    position: fixed;
    top: 16px;
    right: 16px;
    z-index: 10000;
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-width: 420px;
}

.toast {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    padding: 12px 16px;
    border-radius: 8px;
    background: var(--color-surface, #1e293b);
    border: 1px solid var(--color-border, #334155);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
    cursor: pointer;
    position: relative;
    overflow: hidden;
    animation: toast-in 0.3s ease;
}

.toast-success {
    border-left: 4px solid #22c55e;
}

.toast-error {
    border-left: 4px solid #ef4444;
}

.toast-warning {
    border-left: 4px solid #f59e0b;
}

.toast-info {
    border-left: 4px solid #3b82f6;
}

.toast-icon {
    font-size: 1.2rem;
    flex-shrink: 0;
}

.toast-message {
    flex: 1;
    font-size: 0.9rem;
    color: var(--color-text, #e2e8f0);
    line-height: 1.4;
}

.toast-close {
    background: none;
    border: none;
    color: var(--color-text-muted, #94a3b8);
    font-size: 1.5rem;
    line-height: 1;
    cursor: pointer;
    padding: 0 4px;
    flex-shrink: 0;
    transition: color 0.2s;
}

.toast-close:hover {
    color: var(--color-text, #e2e8f0);
}

.toast-progress {
    position: absolute;
    bottom: 0;
    left: 0;
    height: 3px;
    background: var(--color-accent, #6366f1);
    opacity: 0.5;
}

/* Transitions */
.toast-enter-active {
    animation: toast-in 0.3s ease;
}

.toast-leave-active {
    animation: toast-out 0.3s ease;
}

@keyframes toast-in {
    from {
        opacity: 0;
        transform: translateX(100%);
    }
    to {
        opacity: 1;
        transform: translateX(0);
    }
}

@keyframes toast-out {
    from {
        opacity: 1;
        transform: translateX(0);
    }
    to {
        opacity: 0;
        transform: translateX(100%);
    }
}

@media (max-width: 768px) {
    .toast-container {
        top: 8px;
        right: 8px;
        left: 8px;
        max-width: none;
    }

    .toast {
        padding: 10px 12px;
    }

    .toast-message {
        font-size: 0.85rem;
    }
}
</style>
