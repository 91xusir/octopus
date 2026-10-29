import { memo, useCallback, useEffect, useId, useMemo, useRef, useState } from 'react';
import { Pencil, Trash2, ArrowDownToLine, ArrowUpFromLine } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import { useTranslations } from 'use-intl';
import { useUpdateModel, useDeleteModel, type LLMInfo } from '@/api/model';
import { getModelIcon } from '@/lib/model-icons';
import { toast } from 'sonner';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { ModelDeleteOverlay, ModelEditOverlay } from './ItemOverlays';
import { cn, formatModelPrice } from '@/lib/utils';
import { getUSDCNYRate, useDisplayCurrency } from '@/stores/setting';
import { createPortal } from 'react-dom';

// 价格以人民币输入时与美元互转的精度: 显示取 4 位小数, 存库换算回美元取 6 位, 避免来回切换累积误差。
const roundTo = (value: number, places: number) => Math.round(value * 10 ** places) / 10 ** places;

interface ModelItemProps {
    model: LLMInfo;
    layout?: 'grid' | 'list';
}

// 缓存编辑弹层实测高度，首次打开即可正确判断是否需要向上弹出
let cachedEditOverlayHeight = 0;

export const ModelItem = memo(function ModelItem({ model, layout = 'grid' }: ModelItemProps) {
    const t = useTranslations('model');
    // 卡片单价在渲染期经 formatModelPrice 读 store 换算, 订阅显示货币让切换立即重渲染, 不必等列表重新拉取。
    useDisplayCurrency();
    const isListLayout = layout === 'list';
    const [isEditOpen, setIsEditOpen] = useState(false);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const [overlayRect, setOverlayRect] = useState<{ top: number; left: number; width: number } | null>(null);
    const instanceId = useId();
    const editLayoutId = `edit-btn-${model.name}-${instanceId}`;
    const deleteLayoutId = `delete-btn-${model.name}-${instanceId}`;
    const cardRef = useRef<HTMLElement | null>(null);
    const [editCurrency, setEditCurrency] = useState<'USD' | 'CNY'>('USD');
    const [editValues, setEditValues] = useState(() => ({
        input: model.input.toString(),
        output: model.output.toString(),
        cache_read: model.cache_read.toString(),
        cache_write: model.cache_write.toString(),
    }));

    const updateModel = useUpdateModel();
    const deleteModel = useDeleteModel();

    const { Icon, className: iconClassName, color: brandColor } = useMemo(() => getModelIcon(model.name), [model.name]);

    // 卡片单价展示随显示货币换算; 编辑货币切换时把已填数值按汇率重定基, 数值含义保持不变。
    const toEditValues = useCallback((currency: 'USD' | 'CNY') => ({
        input: (currency === 'CNY' ? roundTo(model.input * getUSDCNYRate(), 4) : model.input).toString(),
        output: (currency === 'CNY' ? roundTo(model.output * getUSDCNYRate(), 4) : model.output).toString(),
        cache_read: (currency === 'CNY' ? roundTo(model.cache_read * getUSDCNYRate(), 4) : model.cache_read).toString(),
        cache_write: (currency === 'CNY' ? roundTo(model.cache_write * getUSDCNYRate(), 4) : model.cache_write).toString(),
    }), [model.input, model.output, model.cache_read, model.cache_write]);

    const handleEditCurrencyChange = useCallback((currency: 'USD' | 'CNY') => {
        if (currency === editCurrency) return;
        const rate = getUSDCNYRate();
        const factor = currency === 'CNY' ? rate : 1 / rate;
        setEditCurrency(currency);
        setEditValues((values) => ({
            input: roundTo((parseFloat(values.input) || 0) * factor, 6).toString(),
            output: roundTo((parseFloat(values.output) || 0) * factor, 6).toString(),
            cache_read: roundTo((parseFloat(values.cache_read) || 0) * factor, 6).toString(),
            cache_write: roundTo((parseFloat(values.cache_write) || 0) * factor, 6).toString(),
        }));
    }, [editCurrency]);

    const updateOverlayRect = useCallback(() => {
        const card = cardRef.current;
        if (!card) return;
        const rect = card.getBoundingClientRect();
        const height = cachedEditOverlayHeight;
        // 下方空间不足时改为向上弹出，避免最后一行弹层被视口底部截断
        const flipUp = height > 0 && rect.top + height > window.innerHeight;
        const top = flipUp
            ? Math.min(Math.max(rect.bottom - height, 0), Math.max(window.innerHeight - height, 0))
            : rect.top;
        setOverlayRect((prev) => {
            if (prev && prev.top === top && prev.left === rect.left && prev.width === rect.width) {
                return prev;
            }
            return { top, left: rect.left, width: rect.width };
        });
    }, []);

    const closeEdit = useCallback(() => {
        setIsEditOpen(false);
    }, []);

    const handleOverlayHeightChange = useCallback((height: number) => {
        if (height === cachedEditOverlayHeight) return;
        cachedEditOverlayHeight = height;
        updateOverlayRect();
    }, [updateOverlayRect]);

    const handleEditClick = () => {
        setConfirmDelete(false);
        setEditValues(toEditValues(editCurrency));
        // Ensure first open already has anchor geometry so layout animation can run.
        updateOverlayRect();
        setIsEditOpen(true);
    };

    const handleCancelEdit = () => {
        closeEdit();
    };

    const handleSaveEdit = () => {
        // 人民币输入按汇率换算回美元存储, 数据库始终只保存美元价格。
        const toUSD = (value: string) => {
            const parsed = parseFloat(value) || 0;
            return editCurrency === 'CNY' ? roundTo(parsed / getUSDCNYRate(), 6) : parsed;
        };
        updateModel.mutate({
            name: model.name,
            input: toUSD(editValues.input),
            output: toUSD(editValues.output),
            cache_read: toUSD(editValues.cache_read),
            cache_write: toUSD(editValues.cache_write),
        }, {
            onSuccess: () => {
                closeEdit();
                toast.success(t('toast.updated'));
            },
            onError: (error) => {
                toast.error(t('toast.updateFailed'), { description: error.message });
            }
        });
    };

    const handleDeleteClick = () => {
        closeEdit();
        setConfirmDelete(true);
    };
    const handleCancelDelete = () => setConfirmDelete(false);
    const handleConfirmDelete = () => {
        deleteModel.mutate(model.name, {
            onSuccess: () => {
                setConfirmDelete(false);
                toast.success(t('toast.deleted'));
            },
            onError: (error) => {
                setConfirmDelete(false);
                toast.error(t('toast.deleteFailed'), { description: error.message });
            }
        });
    };

    useEffect(() => {
        if (!isEditOpen) return;

        const handleKeyDown = (event: KeyboardEvent) => {
            if (event.key === 'Escape') closeEdit();
        };

        updateOverlayRect();
        window.addEventListener('resize', updateOverlayRect);
        window.addEventListener('scroll', updateOverlayRect, true);
        document.addEventListener('keydown', handleKeyDown);

        return () => {
            window.removeEventListener('resize', updateOverlayRect);
            window.removeEventListener('scroll', updateOverlayRect, true);
            document.removeEventListener('keydown', handleKeyDown);
        };
    }, [isEditOpen, updateOverlayRect, closeEdit]);

    const shouldRenderEditPortal = isEditOpen || overlayRect !== null;

    return (
        <article
            ref={cardRef}
            className={cn(
                'group relative rounded-3xl border border-border bg-card flex items-center gap-3 p-4',
                (isEditOpen || confirmDelete) && 'z-50'
            )}
        >
            <Icon aria-hidden="true" className={iconClassName} width={52} height={52} />

            <div className="flex-1 min-w-0 flex flex-col justify-center gap-2">
                <Tooltip>
                    <TooltipTrigger asChild>
                        <span className="w-fit max-w-full text-base font-semibold text-card-foreground leading-tight truncate">
                            {model.name}
                        </span>
                    </TooltipTrigger>
                    <TooltipContent key={model.name} side="top" sideOffset={10} align="center">
                        {model.name}
                    </TooltipContent>
                </Tooltip>

                {isListLayout ? (
                    <p className="flex items-center gap-2 overflow-hidden text-sm text-muted-foreground whitespace-nowrap">
                        <span className="inline-flex items-center gap-1">
                            <ArrowDownToLine className="size-3.5 shrink-0" style={{ color: brandColor }} />
                            {t('card.inputCache')}
                            <span className="tabular-nums">{formatModelPrice(model.input)}/{formatModelPrice(model.cache_read)}</span>
                        </span>
                        <span className="text-muted-foreground/60">|</span>
                        <span className="inline-flex items-center gap-1 overflow-hidden">
                            <ArrowUpFromLine className="size-3.5 shrink-0" style={{ color: brandColor }} />
                            {t('card.outputCache')}
                            <span className="tabular-nums truncate">{formatModelPrice(model.output)}/{formatModelPrice(model.cache_write)}</span>
                        </span>
                    </p>
                ) : (
                    <>
                        <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
                            <ArrowDownToLine className="size-3.5" style={{ color: brandColor }} />
                            {t('card.inputCache')}
                            <span className="tabular-nums">{formatModelPrice(model.input)}/{formatModelPrice(model.cache_read)}</span>
                        </p>

                        <p className="flex items-center gap-1.5 text-sm text-muted-foreground">
                            <ArrowUpFromLine className="size-3.5" style={{ color: brandColor }} />
                            {t('card.outputCache')}
                            <span className="tabular-nums">{formatModelPrice(model.output)}/{formatModelPrice(model.cache_write)}</span>
                        </p>
                    </>
                )}
            </div>

            <div
                className={cn(
                    isListLayout
                        ? 'shrink-0 flex items-center gap-2 self-center'
                        : 'shrink-0 flex flex-col justify-between self-stretch',
                    (isEditOpen || confirmDelete) && 'invisible pointer-events-none'
                )}
            >
                <motion.button
                    layoutId={editLayoutId}
                    type="button"
                    onClick={handleEditClick}
                    disabled={isEditOpen || confirmDelete}
                    className="h-9 w-9 flex items-center justify-center rounded-lg bg-muted/60 text-muted-foreground transition-colors hover:bg-muted disabled:opacity-50"
                >
                    <Pencil className="size-4" />
                </motion.button>

                <motion.button
                    layoutId={deleteLayoutId}
                    type="button"
                    onClick={handleDeleteClick}
                    disabled={isEditOpen || confirmDelete}
                    className="h-9 w-9 flex items-center justify-center rounded-lg bg-destructive/10 text-destructive transition-colors hover:bg-destructive hover:text-destructive-foreground disabled:opacity-50"
                >
                    <Trash2 className="size-4" />
                </motion.button>
            </div>

            <AnimatePresence>
                {confirmDelete && (
                    <ModelDeleteOverlay
                        layoutId={deleteLayoutId}
                        isPending={deleteModel.isPending}
                        onCancel={handleCancelDelete}
                        onConfirm={handleConfirmDelete}
                    />
                )}
            </AnimatePresence>

            {shouldRenderEditPortal && typeof document !== 'undefined'
                ? createPortal(
                    <AnimatePresence onExitComplete={() => setOverlayRect(null)}>
                        {isEditOpen && overlayRect && (
                            <div
                                className="fixed z-[90]"
                                style={{
                                    top: `${overlayRect.top}px`,
                                    left: `${overlayRect.left}px`,
                                    width: `${overlayRect.width}px`,
                                }}
                            >
                                <ModelEditOverlay
                                    layoutId={editLayoutId}
                                    onHeightChange={handleOverlayHeightChange}
                                    modelName={model.name}
                                    brandColor={brandColor}
                                    editValues={editValues}
                                    isPending={updateModel.isPending}
                                    currency={editCurrency}
                                    onCurrencyChange={handleEditCurrencyChange}
                                    onChange={setEditValues}
                                    onCancel={handleCancelEdit}
                                    onSave={handleSaveEdit}
                                />
                            </div>
                        )}
                    </AnimatePresence>,
                    document.body
                )
                : null}
        </article>
    );
});
