import { useEffect, useState, useRef } from 'react';
import { useTranslations } from 'use-intl';
import { useQueryClient } from '@tanstack/react-query';
import { Banknote, Clock, Coins, DatabaseBackup, DollarSign, RefreshCw } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useSettingList, useSetSetting, SettingKey } from '@/api/setting';
import { useRebuildModelPrice, useUpdateModelPrice, useLastUpdateTime } from '@/api/model';
import { useDisplayCurrency, useSettingStore, type DisplayCurrency } from '@/stores/setting';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';

export function SettingLLMPrice() {
    const t = useTranslations('setting');
    const { data: settings } = useSettingList();
    const setSetting = useSetSetting();
    const updatePrice = useUpdateModelPrice();
    const rebuildPrice = useRebuildModelPrice();
    const { data: lastUpdateTime } = useLastUpdateTime();
    const queryClient = useQueryClient();
    const displayCurrency = useDisplayCurrency();

    const [updateInterval, setUpdateInterval] = useState('');
    const [rate, setRate] = useState('');
    const [confirmRebuild, setConfirmRebuild] = useState(false);
    const [rebuildCountdown, setRebuildCountdown] = useState(0);
    const initialUpdateInterval = useRef('');
    const initialRate = useRef('');

    useEffect(() => {
        if (settings) {
            const interval = settings.find(s => s.key === SettingKey.ModelInfoUpdateInterval);
            if (interval) {
                queueMicrotask(() => setUpdateInterval(interval.value));
                initialUpdateInterval.current = interval.value;
            }
            const rateSetting = settings.find(s => s.key === SettingKey.USDCNYRate);
            if (rateSetting) {
                queueMicrotask(() => setRate(rateSetting.value));
                initialRate.current = rateSetting.value;
            }
        }
    }, [settings]);

    useEffect(() => {
        if (!confirmRebuild || rebuildCountdown <= 0) return;
        const timer = window.setTimeout(() => setRebuildCountdown((seconds) => seconds - 1), 1000);
        return () => window.clearTimeout(timer);
    }, [confirmRebuild, rebuildCountdown]);

    const handleSave = (key: string, value: string, initialValue: string) => {
        if (value === initialValue) return;

        setSetting.mutate({ key, value }, {
            onSuccess: () => {
                toast.success(t('saved'));
                if (key === SettingKey.ModelInfoUpdateInterval) initialUpdateInterval.current = value;
                if (key === SettingKey.USDCNYRate) initialRate.current = value;
            },
            onError: (error) => {
                toast.error(t('llmPrice.updateFailed'), { description: error.message });
            }
        });
    };

    // 显示货币是纯前端偏好: 切换后失效全部查询, 让缓存里按美元格式化好的金额按新货币重算。
    const handleDisplayCurrencyChange = (currency: DisplayCurrency) => {
        if (currency === useSettingStore.getState().displayCurrency) return;
        useSettingStore.getState().setDisplayCurrency(currency);
        void queryClient.invalidateQueries();
    };

    const handleManualUpdate = () => {
        updatePrice.mutate(undefined, {
            onSuccess: () => {
                toast.success(t('llmPrice.updateSuccess'));
            },
            onError: () => {
                toast.error(t('llmPrice.updateFailed'));
            }
        });
    };

    const handleRebuild = () => {
        if (rebuildCountdown > 0) return;
        rebuildPrice.mutate(undefined, {
            onSuccess: ({ count }) => {
                setConfirmRebuild(false);
                setRebuildCountdown(0);
                toast.success(t('llmPrice.rebuild.success', { count }));
            },
            onError: () => {
                setConfirmRebuild(false);
                setRebuildCountdown(0);
                toast.error(t('llmPrice.rebuild.failed'));
            }
        });
    };

    const formatLastUpdateTime = (timeStr: string | undefined) => {
        if (!timeStr) return t('llmPrice.neverUpdated');
        const date = new Date(timeStr);
        if (date.getFullYear() === 1) return t('llmPrice.neverUpdated');
        return date.toLocaleString();
    };

    return (
        <div className="rounded-3xl border border-border bg-card p-6 space-y-5">
            <h2 className="text-lg font-bold text-card-foreground flex items-center gap-2">
                <DollarSign className="h-5 w-5" />
                {t('llmPrice.title')}
            </h2>

            {/* 更新间隔 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <Clock className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('llmPrice.updateInterval.label')}</span>
                </div>
                <Input
                    type="number"
                    value={updateInterval}
                    onChange={(e) => setUpdateInterval(e.target.value)}
                    onBlur={() => handleSave(SettingKey.ModelInfoUpdateInterval, updateInterval, initialUpdateInterval.current)}
                    placeholder={t('llmPrice.updateInterval.placeholder')}
                    className="w-48 rounded-xl"
                />
            </div>

            {/* 人民币汇率: 仅用于前端显示与输入换算, 存储数据仍为美元 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex flex-col gap-1">
                    <div className="flex items-center gap-3">
                        <DollarSign className="h-5 w-5 text-muted-foreground" />
                        <span className="text-sm font-medium">{t('llmPrice.exchangeRate.label')}</span>
                    </div>
                    <span className="text-xs text-muted-foreground ml-8">{t('llmPrice.exchangeRate.hint')}</span>
                </div>
                <Input
                    type="number"
                    step="0.01"
                    min="0"
                    value={rate}
                    onChange={(e) => setRate(e.target.value)}
                    onBlur={() => handleSave(SettingKey.USDCNYRate, rate, initialRate.current)}
                    placeholder="7.2"
                    className="w-48 rounded-xl"
                />
            </div>

            {/* 金额显示货币 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <Coins className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('llmPrice.displayCurrency.label')}</span>
                </div>
                <div className="flex gap-2">
                    {(['USD', 'CNY'] as const).map((currency) => (
                        <button
                            key={currency}
                            type="button"
                            onClick={() => handleDisplayCurrencyChange(currency)}
                            className={cn(
                                'inline-flex h-8 items-center justify-center gap-1.5 rounded-lg border px-3 text-xs font-medium transition-colors',
                                displayCurrency === currency
                                    ? 'border-primary/30 bg-primary text-primary-foreground'
                                    : 'border-border bg-muted/20 text-foreground hover:bg-muted/30'
                            )}
                        >
                            <Banknote className="size-3.5" />
                            {currency === 'USD' ? t('llmPrice.displayCurrency.usd') : t('llmPrice.displayCurrency.cny')}
                        </button>
                    ))}
                </div>
            </div>

            {/* 手动更新 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex flex-col gap-1">
                    <div className="flex items-center gap-3">
                        <RefreshCw className="h-5 w-5 text-muted-foreground" />
                        <span className="text-sm font-medium">{t('llmPrice.manualUpdate.label')}</span>
                    </div>
                    <span className="text-xs text-muted-foreground ml-8">
                        {t('llmPrice.lastUpdate')}: {formatLastUpdateTime(lastUpdateTime)}
                    </span>
                </div>
                <Button
                    variant="outline"
                    size="sm"
                    onClick={handleManualUpdate}
                    disabled={updatePrice.isPending}
                    className="rounded-xl"
                >
                    {updatePrice.isPending ? t('llmPrice.manualUpdate.updating') : t('llmPrice.manualUpdate.button')}
                </Button>
            </div>

            {/* 重建价格数据 */}
            <div className="flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                    <DatabaseBackup className="h-5 w-5 text-muted-foreground" />
                    <span className="text-sm font-medium">{t('llmPrice.rebuild.label')}</span>
                </div>
                {!confirmRebuild ? (
                    <Button
                        variant="outline"
                        size="sm"
                        onClick={() => {
                            setRebuildCountdown(3);
                            setConfirmRebuild(true);
                        }}
                        disabled={rebuildPrice.isPending}
                        className="rounded-xl"
                    >
                        {t('llmPrice.rebuild.button')}
                    </Button>
                ) : (
                    <div className="flex items-center gap-2">
                        <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => {
                                setConfirmRebuild(false);
                                setRebuildCountdown(0);
                            }}
                            disabled={rebuildPrice.isPending}
                            className="rounded-xl"
                        >
                            {t('llmPrice.rebuild.cancel')}
                        </Button>
                        <Tooltip open>
                            <TooltipTrigger asChild>
                                <span className="inline-flex">
                                    <Button
                                        variant="destructive"
                                        size="sm"
                                        onClick={handleRebuild}
                                        disabled={rebuildCountdown > 0 || rebuildPrice.isPending}
                                        className="min-w-24 rounded-xl"
                                    >
                                        {rebuildPrice.isPending
                                            ? t('llmPrice.rebuild.rebuilding')
                                            : rebuildCountdown > 0
                                                ? `${rebuildCountdown}s`
                                                : t('llmPrice.rebuild.confirm')}
                                    </Button>
                                </span>
                            </TooltipTrigger>
                            <TooltipContent side="top" sideOffset={10} align="center" className="whitespace-pre-line text-center">
                                {t('llmPrice.rebuild.description')}
                            </TooltipContent>
                        </Tooltip>
                    </div>
                )}
            </div>
        </div>
    );
}
